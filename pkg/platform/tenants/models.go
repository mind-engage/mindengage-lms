package tenants

type Tenant struct {
	ID        string
	Issuer    string // https://{tenant}.lti.mindengage.com
	ActiveKID string
}

type Tool struct {
	ClientID      string
	TenantID      string
	Name          string
	JWKSURL       string
	RedirectURIs  []string
	AllowedScopes []string
	AuthMethods   []string // "private_key_jwt", "client_secret_post"
}

type Deployment struct {
	ID        string
	TenantID  string
	ClientID  string
	ContextID string
	Title     string
}
