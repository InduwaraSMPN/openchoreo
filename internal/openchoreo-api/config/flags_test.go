// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFeatureFlags(t *testing.T) {
	const configPath = "config/flags.json"

	originalData, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to read original config file: %v", err)
	}
	hadOriginal := err == nil

	t.Cleanup(func() {
		if hadOriginal {
			if writeErr := os.WriteFile(configPath, originalData, 0o600); writeErr != nil {
				t.Fatalf("failed to restore config file: %v", writeErr)
			}
		} else if removeErr := os.Remove(configPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			t.Fatalf("failed to remove test config file: %v", removeErr)
		}
		InvalidateCache()
	})

	writeConfigFile := func(t *testing.T, content string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			t.Fatalf("failed to create config directory: %v", err)
		}
		if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
	}

	t.Run("env overrides file", func(t *testing.T) {
		t.Cleanup(InvalidateCache)

		writeConfigFile(t, `{"features":{"cursor_pagination_enabled":false}}`)
		t.Setenv("CURSOR_PAGINATION_ENABLED", "true")

		InvalidateCache()
		cfg, err := LoadFeatureFlags()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.Features.CursorPaginationEnabled {
			t.Fatalf("expected env override to enable cursor pagination")
		}
	})

	t.Run("file used when env unset", func(t *testing.T) {
		t.Cleanup(InvalidateCache)

		writeConfigFile(t, `{"features":{"cursor_pagination_enabled":true}}`)
		t.Setenv("CURSOR_PAGINATION_ENABLED", "")
		if err := os.Unsetenv("CURSOR_PAGINATION_ENABLED"); err != nil {
			t.Fatalf("failed to unset env: %v", err)
		}

		InvalidateCache()
		cfg, err := LoadFeatureFlags()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.Features.CursorPaginationEnabled {
			t.Fatalf("expected file configuration to be used when env unset")
		}
	})

	t.Run("invalid JSON surfaces error", func(t *testing.T) {
		t.Cleanup(InvalidateCache)

		writeConfigFile(t, `{"features":{`) // malformed JSON
		t.Setenv("CURSOR_PAGINATION_ENABLED", "")
		if err := os.Unsetenv("CURSOR_PAGINATION_ENABLED"); err != nil {
			t.Fatalf("failed to unset env: %v", err)
		}

		InvalidateCache()
		cfg, err := LoadFeatureFlags()
		if err == nil {
			t.Fatalf("expected error when loading malformed JSON")
		}

		if cfg.Features.CursorPaginationEnabled {
			t.Fatalf("expected defaults to remain disabled on error")
		}
	})
}

func TestConfigCaching(t *testing.T) {
	t.Cleanup(InvalidateCache)

	t.Setenv("CURSOR_PAGINATION_ENABLED", "true")
	InvalidateCache()

	cfg1, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg1.Features.CursorPaginationEnabled {
		t.Fatalf("expected initial config to be enabled")
	}

	t.Setenv("CURSOR_PAGINATION_ENABLED", "false")

	cfg2, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg1 != cfg2 {
		t.Fatalf("expected cached config pointer to be reused")
	}
	if !cfg2.Features.CursorPaginationEnabled {
		t.Fatalf("expected cached value to remain true")
	}

	InvalidateCache()

	cfg3, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg3 == cfg1 {
		t.Fatalf("expected cache invalidation to force new config instance")
	}
	if cfg3.Features.CursorPaginationEnabled {
		t.Fatalf("expected updated config to reflect new env value")
	}
}

func TestCacheTTL(t *testing.T) {
	t.Cleanup(InvalidateCache)

	originalTTL := cacheTTL
	cacheTTL = 5 * time.Millisecond
	t.Cleanup(func() {
		cacheTTL = originalTTL
	})

	t.Setenv("CURSOR_PAGINATION_ENABLED", "true")
	InvalidateCache()

	cfg1, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg1.Features.CursorPaginationEnabled {
		t.Fatalf("expected initial config to be enabled")
	}

	cfg2, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg1 != cfg2 {
		t.Fatalf("expected cache to return same pointer before TTL expiry")
	}

	t.Setenv("CURSOR_PAGINATION_ENABLED", "false")

	deadline := time.Now().Add(5 * cacheTTL)
	var cfg3 *Config
	for time.Now().Before(deadline) {
		cfg3, err = LoadFeatureFlags()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg3 != cfg1 {
			break
		}
		time.Sleep(cacheTTL / 2)
	}

	if cfg3 == nil || cfg3 == cfg1 {
		t.Fatalf("expected cache to refresh after TTL expiry")
	}
	if cfg3.Features.CursorPaginationEnabled {
		t.Fatalf("expected refreshed config to reflect updated env value")
	}
}
