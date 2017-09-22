package utils

import (
	"errors"
	"time"
)

const pollInterval = 500 * time.Millisecond

var errTimeout = errors.New("timed out waiting for condition")

type condFunc func() bool

func waitFor(f condFunc, timeout time.Duration) error {
	out := make(chan struct{}, 1)
	go func() {
		for {
			if f() {
				out <- struct{}{}
			}

			time.Sleep(pollInterval)
		}
	}()

	select {
	case <-out:
		return nil
	case <-time.After(timeout):
		return errTimeout
	}
}
