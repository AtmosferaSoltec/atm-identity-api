// Package auth authenticates consuming systems by API key (header X-API-Key).
// Keys are stored as SHA-256 hashes; they are high-entropy random strings, so a
// fast hash is appropriate and lets us look the key up by its hash.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/apiresponse"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClientSystemKey is the gin context key holding the caller's client system code.
const ClientSystemKey = "client_system_code"

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CreateClientSystem registers a system and returns its API key. The key is
// shown once and cannot be recovered.
func CreateClientSystem(ctx context.Context, pool *pgxpool.Pool, code, name string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := "atm_" + hex.EncodeToString(raw)
	if _, err := pool.Exec(ctx,
		`INSERT INTO client_systems (code, name, api_key_hash) VALUES ($1, $2, $3)`,
		code, name, hashKey(key)); err != nil {
		return "", fmt.Errorf("insert client system: %w", err)
	}
	return key, nil
}

// Middleware authenticates a consuming system. The preferred way is the service
// token of ATM-Auth-API (Authorization: Bearer); the legacy X-API-Key keeps
// working while systems migrate. verifier may be nil, which disables tokens.
func Middleware(pool *pgxpool.Pool, verifier *TokenVerifier) gin.HandlerFunc {
	legacy := apiKeyMiddleware(pool)
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if verifier == nil || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			legacy(c)
			return
		}
		code, err := verifier.SystemCode(parts[1])
		if err != nil {
			apiresponse.Fail(c, apierror.Unauthorized())
			return
		}
		c.Set(ClientSystemKey, code)
		c.Next()
	}
}

// apiKeyMiddleware rejects requests without a valid, active API key.
func apiKeyMiddleware(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			apiresponse.Fail(c, apierror.Unauthorized())
			return
		}
		var code string
		var active bool
		err := pool.QueryRow(c.Request.Context(),
			`UPDATE client_systems SET last_used_at = now() WHERE api_key_hash = $1 RETURNING code, is_active`,
			hashKey(key)).Scan(&code, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			apiresponse.Fail(c, apierror.Unauthorized())
			return
		}
		if err != nil {
			apiresponse.Fail(c, apierror.Internal(err))
			return
		}
		if !active {
			apiresponse.Fail(c, apierror.New(403, apierror.CodeForbidden, "This API key is disabled."))
			return
		}
		c.Set(ClientSystemKey, code)
		c.Next()
	}
}
