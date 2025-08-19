package config

import (
	"os"
	"strings"
)

type Mode string

const (
	ModeOffline Mode = "offline"
	ModeOnline  Mode = "online"
)

type Config struct {
	Mode      Mode
	HTTPAddr  string
	PublicURL string

	DBDriver string
	DBDSN    string

	BlobDriver   string // fs|minio|gcs
	BlobBasePath string // for fs/minio

	EnableLocalAuth bool
	EnableLTI       bool
	EnableJWKS      bool

	AdminUser     string
	AdminPassHash string // bcrypt

	CORSOriginsOnline  []string
	CORSOriginsOffline []string

	// LTI 1.3 / OIDC (Tool-side)
	LTIPlatformAuthURL string
	LTIToolClientID    string
	LTIToolRedirectURI string

	EnableGoogleAuth bool

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string // e.g., PUBLIC_URL + "/api/auth/google/callback"
	GoogleAllowedHD    string // optional: re
}

func FromEnv() Config {
	mode := Mode(os.Getenv("MODE"))
	if mode == "" {
		mode = ModeOffline
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	pub := os.Getenv("PUBLIC_URL")
	defRedirect := ""
	if pub != "" {
		defRedirect = strings.TrimSuffix(pub, "/") + "/api/lti/launch"
	}
	return Config{
		Mode:               mode,
		HTTPAddr:           addr,
		PublicURL:          pub,
		DBDriver:           envOr("DB_DRIVER", "sqlite"),
		DBDSN:              envOr("DB_DSN", ""),
		BlobDriver:         envOr("BLOB_DRIVER", "fs"),
		BlobBasePath:       envOr("BLOB_BASE_PATH", "./data"),
		EnableLocalAuth:    envBool("ENABLE_LOCAL_AUTH", true),
		EnableLTI:          envBool("ENABLE_LTI", mode == ModeOnline),
		EnableJWKS:         envBool("ENABLE_JWKS", mode == ModeOnline),
		AdminUser:          envOr("ADMIN_USER", "admin"),
		AdminPassHash:      envOr("ADMIN_PASS_HASH", "$2y$12$pyZAiWaTfVtM7UElIRStvOC3gNbnp70nmQU4eYopLGBfCJr1DOvji"),
		CORSOriginsOnline:  csvOr("CORS_ORIGINS_ONLINE", "https://lms.mindengage.ai"),
		CORSOriginsOffline: csvOr("CORS_ORIGINS_OFFLINE", "http://localhost:3000,http://localhost:3010,http://localhost:3020"),

		LTIPlatformAuthURL: envOr("LTI_PLATFORM_AUTH_URL", "https://platform.mindengage.ai/oidc/auth"),
		LTIToolClientID:    envOr("LTI_TOOL_CLIENT_ID", "TOOL_CLIENT_ID"),
		LTIToolRedirectURI: envOr("LTI_TOOL_REDIRECT_URI", defRedirect),

		EnableGoogleAuth:   envBool("ENABLE_GOOGLE_AUTH", false),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI:  envOr("GOOGLE_REDIRECT_URI", strings.TrimSuffix(pub, "/")+"/api/auth/google/callback"),
		GoogleAllowedHD:    os.Getenv("GOOGLE_ALLOWED_HD"),
	}
}
func envOr(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func envBool(k string, def bool) bool {
	switch os.Getenv(k) {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	case "0", "false", "FALSE", "no", "NO":
		return false
	default:
		return def
	}
}
func csvOr(k, def string) []string {
	v := envOr(k, def)
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
