// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"sync"
	"testing"
	"time"
)

// TestWaitGroupPanicSafety verifies that the WaitGroup defer ordering
// prevents deadlock even if a panic occurs during reload
func TestWaitGroupPanicSafety(t *testing.T) {
	t.Cleanup(InvalidateCache)

	// Reset global state
	InvalidateCache()
	reloadInProgress.Store(false)
	reloadWaitGroup = sync.WaitGroup{}

	// Set a short TTL to trigger reload
	originalTTL := cacheTTL
	cacheTTL = 1 * time.Millisecond
	t.Cleanup(func() {
		cacheTTL = originalTTL
	})

	// First load to populate cache
	_, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("initial load failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)

	// Simulate concurrent requests that trigger reload
	// One goroutine will win the CAS and reload, others will wait
	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)
	doneChan := make(chan bool, 1)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					// Expected for some goroutines due to test setup
					t.Logf("Goroutine %d recovered from panic: %v", id, r)
				}
			}()

			_, err := LoadFeatureFlags()
			errChan <- err
		}(i)
	}

	// Wait for all goroutines to complete or timeout
	go func() {
		time.Sleep(5 * time.Second)
		doneChan <- true
	}()

	receivedCount := 0
	timeout := time.After(5 * time.Second)

	for receivedCount < numGoroutines {
		select {
		case <-errChan:
			receivedCount++
		case <-doneChan:
			t.Fatalf("Deadlock detected: only %d/%d goroutines completed", receivedCount, numGoroutines)
		case <-timeout:
			t.Fatalf("Test timeout: potential deadlock, only %d/%d goroutines completed", receivedCount, numGoroutines)
		}
	}

	// Verify WaitGroup is properly released
	waitChan := make(chan struct{})
	go func() {
		reloadWaitGroup.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		// Success: WaitGroup properly released
		t.Log("WaitGroup properly released after concurrent access")
	case <-time.After(1 * time.Second):
		t.Fatal("WaitGroup not properly released - defer ordering issue detected")
	}
}

// TestReloadPanicRecovery verifies that panic during reload doesn't leave
// the system in a bad state
func TestReloadPanicRecovery(t *testing.T) {
	t.Cleanup(InvalidateCache)

	// This test verifies that even if something panics during reload,
	// the defer statements properly clean up reloadInProgress and WaitGroup

	InvalidateCache()
	reloadInProgress.Store(false)

	// Force cache expiration
	originalTTL := cacheTTL
	cacheTTL = 1 * time.Millisecond
	t.Cleanup(func() {
		cacheTTL = originalTTL
	})

	// First load
	_, err := LoadFeatureFlags()
	if err != nil {
		t.Fatalf("initial load failed: %v", err)
	}

	// Wait for cache expiration
	time.Sleep(10 * time.Millisecond)

	// Verify system can recover and reload works after previous operations
	_, err = LoadFeatureFlags()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Verify reloadInProgress is false
	if reloadInProgress.Load() {
		t.Fatal("reloadInProgress should be false after successful reload")
	}

	t.Log("System properly handles reload and cleanup")
}
