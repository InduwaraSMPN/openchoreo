// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// FeatureFlags contains feature flag configuration
type FeatureFlags struct {
	CursorPaginationEnabled bool `json:"cursor_pagination_enabled"`
	// Add other feature flags here as needed
}

// Config contains the complete API configuration
type Config struct {
	Features FeatureFlags `json:"features"`
	// Add other configuration sections here
}

var (
	globalConfig *Config
	configMutex  sync.RWMutex
	lastLoadTime time.Time
	cacheTTL     = 5 * time.Minute // Cache config for 5 minutes
)

// LoadFeatureFlags loads the feature flags configuration
func LoadFeatureFlags() (*Config, error) {
	configMutex.RLock()

	// Return cached config if still fresh
	if globalConfig != nil && time.Since(lastLoadTime) < cacheTTL {
		defer configMutex.RUnlock()
		return globalConfig, nil
	}
	configMutex.RUnlock()

	// Acquire write lock to reload config
	configMutex.Lock()
	defer configMutex.Unlock()

	// Double-check after acquiring write lock
	if globalConfig != nil && time.Since(lastLoadTime) < cacheTTL {
		return globalConfig, nil
	}

	config := &Config{
		Features: FeatureFlags{
			CursorPaginationEnabled: false, // Default to safe legacy mode
		},
	}

	// Load from environment variable first (highest priority)
	if envValue := os.Getenv("CURSOR_PAGINATION_ENABLED"); envValue != "" {
		config.Features.CursorPaginationEnabled = envValue == "true"
	}

	// Load from config file if it exists
	if err := loadFromFile("config/flags.json", config); err != nil {
		// Log warning but continue with defaults/env vars
		// In a real implementation, you'd use proper logging
		_ = err
	}

	globalConfig = config
	lastLoadTime = time.Now()

	return config, nil
}

// loadFromFile loads configuration from a JSON file
func loadFromFile(filename string, config *Config) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, config)
}

// GetCursorPaginationEnabled returns the current state of the cursor pagination flag
func GetCursorPaginationEnabled() bool {
	config, err := LoadFeatureFlags()
	if err != nil {
		// Fail safe to disabled if config loading fails
		return false
	}
	return config.Features.CursorPaginationEnabled
}

// InvalidateCache forces a reload of the configuration on next access
func InvalidateCache() {
	configMutex.Lock()
	defer configMutex.Unlock()

	globalConfig = nil
	lastLoadTime = time.Time{}
}
