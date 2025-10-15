// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	DefaultLimit           = 16
	MaxLimit               = 1024 // Reduced maximum items per page to limit DoS impact
	MaxCursorLength        = 512  // Kubernetes API Server's default maximum URL length is 8KB (8192 characters)
	MaxDecodedCursorLength = 512  // Maximum decoded cursor content size
)

// writeSuccessResponse writes a successful API response
func writeSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.SuccessResponse(data)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Can't change status code (already sent), write error to response
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// writeErrorResponse writes an error API response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.ErrorResponse(message, code)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Can't change status code (already sent), but log the error
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// writeListResponse writes a paginated list response
func writeTokenExpiredError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)

	metadata := map[string]any{
		"retryable": true,
		"code":      "CONTINUE_TOKEN_EXPIRED",
	}

	response := models.ErrorResponseWithMetadata(
		"Continue token has expired",
		"CONTINUE_TOKEN_EXPIRED",
		metadata,
	)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

func writeListResponse[T any](w http.ResponseWriter, items []T, total, page, pageSize int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.ListSuccessResponse(items, total, page, pageSize)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// parseCursorParams parses cursor and limit parameters with security bounds and feature flags
//
// Pagination Mode Precedence (highest to lowest):
// 1. Explicit ?pagination=cursor or ?pagination=legacy parameter
// 2. Presence of ?cursor parameter (enables cursor mode)
// 3. Feature flag config.GetCursorPaginationEnabled()
// 4. Default: legacy mode (useCursor=false)
func parseCursorParams(r *http.Request) (cursor string, limit int64, useCursor bool, err error) {
	query := r.URL.Query()

	cursor = query.Get("cursor")
	limitStr := query.Get("limit")
	mode := query.Get("pagination")

	// Determine pagination mode using precedence rules
	// Precedence 1: Explicit mode parameter overrides everything
	if mode == "cursor" {
		useCursor = true
	} else if mode == "legacy" {
		useCursor = false
	} else if mode != "" {
		// Invalid pagination mode specified
		return "", 0, false, fmt.Errorf("invalid pagination mode: %s. Valid values are 'cursor' or 'legacy'", mode)
	} else if cursor != "" {
		// Precedence 2: Presence of cursor parameter enables cursor mode
		useCursor = true
	} else {
		// Precedence 3: Feature flag determines default behavior
		useCursor = config.GetCursorPaginationEnabled()
	}

	// Validate cursor if we're using cursor mode
	if useCursor {
		if err := validateCursorModeParams(cursor); err != nil {
			return "", 0, false, err
		}
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

// validateCursor is the single source of truth for cursor validation
// It consolidates all cursor validation logic to prevent inconsistencies
func validateCursor(cursor string) error {
	if cursor == "" {
		// Allow empty cursor for first page
		return nil
	}

	// 1. Length check (encoded)
	if len(cursor) > MaxCursorLength {
		return fmt.Errorf("cursor exceeds maximum allowed length of %d characters", MaxCursorLength)
	}

	// 2. Base64 decode validation
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(cursor)
		if err != nil {
			return fmt.Errorf("cursor format is invalid: malformed base64 encoding")
		}
	}

	// 3. SECURITY: Validate decoded content
	// Check for null bytes (binary injection prevention)
	for _, b := range decoded {
		if b == 0x00 {
			return fmt.Errorf("cursor format is invalid: contains null bytes")
		}
	}

	// 4. Validate decoded length
	if len(decoded) > MaxDecodedCursorLength {
		return fmt.Errorf("cursor exceeds maximum decoded size of %d bytes", MaxDecodedCursorLength)
	}

	// 5. Validate UTF-8 encoding (content sanity check)
	if len(decoded) > 0 && !isValidUTF8(decoded) {
		return fmt.Errorf("cursor format is invalid: not valid UTF-8")
	}

	return nil
}

// validateCursorModeParams validates cursor-specific parameters
// Deprecated: Use validateCursor instead for comprehensive validation
func validateCursorModeParams(cursor string) error {
	return validateCursor(cursor)
}

// validateCursorWithContext validates the cursor string with security bounds
// Deprecated: Use validateCursor instead for comprehensive validation
func validateCursorWithContext(cursor string) error {
	return validateCursor(cursor)
}

func isValidContinueToken(token string) bool {
	if len(token) > MaxCursorLength {
		return false
	}

	// 1. Validate it's actually valid base64 and decode
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(token)
		if err != nil {
			return false
		}
	}

	// 2. SECURITY: Validate decoded content
	// Check for null bytes (binary injection prevention)
	for _, b := range decoded {
		if b == 0x00 {
			return false
		}
	}

	// 3. Validate decoded length
	if len(decoded) > MaxDecodedCursorLength {
		return false
	}

	// 4. Validate UTF-8 encoding (content sanity check)
	// Kubernetes continue tokens should be valid UTF-8
	if len(decoded) > 0 && !isValidUTF8(decoded) {
		return false
	}

	return true
}

// isValidUTF8 checks if the byte slice is valid UTF-8
func isValidUTF8(b []byte) bool {
	return utf8.Valid(b)
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}
