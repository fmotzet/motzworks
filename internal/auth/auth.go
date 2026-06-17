// Package auth handles password hashing and stateless, HMAC-signed session
// tokens for the dashboard/API. Tokens are self-contained (no server-side
// session store): base64(payload).base64(hmac-sha256(payload)).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Roles.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// Claims are carried in a session token.
type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"role"`
	Expires  int64  `json:"exp"` // unix seconds
}

// HashPassword returns a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateSecret returns a random base64 secret for signing tokens.
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

var b64 = base64.RawURLEncoding

// IssueToken signs the claims and returns a token string.
func IssueToken(secret []byte, c Claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := b64.EncodeToString(payload)
	return p + "." + b64.EncodeToString(sign(secret, p)), nil
}

// VerifyToken validates a token's signature and expiry and returns its claims.
func VerifyToken(secret []byte, token string) (Claims, error) {
	var c Claims
	dot := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return c, errors.New("malformed token")
	}
	p, sig := token[:dot], token[dot+1:]

	want := sign(secret, p)
	got, err := b64.DecodeString(sig)
	if err != nil {
		return c, errors.New("malformed token signature")
	}
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return c, errors.New("invalid token signature")
	}

	payload, err := b64.DecodeString(p)
	if err != nil {
		return c, errors.New("malformed token payload")
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return c, err
	}
	if time.Now().Unix() > c.Expires {
		return c, errors.New("token expired")
	}
	return c, nil
}

func sign(secret []byte, payload string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// NormalizeRole validates and returns a role, defaulting to viewer.
func NormalizeRole(role string) (string, error) {
	switch role {
	case RoleAdmin, RoleViewer:
		return role, nil
	case "":
		return RoleViewer, nil
	default:
		return "", fmt.Errorf("invalid role %q (want admin or viewer)", role)
	}
}
