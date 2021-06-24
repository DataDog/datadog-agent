// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package module

import (
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	Enabled         bool
	waitingForEvent bool
	EventChan       chan eval.Event
}

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(enabled bool) *SelfTester {
	return &SelfTester{
		Enabled:         enabled,
		waitingForEvent: false,
		EventChan:       make(chan eval.Event),
	}
}

// BeginWaitingForEvent passes the tester in the waiting for event state
func (t *SelfTester) BeginWaitingForEvent() {
	t.waitingForEvent = true
}

// EndWaitingForEvent exits the waiting for event state
func (t *SelfTester) EndWaitingForEvent() {
	t.waitingForEvent = false
}

// SendEventIfExpecting sends an event to the tester
func (t *SelfTester) SendEventIfExpecting(event eval.Event) {
	if t.Enabled && t.waitingForEvent {
		t.EventChan <- event
	}
}

func (t *SelfTester) expectEvent(predicate func(eval.Event) (bool, error)) error {
	timer := time.After(10 * time.Second)
	for {
		select {
		case event := <-t.EventChan:
			ok, err := predicate(event)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}
		case <-timer:
			return errors.New("failed to receive expected event")
		}
	}
}

func (t *SelfTester) selfTestOpen(targetFilePath string) error {
	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("touch", targetFilePath)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running touch: %v", err)
		return err
	}

	return t.expectEvent(func(event eval.Event) (bool, error) {
		eventOpenFilePath, err := event.GetFieldValue("open.file.path")
		if err != nil {
			return false, errors.Wrap(err, "failed to extract open file path from event")
		}
		return eventOpenFilePath == targetFilePath, nil
	})
}

func (t *SelfTester) selfTestChmod(targetFilePath string) error {
	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("chmod", "777", targetFilePath)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chmod: %v", err)
		return err
	}

	return t.expectEvent(func(event eval.Event) (bool, error) {
		eventOpenFilePath, err := event.GetFieldValue("chmod.file.path")
		if err != nil {
			return false, errors.Wrap(err, "failed to extract chmod file path from event")
		}
		return eventOpenFilePath == targetFilePath, nil
	})
}
