package admin

type CreateToolReq struct {
	ClientID      string
	Name          string
	JWKSURL       string
	RedirectURIs  []string
	AllowedScopes []string
	AuthMethods   []string
}

type CreateDeploymentReq struct {
	ID        string
	ClientID  string
	ContextID string
	Title     string
}
