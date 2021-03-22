package crypto

import (
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	key := MakeSignSecretKey()
	tok := MintToken(key, Token{Scope: "foo", Subject: "blah blah"}, time.Hour)

	decoded, err := DecodeToken(tok)
	if err != nil {
		t.Fatalf("DecodeToken failed: %v", err)
	}
	if decoded.Scope != "foo" || decoded.Subject != "blah blah" {
		t.Errorf("Unexpected token. Got %+v, want {'foo', 'blah blah'}", decoded)
	}

	if err := ValidateToken(key, decoded); err != nil {
		t.Errorf("ValidateToken failed. err = %v", err)
	}

	decoded.Scope = "bar" // Invalidates the signature
	if err := ValidateToken(key, decoded); err == nil {
		t.Errorf("ValidateToken(invTok) succeeded unexpectedly, err=%v", err)
	}
}
