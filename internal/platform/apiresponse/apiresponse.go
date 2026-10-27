// Package apiresponse renders every HTTP response of the API using a single
// envelope. Handlers must never call c.JSON directly.
//
//	success: {"success": true,  "data": {...}, "meta": {...}}
//	error:   {"success": false, "error": {"code", "message", "details"}, "meta": {...}}
package apiresponse

import (
	"errors"
	"log/slog"
	"net/http"

	"atm-identity-api/internal/platform/apierror"

	"github.com/gin-gonic/gin"
)

// RequestIDKey is the gin context key holding the request id.
const RequestIDKey = "request_id"

type Meta struct {
	RequestID string `json:"request_id,omitempty"`
	// Source tells where a lookup was resolved: "database" or "apisperu".
	Source string `json:"source,omitempty"`
}

type successBody struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
	Meta    Meta `json:"meta"`
}

type errorPayload struct {
	Code    apierror.Code     `json:"code"`
	Message string            `json:"message"`
	Details []apierror.Detail `json:"details,omitempty"`
}

type errorBody struct {
	Success bool         `json:"success"`
	Error   errorPayload `json:"error"`
	Meta    Meta         `json:"meta"`
}

func meta(c *gin.Context, source string) Meta {
	id, _ := c.Get(RequestIDKey)
	rid, _ := id.(string)
	return Meta{RequestID: rid, Source: source}
}

// OK sends 200 with data. Pass an empty source when it does not apply.
func OK(c *gin.Context, data any, source string) {
	c.JSON(http.StatusOK, successBody{Success: true, Data: data, Meta: meta(c, source)})
}

// Created sends 201 with data.
func Created(c *gin.Context, data any, source string) {
	c.JSON(http.StatusCreated, successBody{Success: true, Data: data, Meta: meta(c, source)})
}

// Fail renders err. Anything that is not an *apierror.Error becomes a 500 and
// its details are only logged.
func Fail(c *gin.Context, err error) {
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) {
		apiErr = apierror.Internal(err)
	}
	if apiErr.Cause != nil || apiErr.Status >= http.StatusInternalServerError {
		slog.Error("request failed",
			"code", apiErr.Code, "path", c.FullPath(), "request_id", meta(c, "").RequestID, "cause", apiErr.Cause)
	}
	c.AbortWithStatusJSON(apiErr.Status, errorBody{
		Success: false,
		Error:   errorPayload{Code: apiErr.Code, Message: apiErr.Message, Details: apiErr.Details},
		Meta:    meta(c, ""),
	})
}
