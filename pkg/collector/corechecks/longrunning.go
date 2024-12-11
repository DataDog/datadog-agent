// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//nolint:revive
package corechecks

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

// LongRunningCheck is a wrapper that converts a long running check
// (with Run never terminating) to a typical check in order to be handled
// in the agent status.
type LongRunningCheck interface {
	check.Check

	GetSender() (sender.Sender, error)
}

// LongRunningCheckWrapper provides a wrapper for long running checks
// that will be used by the collector to handle the check lifecycle.
type LongRunningCheckWrapper struct {
	LongRunningCheck
	running bool
	mutex   sync.Mutex

	stopped bool
}

// NewLongRunningCheckWrapper returns a new LongRunningCheckWrapper
func NewLongRunningCheckWrapper(check LongRunningCheck) *LongRunningCheckWrapper {
	return &LongRunningCheckWrapper{LongRunningCheck: check, mutex: sync.Mutex{}}
}

// Run runs the check in a goroutine if it is not already running.
// If the check is already running, it will commit the sender.
func (cw *LongRunningCheckWrapper) Run() error {
	if cw.LongRunningCheck == nil {
		return fmt.Errorf("no check defined")
	}
	cw.mutex.Lock()
	defer cw.mutex.Unlock()

	if cw.stopped {
		return fmt.Errorf("check already stopped")
	}

	if cw.running {
		s, err := cw.LongRunningCheck.GetSender()
		if err != nil {
			return fmt.Errorf("error getting sender: %w", err)
		}
		s.Commit()
		return nil
	}

	cw.running = true
	go func() {
		if err := cw.LongRunningCheck.Run(); err != nil {
			fmt.Printf("Error running check: %v\n", err)
		}
		// Long running checks are not meant to be restarted. Thus we never reset the running flag.
	}()

	return nil
}

// Interval defines how frequently we should update the metrics.
// It can't be 0 otherwise the check will be considered as a long running check,
// Run() will be called only once and the metrics won't be collected.
func (cw *LongRunningCheckWrapper) Interval() time.Duration {
	return 15 * time.Second
}

// GetSenderStats returns the stats from the last run of the check and sets the field
// LongRunningCheck to true. It is necessary for formatting the stats in the status page.
func (cw *LongRunningCheckWrapper) GetSenderStats() (stats.SenderStats, error) {
	if cw.LongRunningCheck == nil {
		return stats.SenderStats{}, fmt.Errorf("no check defined")
	}
	s, err := cw.LongRunningCheck.GetSenderStats()
	if err != nil {
		return stats.SenderStats{}, fmt.Errorf("error getting sender stats: %w", err)
	}
	s.LongRunningCheck = true
	return s, nil
}

// Cancel calls the cancel method of the check.
// It makes sure it is called only once.
func (cw *LongRunningCheckWrapper) Cancel() {
	cw.mutex.Lock()
	defer cw.mutex.Unlock()
	if cw.stopped {
		return
	}
	cw.LongRunningCheck.Cancel()
	cw.stopped = true
}
