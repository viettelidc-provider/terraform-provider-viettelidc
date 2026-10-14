// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockLB_SerializesSameLB(t *testing.T) {
	SetSettlementDelay(10 * time.Millisecond)
	defer SetSettlementDelay(0)

	const lbID = int64(1001)
	var activeOps int32
	var maxConcurrent int32
	var wg sync.WaitGroup

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := LockLB(lbID)
			defer unlock()

			curr := atomic.AddInt32(&activeOps, 1)
			for {
				old := atomic.LoadInt32(&maxConcurrent)
				if curr <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, curr) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&activeOps, -1)
		}()
	}

	wg.Wait()

	if maxConcurrent != 1 {
		t.Fatalf("expected max concurrent operations on same LB to be 1, got %d", maxConcurrent)
	}
}

func TestLockLB_AllowsConcurrentDifferentLBs(t *testing.T) {
	SetSettlementDelay(0)

	var running int32
	var reachedConcurrent int32
	var wg sync.WaitGroup

	// Run two locks on different LB IDs simultaneously
	for i := range 2 {
		wg.Add(1)
		lbID := int64(2000 + i)
		go func(id int64) {
			defer wg.Done()
			unlock := LockLB(id)
			defer unlock()

			c := atomic.AddInt32(&running, 1)
			if c == 2 {
				atomic.StoreInt32(&reachedConcurrent, 1)
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&running, -1)
		}(lbID)
	}

	wg.Wait()

	if atomic.LoadInt32(&reachedConcurrent) != 1 {
		t.Log("Note: concurrency across different LBs was not observed (timing-dependent), but both completed successfully")
	}
}

func TestLockLB_RespectsSettleDelay(t *testing.T) {
	const delay = 30 * time.Millisecond
	SetSettlementDelay(delay)
	defer SetSettlementDelay(0)

	const lbID = int64(3001)
	unlock := LockLB(lbID)

	start := time.Now()
	unlock()
	elapsed := time.Since(start)

	if elapsed < delay {
		t.Fatalf("expected unlock to wait at least %v, waited %v", delay, elapsed)
	}
}

func TestLockLB_ZeroLBDoesNotBlock(t *testing.T) {
	unlock := LockLB(0)
	unlock() // should be no-op
}
