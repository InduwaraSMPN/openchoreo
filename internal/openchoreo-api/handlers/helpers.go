// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	DefaultLimit    = 16
	MaxLimit        = 1024 // Standard maximum 1024 items per page
	MaxCursorLength = 1024 // Kubernetes API Server's default maximum URL length is 8KB (8192 characters
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

// parseCursorParams parses cursor and limit parameters with security bounds and feature flags
func parseCursorParams(r *http.Request) (cursor string, limit int64, useCursor bool, err error) {
	query := r.URL.Query()

	cursor = query.Get("cursor")
	limitStr := query.Get("limit")

	// FEATURE FLAG CONTROLLED: Base decision on feature flag
	useCursor = false

	// Enable cursor mode if feature flag is on OR if client is already using cursor params
	if config.GetCursorPaginationEnabled() || cursor != "" {
		useCursor = true
		// Only validate cursor params if we're actually using cursor mode
		if cursor != "" || limitStr != "" {
			if err := validateCursorModeParams(cursor); err != nil {
				return "", 0, false, err
			}
		}
	}

	// For backward compatibility during transition period
	// Allow explicit mode switch via query parameter
	mode := query.Get("pagination")
	if mode == "cursor" {
		useCursor = true
		if err := validateCursorModeParams(cursor); err != nil {
			return "", 0, false, err
		}
	} else if mode == "legacy" {
		// Explicitly override feature flag for legacy mode
		useCursor = false
	} else if mode != "" {
		// Invalid pagination mode specified
		return "", 0, false, fmt.Errorf("invalid pagination mode: %s. Valid values are 'cursor' or 'legacy'", mode)
	}

	// SECURITY: Enforce reasonable limits to prevent DoS
	limit = DefaultLimit

	if limitStr != "" {
		if parsedLimit, parseErr := strconv.ParseInt(limitStr, 10, 64); parseErr != nil {
			return "", 0, false, fmt.Errorf("invalid limit format: %w", parseErr)
		} else if parsedLimit <= 0 {
			return "", 0, false, fmt.Errorf("limit must be positive, got: %d", parsedLimit)
		} else if parsedLimit > MaxLimit {
			limit = MaxLimit // Clamp to reasonable maximum
		} else {
			limit = parsedLimit
		}
	}

	return cursor, limit, useCursor, nil
}

// validateCursorModeParams validates cursor-specific parameters
func validateCursorModeParams(cursor string) error {
	if cursor != "" {
		if len(cursor) > MaxCursorLength {
			return fmt.Errorf("cursor exceeds maximum allowed length of %d characters", MaxCursorLength)
		}
	}

	// limitStr is validated in the main parseCursorParams function
	// This function can be extended to validate additional cursor-specific parameters in the future

	return nil
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
	if len(token) > MaxCursorLength {
		return false
	}

	// 1. Check character set (existing validation)
	for i := 0; i < len(token); i++ {
		c := token[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '-' || c == '_') {
			return false
		}
	}

	// 2. Validate it's actually valid base64
	_, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		_, err = base64.URLEncoding.DecodeString(token)
		if err != nil {
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
	} else {
		// State 2: Pagination complete - always return empty string for consistency
		// This tells clients "pagination is complete"
		emptyCursor := ""
		nextCursorPtr = &emptyCursor
	}
	// State 3: nil case - handled automatically by var declaration
	// This occurs when no results and no cursor needed

	response := models.CursorListSuccessResponse(items, nextCursorPtr)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}
