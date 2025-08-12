package config

import (
	"os"
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
	return Config{
		Mode:            mode,
		HTTPAddr:        addr,
		PublicURL:       os.Getenv("PUBLIC_URL"),
		DBDriver:        envOr("DB_DRIVER", "sqlite"),
		DBDSN:           os.Getenv("DB_DSN"),
		BlobDriver:      envOr("BLOB_DRIVER", "fs"),
		BlobBasePath:    envOr("BLOB_BASE_PATH", "./data"),
		EnableLocalAuth: envBool("ENABLE_LOCAL_AUTH", true),
		EnableLTI:       envBool("ENABLE_LTI", mode == ModeOnline),
		EnableJWKS:      envBool("ENABLE_JWKS", mode == ModeOnline),
		AdminUser:       envOr("ADMIN_USER", "admin"),
		AdminPassHash:   envOr("ADMIN_PASS_HASH", "$2y$12$pyZAiWaTfVtM7UElIRStvOC3gNbnp70nmQU4eYopLGBfCJr1DOvji"),
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
