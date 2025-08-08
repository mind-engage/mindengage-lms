package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthService struct{ hmac []byte }

func NewAuthService(secret string) *AuthService { return &AuthService{hmac: []byte(secret)} }

type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"` // "teacher" or "student"
	jwt.RegisteredClaims
}

func (a *AuthService) IssueJWT(sub, role string) (string, error) {
	now := time.Now()
	claims := &Claims{
		Sub:  sub,
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "mindengage-offline",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(8 * time.Hour)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(a.hmac)
}

func (a *AuthService) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return a.hmac, nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}
	c, _ := token.Claims.(*Claims)
	return c, nil
}

// POST /auth/login  { "username": "...", "password": "...", "role": "teacher|student" }
func LoginHandler(a *AuthService) http.HandlerFunc {
	// ultra-minimal: "teacher:teacher" and "student:student" (replace with your own)
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		valid := (req.Username == req.Password) && (req.Role == "teacher" || req.Role == "student")
		if !valid {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		tok, err := a.IssueJWT(req.Username, req.Role)
		if err != nil {
			http.Error(w, "issue token", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": tok})
	}
}

func JWTMiddleware(a *AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			_, err := a.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
