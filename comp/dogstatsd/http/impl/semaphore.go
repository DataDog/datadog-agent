// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

// semaphore bounds how many requests are in flight at once. A nil semaphore is
// unlimited, so callers never have to special case a disabled limit.
type semaphore chan struct{}

// newSemaphore returns a semaphore of n slots, or an unlimited one if n is zero
// or less.
func newSemaphore(n int) semaphore {
	if n <= 0 {
		return nil
	}
	return make(semaphore, n)
}

// acquire claims a slot and reports whether it got one. It never blocks: a
// caller that is refused is expected to give up rather than wait.
func (s semaphore) acquire() bool {
	if s == nil {
		return true
	}
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}

// release returns a slot claimed by a successful acquire.
func (s semaphore) release() {
	if s == nil {
		return
	}
	<-s
}
