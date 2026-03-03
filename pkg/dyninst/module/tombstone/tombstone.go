// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package tombstone implements a simple mechanism to protect against repeated
// crashes when placing probes by writing a file to disk while loading a program
// and checking for the presence of the file on startup; if the file is present,
// no programs will be loaded for a while.
package tombstone

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// The tombstone file tracks whether we crashed while loading a program.

// FileContents represents the contents of a tombstone JSON file.
type FileContents struct {
	// AgentVersion is the version of the agent that wrote this file.
	AgentVersion string `json:"agent_version"`
	// ErrorNumber counts how many crashes have occurred since the file was
	// created.
	ErrorNumber int `json:"attempt_number"`
	// Timestamp is the time when the file was created, or when ErrorNumber was
	// last incremented.
	Timestamp time.Time `json:"timestamp"`
}

// WriteTombstoneFile creates or updates a tombstone file at the specified path.
func WriteTombstoneFile(filePath string, errorNumber int) error {
	contents := FileContents{
		AgentVersion: version.AgentVersion,
		ErrorNumber:  errorNumber,
		Timestamp:    time.Now(),
	}

	data, err := json.Marshal(contents)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// ReadTombstoneFile reads and unmarshals the tombstone file at the specified path.
func ReadTombstoneFile(filePath string) (*FileContents, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var contents FileContents
	if err := json.Unmarshal(data, &contents); err != nil {
		return nil, err
	}

	return &contents, nil
}

// Remove removes the tombstone file at the specified path.
func Remove(filePath string) error {
	if filePath == "" {
		return nil
	}

	err := os.Remove(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// WaitTestingKnobs groups testing knobs for WaitOutTombstone.
type WaitTestingKnobs struct {
	OnSleep       func()
	BackoffPolicy backoff.Policy
}

// WaitOutTombstone waits for a while if it looks like we're recovering from a
// crash that happened during program loading -- i.e. if we find a tombstone
// file.
//
// Note that, for simplicity, we don't take into account which probe caused the
// crash; we simply avoid placing any probes for a while.
//
// knobs, if not nil, should contain a single element.
func WaitOutTombstone(ctx context.Context, tombstoneFilePath string, knobs ...WaitTestingKnobs) {
	if tombstoneFilePath == "" {
		return
	}
	// Check if a tombstone file exists. If it does, we increment the
	// attempt number and block according to an exponential backoff.
	_, err := os.Stat(tombstoneFilePath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		log.Warnf("failed to read tombstone file: %s", err)
	}

	// Helper function to delete the tombstone file.
	deleteTombstone := func() {
		err := Remove(tombstoneFilePath)
		if err != nil {
			log.Warnf("failed to delete tombstone file %s: %s", tombstoneFilePath, err)
		}
	}

	contents, err := ReadTombstoneFile(tombstoneFilePath)
	if err != nil {
		log.Warnf("failed to read tombstone file %s: %s, deleting it", tombstoneFilePath, err)
		deleteTombstone()
		return
	}

	// If the agent version in the tombstone doesn't match the current agent
	// version, delete the tombstone and don't block. This prevents blocking
	// after an agent upgrade.
	if contents.AgentVersion != version.AgentVersion {
		log.Infof("tombstone file has different agent version (%s vs %s), ignoring it", contents.AgentVersion, version.AgentVersion)
		deleteTombstone()
		return
	}

	// Calculate backoff duration based on attempt number (exponential backoff).
	backoffPolicy := backoff.NewExpBackoffPolicy(
		2.0,   // minBackoffFactor
		60,    // baseBackoffTime (seconds)
		3600,  // maxBackoffTime (seconds)
		1,     // recoveryInterval
		false, // recoveryReset
	)

	var onSleep func()
	if len(knobs) > 0 {
		if len(knobs) > 1 {
			panic("WaitOutTombstone called with multiple knobs")
		}
		if knobs[0].BackoffPolicy != nil {
			backoffPolicy = knobs[0].BackoffPolicy
		}
		if knobs[0].OnSleep != nil {
			onSleep = knobs[0].OnSleep
		}
	}

	targetWaitDuration := backoffPolicy.GetBackoffDuration(contents.ErrorNumber)

	// Wait for the tombstone to expire.
	elapsedSinceTimestamp := time.Since(contents.Timestamp)
	remainingWait := targetWaitDuration - elapsedSinceTimestamp
	if onSleep != nil {
		// If we have a knob installed, make sure remainingWait is positive.
		remainingWait = targetWaitDuration
	}

	if remainingWait > 0 {
		log.Warnf("tombstone file detected (previous errors: %d), waiting %s before installing probes: %s", contents.ErrorNumber, remainingWait, tombstoneFilePath)
		if onSleep != nil {
			onSleep()
		}
		select {
		case <-time.After(remainingWait):
		case <-ctx.Done():
			log.Debugf("waiting for tombstone canceled: %s", context.Cause(ctx))
			return
		}
	} else {
		log.Infof("tombstone file detected (previous errors: %d), backoff period already elapsed: %s", contents.ErrorNumber, tombstoneFilePath)
	}

	// Increment error number and update timestamp. If we crash again, we'll
	// wait even longer.
	err = WriteTombstoneFile(tombstoneFilePath, contents.ErrorNumber+1)
	if err != nil {
		log.Warnf("failed to update tombstone file: %s", err)
		return
	}

	// Schedule the deletion of the tombstone file in one minute, if we're still
	// alive by then. Being alive in one minute is taken as an indication that
	// we were able to place all the active probes without crashing again.
	time.AfterFunc(time.Minute, func() { _ = Remove(tombstoneFilePath) })
}
