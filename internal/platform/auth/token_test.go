package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testIssuer = "atm-auth"

func newTestVerifier(t *testing.T) (*TokenVerifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	verifier, err := NewTokenVerifier(base64.StdEncoding.EncodeToString(pemBytes), testIssuer)
	if err != nil {
		t.Fatal(err)
	}
	return verifier, key
}

func sign(t *testing.T, key *rsa.PrivateKey, issuer, subject, role string, expires time.Duration) string {
	t.Helper()
	claims := serviceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expires)),
		},
		Role: role,
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSystemCode(t *testing.T) {
	verifier, key := newTestVerifier(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		token   string
		want    string
		wantErr bool
	}{
		{"service token", sign(t, key, testIssuer, "system:hotel", "service", time.Hour), "hotel", false},
		{"user token", sign(t, key, testIssuer, "user-1", "user", time.Hour), "", true},
		{"service role without system subject", sign(t, key, testIssuer, "user-1", "service", time.Hour), "", true},
		{"expired", sign(t, key, testIssuer, "system:hotel", "service", -time.Second), "", true},
		{"other issuer", sign(t, key, "someone-else", "system:hotel", "service", time.Hour), "", true},
		{"other signing key", sign(t, otherKey, testIssuer, "system:hotel", "service", time.Hour), "", true},
		{"garbage", "not-a-token", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := verifier.SystemCode(tt.token)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("SystemCode() = %q, %v; want %q, error=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
