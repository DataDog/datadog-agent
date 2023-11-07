// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"sync"
	"time"
)

type minTracker struct {
	sync.Mutex
	val            int
	timestamp      time.Time
	expiryDuration time.Duration
}

func newMinTracker(expiryDuration time.Duration) *minTracker {
	return &minTracker{
		val:            -1,
		timestamp:      time.Now(),
		expiryDuration: expiryDuration,
	}
}

func (mt *minTracker) update(newVal int) {
	mt.Lock()
	defer mt.Unlock()

	isSet := mt.val >= 0
	hasExpired := time.Since(mt.timestamp) > mt.expiryDuration

	if newVal <= mt.val || !isSet || hasExpired {
		mt.val = newVal
		mt.timestamp = time.Now()
	}
}

func (mt *minTracker) get() int {
	mt.Lock()
	defer mt.Unlock()
	return mt.val
}
