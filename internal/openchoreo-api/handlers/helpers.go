// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// writeSuccessResponse writes a successful API response
func writeSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.SuccessResponse(data)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// writeErrorResponse writes an error API response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.ErrorResponse(message, code)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// writeListResponse writes a paginated list response
func writeTokenExpiredError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)

	metadata := map[string]any{
		"retryable":     true,
		"retryAction":   "restart_pagination",
		"retryGuidance": "The pagination token has expired. To restart pagination, make a new request without the cursor parameter to get the first page, or if you need to continue from where you left off, implement checkpointing in your application to track the last successfully processed item.",
	}

	response := models.ErrorResponseWithMetadata(
		"Continue token has expired",
		"CONTINUE_TOKEN_EXPIRED",
		metadata,
	)
	_ = json.NewEncoder(w).Encode(response)
}

func writeListResponse[T any](w http.ResponseWriter, items []T, total, page, pageSize int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.ListSuccessResponse(items, total, page, pageSize)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}


// parsed limit as int64, defaulting to 16 if not provided or invalid.
func parseCursorParams(r *http.Request) (cursor string, limit int64, useCursor bool) {
	query := r.URL.Query()

	cursor = query.Get("cursor")
	limitStr := query.Get("limit")

	useCursor = cursor != "" || limitStr != ""

	limit = 16
	
	if limitStr != "" {
		if parsedLimit, err := strconv.ParseInt(limitStr, 10, 64); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	return cursor, limit, useCursor
}

// validates the cursor string, ensuring it does not exceed 8192 characters, which aligns with the Kubernetes API Server's default maximum URL length.
func validateCursorWithContext(cursor string) error {
	if cursor == "" {
		return nil
	}
	if len(cursor) > 8192 {
		return fmt.Errorf("cursor exceeds maximum allowed length")
	}

	if !isValidContinueToken(cursor) {
		return fmt.Errorf("cursor format is invalid")
	}

	return nil
}

func isValidContinueToken(token string) bool {
	if token == "" {
		return true
	}

	// manual character validation loop is intentionally used here to mitigate regex compilation overhead.
	for i := 0; i < len(token); i++ {
		c := token[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

func writeCursorListResponse[T any](w http.ResponseWriter, items []T, nextCursor string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var nextCursorPtr *string
	if nextCursor != "" {
		nextCursorPtr = &nextCursor
	}

	response := models.CursorListSuccessResponse(items, nextCursorPtr)
	_ = json.NewEncoder(w).Encode(response)
}