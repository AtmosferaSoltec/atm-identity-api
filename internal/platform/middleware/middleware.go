package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/apiresponse"

	"github.com/gin-gonic/gin"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		c.Set(apiresponse.RequestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// Logger logs the route pattern, never the raw path: paths carry document numbers.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		id, _ := c.Get(apiresponse.RequestIDKey)
		client, _ := c.Get("client_system_code")
		slog.Info("request", "method", c.Request.Method, "route", c.FullPath(),
			"status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(),
			"request_id", id, "client", client)
	}
}

// Recovery turns panics into the standard 500 envelope.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		apiresponse.Fail(c, apierror.Internal(nil))
	})
}

// NotFound renders unknown routes with the standard envelope.
func NotFound(c *gin.Context) {
	apiresponse.Fail(c, apierror.New(404, apierror.CodeNotFound, "Route not found."))
}
