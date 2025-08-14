package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mind-engage/mindengage-lms/pkg/platform/config"
	"github.com/mind-engage/mindengage-lms/pkg/platform/lti"
	"github.com/mind-engage/mindengage-lms/pkg/platform/lti/ags"
	"github.com/mind-engage/mindengage-lms/pkg/platform/lti/deeplinking"
	"github.com/mind-engage/mindengage-lms/pkg/platform/lti/nrps"
)

/* --------- tiny stubs so the server compiles; replace later --------- */

// issuerResolverFunc satisfies lti.IssuerResolver & deeplinking.IssuerResolver
type issuerResolverFunc func(context.Context, string) (string, error)

func (f issuerResolverFunc) IssuerForTenant(ctx context.Context, tenantID string) (string, error) {
	return f(ctx, tenantID)
}

// OAuth client registry (for /oauth/token). Replace with real impl.
type stubOAuthRegistry struct{}

func (stubOAuthRegistry) GetOAuthClient(ctx context.Context, tenantID, clientID string) (lti.OAuthClient, error) {
	// Return a placeholder client; real impl should fetch from storage
	return lti.OAuthClient{
		ClientID:      clientID,
		SecretHash:    "$2a$14$replace-me-with-real-bcrypt-hash",
		AllowedScopes: nil, // allow default set
	}, nil
}

// NRPS storage stub
type stubNRPSStore struct{}

func (stubNRPSStore) ListMemberships(ctx context.Context, tenantID, contextID, roleFilter, pageToken string, limit int) ([]nrps.Membership, string, error) {
	return []nrps.Membership{}, "", nil
}
func (stubNRPSStore) GetContextMeta(ctx context.Context, tenantID, contextID string) (nrps.ContextMeta, error) {
	return nrps.ContextMeta{ID: contextID}, nil
}

// Deep Linking stubs
type stubDLRegistry struct{}

func (stubDLRegistry) GetTool(ctx context.Context, tenantID, clientID string) (deeplinking.Tool, error) {
	return deeplinking.Tool{}, errors.New("stub: deep-link registry not implemented")
}

type stubDLVerifier struct{}

func (stubDLVerifier) VerifyToolJWT(ctx context.Context, tenantID, toolClientID, rawJWT, expectedAud string) (map[string]any, error) {
	return nil, errors.New("stub: deep-link verifier not implemented")
}

type stubDLStore struct{}

func (stubDLStore) UpsertResourceLink(ctx context.Context, tenantID, clientID, deploymentID, contextID, resourceLinkID, title, targetURL string, custom map[string]string) (string, error) {
	return resourceLinkID, nil
}
func (stubDLStore) UpsertPlatformLineItem(ctx context.Context, tenantID, contextID, resourceLinkID, resourceID, label string, scoreMax float64) (string, error) {
	return "", nil
}

/* --------------------------------------------------------------------- */

func main() {
	var cfg config.Config // TODO: load from env/file
	// If cfg.Platform.Bind is not set in your config, fall back to :8080
	if cfg.Platform.Bind == "" {
		cfg.Platform.Bind = ":8080"
	}

	resolveTenantID := func(r *http.Request) (string, error) {
		// TODO: derive tenant from host/path/header
		return "default", nil
	}

	// Until you wire a real issuer per tenant, use a static base. Replace later.
	issuerResolver := issuerResolverFunc(func(ctx context.Context, tenantID string) (string, error) {
		// Example issuer; set this to your public base URL
		return "http://localhost:8080", nil
	})

	// Key manager for JWKS + signing
	keyManager := &lti.KeyManager{
		Storage:          lti.NewInMemoryKeyStorage(), // TODO: replace in prod
		Alg:              "RS256",
		RSAKeyBits:       2048,
		RotationInterval: 90 * 24 * time.Hour,
		Overlap:          7 * 24 * time.Hour,
	}

	r := chi.NewRouter()

	// JWKS (/.well-known/jwks.json)
	jwks := &lti.JWKSHandler{
		ResolveTenantID: resolveTenantID,
		Provider:        keyManager,
	}
	r.Handle("/.well-known/jwks.json", jwks)

	// OAuth token endpoint
	ts := &lti.TokenServer{
		ResolveTenantID: resolveTenantID,
		Issuers:         issuerResolver,
		Registry:        stubOAuthRegistry{},
		Signer:          keyManager,
		AccessTokenTTL:  time.Hour,
	}
	r.Post("/oauth/token", ts.Handler())

	// NOTE: We are intentionally skipping the OpenID metadata and /oidc/authorize
	// wiring here, because your current lti.MetadataServer and lti.AuthorizeServer
	// APIs in the repo don’t match what main.go expected. Add them back once you
	// confirm the exact constructors/handlers in:
	//   pkg/platform/lti/metadata.go
	//   pkg/platform/lti/authorize.go
	// and mount the returned http.Handlers here.

	// AGS routes (fill Server deps if your ags.Server requires them)
	agsServer := &ags.Server{}
	r.Mount("/api/lti/ags", ags.Routes(agsServer))

	// NRPS routes
	nrpsServer := &nrps.Server{
		Store:           stubNRPSStore{},
		ResolveTenantID: resolveTenantID,
	}
	r.Mount("/api/lti/nrps", nrps.Routes(nrpsServer))

	// Deep Linking response
	dl := &deeplinking.Server{
		ResolveTenantID: resolveTenantID,
		Issuers:         issuerResolver,
		Tools:           stubDLRegistry{},
		Verify:          stubDLVerifier{},
		Store:           stubDLStore{},
	}
	r.Handle("/lti/deep-linking/response", dl.ResponseHandler())

	s := &http.Server{
		Addr:              cfg.Platform.Bind,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(s.ListenAndServe())
}
