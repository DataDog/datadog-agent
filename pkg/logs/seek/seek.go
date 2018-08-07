// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package seek

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
)

// Strategy represents the stategy a tailer should be using while starting tailing a new element.
type Strategy int

const (
	// Start tail from the beginning
	Start Strategy = iota
	// Recover tail from a given offset
	Recover
	// End tail from the end
	End
)

// Seeker provides seek strategies to inputs when starting tailing new element.
type Seeker struct {
	origin  time.Time
	auditor *auditor.Auditor
}

// NewSeeker returns a new seeker
func NewSeeker(auditor *auditor.Auditor) *Seeker {
	return &Seeker{
		origin:  time.Now(),
		auditor: auditor,
	}
}

// Seek returns the position to be used by a tailer when starting tailing an element:
// - elements that have already been tailed in the past should be tailed from an offset registered in the auditor
// - elements that have been created before the agent start should be tailed from the end
// - elements that have been created after the agent start should be tailed from the beginning
func (s *Seeker) Seek(ctime time.Time, identifier string) (Strategy, string) {
	offset := s.auditor.GetLastCommittedOffset(identifier)
	if offset != "" {
		return Recover, offset
	}
	if ctime.After(s.origin) {
		return Start, ""
	}
	return End, ""
}
