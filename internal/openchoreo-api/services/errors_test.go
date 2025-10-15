// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestIsExpiredTokenError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "k8s Gone error",
			err:      apierrors.NewGone("resource gone"),
			expected: true,
		},
		{
			name:     "wrapped k8s Gone error",
			err:      fmt.Errorf("wrap: %w", apierrors.NewGone("resource gone")),
			expected: true,
		},
		{
			name:     "expired token message",
			err:      errors.New("continue token has expired"),
			expected: true,
		},
		{
			name:     "sentinel expired token error",
			err:      ErrContinueTokenExpired,
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isExpiredTokenError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsInvalidCursorError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "k8s BadRequest error",
			err:      apierrors.NewBadRequest("bad request"),
			expected: true,
		},
		{
			name:     "wrapped k8s BadRequest error",
			err:      fmt.Errorf("wrap: %w", apierrors.NewBadRequest("bad request")),
			expected: true,
		},
		{
			name:     "invalid cursor message",
			err:      errors.New("invalid cursor format"),
			expected: true,
		},
		{
			name:     "sentinel invalid cursor error",
			err:      ErrInvalidCursorFormat,
			expected: true,
		},
		{
			name:     "invalid token message",
			err:      errors.New("invalid token provided"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInvalidCursorError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsServiceUnavailableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "k8s ServiceUnavailable error",
			err:      apierrors.NewServiceUnavailable("service unavailable"),
			expected: true,
		},
		{
			name:     "wrapped k8s ServiceUnavailable error",
			err:      fmt.Errorf("wrap: %w", apierrors.NewServiceUnavailable("service unavailable")),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isServiceUnavailableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
