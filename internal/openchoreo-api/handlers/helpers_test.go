// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
    "encoding/base64"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

func TestParseCursorParams(t *testing.T) {
	// Save original config and restore after test
	originalEnabled := config.GetCursorPaginationEnabled()
	defer func() {
		// Reset config after test
		if originalEnabled {
			t.Setenv("CURSOR_PAGINATION_ENABLED", "true")
		} else {
			t.Setenv("CURSOR_PAGINATION_ENABLED", "false")
		}
		config.InvalidateCache()
	}()

	longCursor := strings.Repeat("x", MaxCursorLength+1)

	tests := []struct {
		name              string
		url               string
		featureEnabled    bool
		expectedCursor    string
		expectedLimit     int64
		expectedUseCursor bool
		expectError       bool
	}{
		{
			name:              "feature disabled, no cursor params",
			url:               "/api/v1/orgs",
			featureEnabled:    false,
			expectedCursor:    "",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: false,
			expectError:       false,
		},
		{
			name:              "feature enabled, no cursor params",
			url:               "/api/v1/orgs",
			featureEnabled:    true,
			expectedCursor:    "",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:              "feature enabled, cursor param present",
			url:               "/api/v1/orgs?cursor=SGVsbG8gV29ybGQ=", // "Hello World" in base64
			featureEnabled:    true,
			expectedCursor:    "SGVsbG8gV29ybGQ=",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:              "feature disabled, cursor param forces cursor mode",
			url:               "/api/v1/orgs?cursor=SGVsbG8gV29ybGQ=", // "Hello World" in base64
			featureEnabled:    false,
			expectedCursor:    "SGVsbG8gV29ybGQ=",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:              "explicit cursor pagination mode",
			url:               "/api/v1/orgs?pagination=cursor",
			featureEnabled:    false,
			expectedCursor:    "",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:           "cursor too long rejected",
			url:            fmt.Sprintf("/api/v1/orgs?pagination=cursor&cursor=%s", longCursor),
			featureEnabled: false,
			expectError:    true,
		},
		{
			name:              "explicit legacy pagination mode",
			url:               "/api/v1/orgs?pagination=legacy",
			featureEnabled:    true,
			expectedCursor:    "",
			expectedLimit:     DefaultLimit,
			expectedUseCursor: false,
			expectError:       false,
		},
		{
			name:              "legacy pagination honors limit",
			url:               "/api/v1/orgs?pagination=legacy&limit=50",
			featureEnabled:    true,
			expectedCursor:    "",
			expectedLimit:     50,
			expectedUseCursor: false,
			expectError:       false,
		},
		{
			name:           "invalid pagination mode",
			url:            "/api/v1/orgs?pagination=invalid",
			featureEnabled: true,
			expectError:    true,
		},
		{
			name:              "custom limit",
			url:               "/api/v1/orgs?pagination=cursor&limit=50",
			featureEnabled:    true,
			expectedCursor:    "",
			expectedLimit:     50,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:              "limit exceeds maximum",
			url:               "/api/v1/orgs?pagination=cursor&limit=2000",
			featureEnabled:    true,
			expectedCursor:    "",
			expectedLimit:     MaxLimit,
			expectedUseCursor: true,
			expectError:       false,
		},
		{
			name:           "zero limit",
			url:            "/api/v1/orgs?pagination=cursor&limit=0",
			featureEnabled: true,
			expectError:    true,
		},
		{
			name:           "negative limit",
			url:            "/api/v1/orgs?pagination=cursor&limit=-1",
			featureEnabled: true,
			expectError:    true,
		},
		{
			name:           "invalid limit format",
			url:            "/api/v1/orgs?pagination=cursor&limit=abc",
			featureEnabled: true,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(config.InvalidateCache)

			// Set feature flag
			if tt.featureEnabled {
				t.Setenv("CURSOR_PAGINATION_ENABLED", "true")
			} else {
				t.Setenv("CURSOR_PAGINATION_ENABLED", "false")
			}
			config.InvalidateCache()

			req := httptest.NewRequest("GET", tt.url, nil)
			cursor, limit, useCursor, err := parseCursorParams(req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cursor != tt.expectedCursor {
				t.Errorf("expected cursor %q, got %q", tt.expectedCursor, cursor)
			}

			if limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit)
			}

			if useCursor != tt.expectedUseCursor {
				t.Errorf("expected useCursor %v, got %v", tt.expectedUseCursor, useCursor)
			}
		})
	}
}

func TestValidateCursorModeParams(t *testing.T) {
	tests := []struct {
		name        string
		cursor      string
		expectError bool
	}{
		{
			name:        "empty cursor",
			cursor:      "",
			expectError: false,
		},
		{
			name:        "valid cursor length",
			cursor:      "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			expectError: false,
		},
		{
			name: "max length allowed",
			// Create a valid base64 string that's within limits when decoded
			cursor:      "eyJ2ZXJzaW9uIjoxLCJjb250aW51ZSI6InRlc3QiLCJydiI6IjEyMzQ1In0=", // Valid JSON in base64
			expectError: false,
		},
		{
			name:        "cursor too long",
			cursor:      strings.Repeat("b", MaxCursorLength+1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursorModeParams(tt.cursor)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCursorWithContext(t *testing.T) {
	tests := []struct {
		name        string
		cursor      string
		expectError bool
	}{
		{
			name:        "empty cursor",
			cursor:      "",
			expectError: false,
		},
		{
			name:        "valid base64 cursor",
			cursor:      "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			expectError: false,
		},
		{
			name:        "valid URL-safe base64",
			cursor:      "SGVsbG8tV29ybGQ_",
			expectError: false,
		},
		{
			name:        "invalid character set",
			cursor:      "invalid@cursor!",
			expectError: true,
		},
		{
			name:        "invalid base64",
			cursor:      "not-base64!!!",
			expectError: true,
		},
		{
			name:        "invalid base64 despite valid charset",
			cursor:      "AAAAAAA",
			expectError: true,
		},
		{
			name:        "cursor too long",
			cursor:      strings.Repeat("c", MaxCursorLength+1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursorWithContext(tt.cursor)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsValidContinueToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{
			name:  "empty token",
			token: "",
			valid: true, // Empty is valid for first page
		},
		{
			name:  "valid base64",
			token: "SGVsbG8gV29ybGQ=",
			valid: true,
		},
		{
			name:  "valid URL-safe base64",
			token: "SGVsbG8tV29ybGQ_",
			valid: true,
		},
		{
			name:  "invalid character",
			token: "invalid@token!",
			valid: false,
		},
		{
			name:  "invalid base64 structure",
			token: "not-base64!!!",
			valid: false,
		},
		{
			name:  "valid chars but invalid base64",
			token: "AAAAAAA", // 7 chars, not valid base64 padding
			valid: false,
		},
		{
			name:  "exceeds max length",
			token: strings.Repeat("A", MaxCursorLength+1),
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidContinueToken(tt.token)
			if result != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, result)
			}
		})
	}
}

func TestValidateCursorContentSecurity(t *testing.T) {
    // Null byte in decoded content should be rejected
    // base64 of "A\x00B"
    cursorWithNull := "QQBC" // Decodes to A\x00B
    if err := validateCursor(cursorWithNull); err == nil {
        t.Fatalf("expected null-byte cursor to be invalid")
    }

    // Decoded content exceeding MaxDecodedCursorLength should be rejected
    decoded := []byte(strings.Repeat("A", MaxDecodedCursorLength+1))
    encoded := base64.StdEncoding.EncodeToString(decoded)
    if err := validateCursor(encoded); err == nil {
        t.Fatalf("expected decoded-length-exceeding cursor to be invalid")
    }
}
