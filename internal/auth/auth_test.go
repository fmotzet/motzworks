package auth

import (
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "s3cret!") {
		t.Error("correct password rejected")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password accepted")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	c := Claims{UserID: "u1", Username: "admin", Role: RoleAdmin, Expires: time.Now().Add(time.Hour).Unix()}
	tok, err := IssueToken(secret, c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyToken(secret, tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Username != "admin" || got.Role != RoleAdmin {
		t.Errorf("claims = %+v", got)
	}
}

func TestTokenTampering(t *testing.T) {
	secret := []byte("test-secret")
	c := Claims{UserID: "u1", Role: RoleViewer, Expires: time.Now().Add(time.Hour).Unix()}
	tok, _ := IssueToken(secret, c)

	if _, err := VerifyToken([]byte("other-secret"), tok); err == nil {
		t.Error("expected signature failure with wrong secret")
	}
	if _, err := VerifyToken(secret, tok+"x"); err == nil {
		t.Error("expected failure for tampered token")
	}
}

func TestTokenExpiry(t *testing.T) {
	secret := []byte("test-secret")
	c := Claims{UserID: "u1", Role: RoleViewer, Expires: time.Now().Add(-time.Minute).Unix()}
	tok, _ := IssueToken(secret, c)
	if _, err := VerifyToken(secret, tok); err == nil {
		t.Error("expected expired token to fail")
	}
}

func TestNormalizeRole(t *testing.T) {
	if r, _ := NormalizeRole(""); r != RoleViewer {
		t.Errorf("default role = %q", r)
	}
	if _, err := NormalizeRole("superuser"); err == nil {
		t.Error("expected invalid role error")
	}
}
