// pkg/platform/lti/keys.go
package lti

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
Key manager & signer for the MindEngage LTI Platform

What this file provides:

  • A KeyManager that:
      - Generates per-tenant signing keys (RSA-2048 by default)
      - Rotates keys on a schedule
      - Exposes a JWKS for verification (implements JWKSProvider)
      - Signs JWTs for LTI id_tokens (implements Signer)

  • An in-memory KeyStorage implementation suitable for dev/tests.

How to wire:

    // Construct a manager (keep one singleton in your platform service)
    km := &KeyManager{
        Storage:          NewInMemoryKeyStorage(),
        Alg:              "RS256",
        RSAKeyBits:       2048,
        RotationInterval: 90 * 24 * time.Hour,
        Overlap:          7 * 24 * time.Hour,   // keep old keys visible this long
    }

    // Use as the Signer in the authorize server:
    authz := &AuthorizeServer{ Signer: km, ...}

    // Use as the JWKS provider:
    jwks := &JWKSHandler{
        ResolveTenantID: resolveTenant,
        Provider:        km,
    }

Production notes:
- Replace InMemoryKeyStorage with a durable store (SQL/kv/HSM) in production.
- Never expose private material; PublicJWKS returns only public parameters.
- Overlap should be >= maximum token lifetime + clock skew.
*/

var (
	ErrNoActiveKey = errors.New("keys: no active signing key for tenant")
)

// --------------------------------- Models ------------------------------------

// KeyRecord holds a tenant signing key and its lifecycle window.
type KeyRecord struct {
	KID       string
	Alg       string // e.g., "RS256", "ES256"
	CreatedAt time.Time
	NotBefore time.Time
	NotAfter  time.Time // when key should stop being used to sign new tokens

	// One of the following is set, depending on Alg.
	RSAPrivate   *rsa.PrivateKey
	ECDSAPrivate *ecdsa.PrivateKey
}

// Public returns a public-only view of the key (for JWKS building).
func (k KeyRecord) Public() map[string]any {
	switch {
	case k.RSAPrivate != nil:
		return RSAPublicJWK(&k.RSAPrivate.PublicKey, k.KID, k.Alg)
	case k.ECDSAPrivate != nil:
		return ECPublicJWK(&k.ECDSAPrivate.PublicKey, k.KID, k.Alg)
	default:
		return nil
	}
}

// IsActive returns true when 'now' is within [NotBefore, NotAfter).
func (k KeyRecord) IsActive(now time.Time) bool {
	return !now.Before(k.NotBefore) && now.Before(k.NotAfter)
}

// IsVisibleInJWKS returns true when the key should still be published in JWKS
// to validate tokens signed near the end of its life (caller adds overlap).
func (k KeyRecord) IsVisibleInJWKS(now, overlap time.Time) bool {
	// Visible until NotAfter+overlap >= now
	return now.Before(overlap)
}

// -------------------------------- Storage ------------------------------------

// KeyStorage persists keys per tenant. Provide a durable implementation for prod.
type KeyStorage interface {
	// List returns all keys for the tenant (any order). Caller may sort.
	List(ctx context.Context, tenantID string) ([]KeyRecord, error)
	// Save inserts or replaces a key by KID.
	Save(ctx context.Context, tenantID string, rec KeyRecord) error
	// Get returns a key by KID.
	Get(ctx context.Context, tenantID, kid string) (KeyRecord, error)
}

// InMemoryKeyStorage is a process-local storage (dev/tests).
type InMemoryKeyStorage struct {
	mu   sync.RWMutex
	data map[string]map[string]KeyRecord // tenant -> kid -> key
}

func NewInMemoryKeyStorage() *InMemoryKeyStorage {
	return &InMemoryKeyStorage{data: make(map[string]map[string]KeyRecord)}
}

func (s *InMemoryKeyStorage) List(_ context.Context, tenantID string) ([]KeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.data[tenantID]
	if m == nil {
		return nil, nil
	}
	out := make([]KeyRecord, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out, nil
}

func (s *InMemoryKeyStorage) Save(_ context.Context, tenantID string, rec KeyRecord) error {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(rec.KID) == "" {
		return errors.New("keystore: tenant and kid required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.data[tenantID]
	if m == nil {
		m = make(map[string]KeyRecord)
		s.data[tenantID] = m
	}
	m[rec.KID] = rec
	return nil
}

func (s *InMemoryKeyStorage) Get(_ context.Context, tenantID, kid string) (KeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.data[tenantID]
	if m == nil {
		return KeyRecord{}, errors.New("keystore: tenant not found")
	}
	rec, ok := m[kid]
	if !ok {
		return KeyRecord{}, errors.New("keystore: key not found")
	}
	return rec, nil
}

// ------------------------------- Key Manager ---------------------------------

// KeyManager implements both Signer and JWKSProvider.
type KeyManager struct {
	Storage KeyStorage

	// Algorithm & generation options
	Alg        string // default "RS256"
	RSAKeyBits int    // default 2048

	// Rotation policy
	RotationInterval time.Duration // default 90 days
	Overlap          time.Duration // default 7 days (JWKS visibility beyond NotAfter)

	// Clock (for tests)
	Now func() time.Time

	// internal lock to serialize rotations per tenant
	mu sync.Mutex
}

// Sign implements the Signer interface. It ensures an active key and signs claims.
func (km *KeyManager) Sign(ctx context.Context, tenantID string, claims map[string]any) (string, error) {
	if km.Storage == nil {
		return "", errors.New("keys: storage not configured")
	}
	rec, err := km.ensureCurrentKey(ctx, tenantID)
	if err != nil {
		return "", err
	}
	// Prepare header
	header := map[string]any{
		"alg": rec.Alg,
		"typ": "JWT",
		"kid": rec.KID,
	}
	return km.signJWT(header, claims, rec)
}

// PublicJWKS implements the JWKSProvider interface (used by JWKSHandler).
func (km *KeyManager) PublicJWKS(ctx context.Context, tenantID string) (JWKS, error) {
	if km.Storage == nil {
		return JWKS{}, errors.New("keys: storage not configured")
	}
	keys, err := km.Storage.List(ctx, tenantID)
	if err != nil {
		return JWKS{}, err
	}
	now := km.now()
	// Include keys whose NotBefore <= now and now < NotAfter+Overlap
	cutoff := now.Add(km.overlap())
	var jwks JWKS
	for _, k := range keys {
		if now.Before(k.NotBefore) {
			continue
		}
		if !cutoff.Before(k.NotAfter) || cutoff.Equal(k.NotAfter) {
			// keep while cutoff <= NotAfter ? No: we want (now <= NotAfter+overlap)
			// So include when now <= NotAfter+overlap
			// If cutoff <= NotAfter fails, then now > NotAfter+overlap, so skip.
			// The condition above is inverted; rewrite simply:
		}
		if now.After(k.NotAfter.Add(km.overlap())) {
			continue
		}
		if pub := k.Public(); pub != nil {
			jwks.Keys = append(jwks.Keys, pub)
		}
	}
	// Sort by newest first (optional)
	sort.SliceStable(jwks.Keys, func(i, j int) bool {
		ki, _ := jwks.Keys[i]["kid"].(string)
		kj, _ := jwks.Keys[j]["kid"].(string)
		return ki > kj
	})
	return jwks, nil
}

// ensureCurrentKey returns an active key for signing; rotates if needed.
func (km *KeyManager) ensureCurrentKey(ctx context.Context, tenantID string) (KeyRecord, error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	keys, err := km.Storage.List(ctx, tenantID)
	if err != nil {
		return KeyRecord{}, err
	}
	now := km.now()

	// Find an active key
	var current *KeyRecord
	for i := range keys {
		if keys[i].IsActive(now) {
			cp := keys[i]
			current = &cp
			break
		}
	}
	if current != nil && now.Add(km.overlap()).Before(current.NotAfter) {
		// Current key not close to rotation window -> use it
		return *current, nil
	}

	// If no active or expiring soon, generate and store a new key
	rec, err := km.generateKey(now)
	if err != nil {
		return KeyRecord{}, err
	}
	if err := km.Storage.Save(ctx, tenantID, rec); err != nil {
		return KeyRecord{}, err
	}
	return rec, nil
}

func (km *KeyManager) generateKey(now time.Time) (KeyRecord, error) {
	alg := km.alg()
	switch alg {
	case "RS256":
		priv, err := rsa.GenerateKey(rand.Reader, km.rsaBits())
		if err != nil {
			return KeyRecord{}, fmt.Errorf("rsa generate: %w", err)
		}
		kid := makeKID("rsa", &priv.PublicKey)
		return KeyRecord{
			KID:          kid,
			Alg:          "RS256",
			CreatedAt:    now,
			NotBefore:    now,
			NotAfter:     now.Add(km.rotateEvery()),
			RSAPrivate:   priv,
			ECDSAPrivate: nil,
		}, nil
	default:
		return KeyRecord{}, fmt.Errorf("unsupported alg %q (only RS256 implemented here)", alg)
	}
}

func (km *KeyManager) signJWT(header, claims map[string]any, rec KeyRecord) (string, error) {
	// Base64URL(header) + "." + Base64URL(claims) + "." + signature
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	hEnc := b64url(hb)
	cEnc := b64url(cb)
	toSign := hEnc + "." + cEnc

	var sig []byte
	switch rec.Alg {
	case "RS256":
		if rec.RSAPrivate == nil {
			return "", errors.New("key: missing RSA private key")
		}
		sum := sha256.Sum256([]byte(toSign))
		s, err := rsa.SignPKCS1v15(rand.Reader, rec.RSAPrivate, crypto.SHA256, sum[:])
		if err != nil {
			return "", err
		}
		sig = s
	default:
		return "", fmt.Errorf("sign: unsupported alg %q", rec.Alg)
	}

	jws := toSign + "." + b64url(sig)
	return jws, nil
}

// --------------------------------- Helpers -----------------------------------

func (km *KeyManager) now() time.Time {
	if km.Now != nil {
		return km.Now()
	}
	return time.Now().UTC()
}

func (km *KeyManager) alg() string {
	if strings.TrimSpace(km.Alg) == "" {
		return "RS256"
	}
	return km.Alg
}

func (km *KeyManager) rsaBits() int {
	if km.RSAKeyBits <= 0 {
		return 2048
	}
	return km.RSAKeyBits
}

func (km *KeyManager) rotateEvery() time.Duration {
	if km.RotationInterval <= 0 {
		return 90 * 24 * time.Hour
	}
	return km.RotationInterval
}

func (km *KeyManager) overlap() time.Duration {
	if km.Overlap <= 0 {
		return 7 * 24 * time.Hour
	}
	return km.Overlap
}

// makeKID creates a deterministic kid from the public key material plus entropy.
func makeKID(prefix string, pub *rsa.PublicKey) string {
	// Hash modulus + exponent; append short random suffix to avoid collisions across tenants
	h := sha256.New()
	if pub != nil {
		h.Write(pub.N.Bytes())
		h.Write([]byte{byte(pub.E >> 24), byte(pub.E >> 16), byte(pub.E >> 8), byte(pub.E)})
	}
	r := make([]byte, 4)
	_, _ = rand.Read(r)
	sum := h.Sum(nil)
	return fmt.Sprintf("%s-%s-%s", prefix, hex.EncodeToString(sum[:6]), hex.EncodeToString(r))
}

// -------------------------------- Convenience --------------------------------

// SeedRSAKey allows injecting a pre-generated RSA private key (e.g., from disk)
// for a tenant with explicit validity window. Useful during migrations.
func (km *KeyManager) SeedRSAKey(ctx context.Context, tenantID, kid string, priv *rsa.PrivateKey, notBefore, notAfter time.Time) error {
	if km.Storage == nil {
		return errors.New("keys: storage not configured")
	}
	if priv == nil {
		return errors.New("keys: nil rsa key")
	}
	if strings.TrimSpace(kid) == "" {
		kid = makeKID("rsa", &priv.PublicKey)
	}
	rec := KeyRecord{
		KID:          kid,
		Alg:          "RS256",
		CreatedAt:    km.now(),
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		RSAPrivate:   priv,
		ECDSAPrivate: nil,
	}
	return km.Storage.Save(ctx, tenantID, rec)
}

// ActiveKID returns the kid of the currently active key (if any).
func (km *KeyManager) ActiveKID(ctx context.Context, tenantID string) (string, error) {
	keys, err := km.Storage.List(ctx, tenantID)
	if err != nil {
		return "", err
	}
	now := km.now()
	for _, k := range keys {
		if k.IsActive(now) {
			return k.KID, nil
		}
	}
	return "", ErrNoActiveKey
}
