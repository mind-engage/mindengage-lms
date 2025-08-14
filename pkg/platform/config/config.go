package config

import "time"

type TLS struct {
	CertFile string
	KeyFile  string
}

type DB struct {
	Driver string // postgres|sqlite
	DSN    string
}

type Issuer struct {
	// Base like "https://{tenant}.lti.mindengage.com"
	BaseURL       string
	HostIsTenant  bool   // true if {tenant}.domain
	PathTenantKey string // e.g. "/t/{tenant}" when HostIsTenant=false
}

type Keys struct {
	RotateEvery time.Duration
	GracePeriod time.Duration
	KeystoreURI string // e.g., file://, kms://
}

type Platform struct {
	Bind                 string // ":8443"
	TenantHeader         string // optional override for tenancy
	DevAllowClientSecret bool
}

type Config struct {
	TLS      TLS
	DB       DB
	Issuer   Issuer
	Keys     Keys
	Platform Platform
}
