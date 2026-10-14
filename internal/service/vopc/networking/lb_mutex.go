// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	lbMutexesMu sync.Mutex
	lbMutexes   = make(map[int64]*sync.Mutex)
	settleDelay = 10 * time.Second
)

func init() {
	if v := os.Getenv("VTT_LB_SETTLE_DELAY"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec >= 0 {
			settleDelay = time.Duration(sec) * time.Second
		}
	}
}

// SetSettlementDelay overrides the default settle delay (useful in tests).
func SetSettlementDelay(d time.Duration) {
	lbMutexesMu.Lock()
	defer lbMutexesMu.Unlock()
	settleDelay = d
}

// GetSettlementDelay returns the current settle delay.
func GetSettlementDelay() time.Duration {
	lbMutexesMu.Lock()
	defer lbMutexesMu.Unlock()
	return settleDelay
}

// LockLB acquires a per-load-balancer mutex to serialize all mutating operations
// on the given load balancer or its sub-resources (listeners, pools, members,
// health monitors, L7 policies, L7 rules).
//
// It returns an unlock function. When called, the unlock function waits for
// settleDelay before releasing the mutex, ensuring Octavia returns to an ACTIVE
// state before subsequent operations proceed on the same load balancer.
func LockLB(lbID int64) func() {
	if lbID <= 0 {
		return func() {}
	}

	lbMutexesMu.Lock()
	m, ok := lbMutexes[lbID]
	if !ok {
		m = &sync.Mutex{}
		lbMutexes[lbID] = m
	}
	lbMutexesMu.Unlock()

	m.Lock()
	return func() {
		delay := GetSettlementDelay()
		if delay > 0 {
			time.Sleep(delay)
		}
		m.Unlock()
	}
}
