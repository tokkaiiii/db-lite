package auth

import (
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "correct-horse") {
		t.Error("CheckPassword should accept the original password")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword should reject a wrong password")
	}
}

func TestTokenIssueAndVerifyRoundtrip(t *testing.T) {
	issuer := NewTokenIssuer([]byte("test-secret"), time.Hour)

	token, err := issuer.Issue(42, "alice", true)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" || !claims.IsAdmin {
		t.Errorf("claims = %+v, want UserID=42 Username=alice IsAdmin=true", claims)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	issued := NewTokenIssuer([]byte("secret-a"), time.Hour)
	token, err := issued.Issue(1, "bob", false)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	other := NewTokenIssuer([]byte("secret-b"), time.Hour)
	if _, err := other.Verify(token); err == nil {
		t.Error("Verify should reject a token signed with a different secret")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	issuer := NewTokenIssuer([]byte("test-secret"), -time.Hour)
	token, err := issuer.Issue(1, "bob", false)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := issuer.Verify(token); err == nil {
		t.Error("Verify should reject an already-expired token")
	}
}
