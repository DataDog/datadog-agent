// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netflowstate

import (
	"errors"
)

// ErrAlreadyStarted error happens when you try to start twice a flow routine
var ErrAlreadyStarted = errors.New("the routine is already started")

// stopper mechanism, common for all the flow routines
type stopper struct {
	stopCh chan struct{}
}

func (s *stopper) start() error {
	if s.stopCh != nil {
		return ErrAlreadyStarted
	}
	s.stopCh = make(chan struct{})
	return nil
}

func (s *stopper) Shutdown() {
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}

		s.stopCh = nil
	}
}
