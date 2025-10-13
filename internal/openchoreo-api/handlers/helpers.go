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

const (
	DefaultLimit    = 16
	MaxLimit        = 1024 // Standard maximum 1024 items per page
	MaxCursorLength = 8192 // Kubernetes API Server's default maximum URL length
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

// parseCursorParams parses cursor and limit parameters with security bounds
func parseCursorParams(r *http.Request) (cursor string, limit int64, useCursor bool, err error) {
	query := r.URL.Query()

	cursor = query.Get("cursor")
	limitStr := query.Get("limit")

	useCursor = cursor != "" || limitStr != ""

	limit = DefaultLimit

	if limitStr != "" {
		if parsedLimit, parseErr := strconv.ParseInt(limitStr, 10, 64); parseErr != nil {
			return "", 0, false, fmt.Errorf("invalid limit format: %v", parseErr)
		} else if parsedLimit <= 0 {
			return "", 0, false, fmt.Errorf("limit must be positive, got: %d", parsedLimit)
		} else if parsedLimit > MaxLimit {
			limit = MaxLimit
		} else {
			limit = parsedLimit
		}
	}

	return cursor, limit, useCursor, nil
}

// validateCursorWithContext validates the cursor string with security bounds
func validateCursorWithContext(cursor string) error {
	if cursor == "" {
		// allow empty cursor for first page, but will enforce other restrictions
		return nil
	}
	if len(cursor) > MaxCursorLength {
		return fmt.Errorf("cursor exceeds maximum allowed length of %d characters", MaxCursorLength)
	}

	if !isValidContinueToken(cursor) {
		return fmt.Errorf("cursor format is invalid: contains unauthorized characters")
	}

	return nil
}

func isValidContinueToken(token string) bool {
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
		// State 1: More pages available - return the token
		nextCursorPtr = &nextCursor
	} else if len(items) > 0 {
		// State 2: End of results - return empty string
		// This tells clients "pagination is complete"
		emptyCursor := ""
		nextCursorPtr = &emptyCursor
	}
	// State 3: nil case - handled automatically by var declaration
	// This occurs when no results and no cursor needed

	response := models.CursorListSuccessResponse(items, nextCursorPtr)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}
