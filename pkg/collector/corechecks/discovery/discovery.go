// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package discovery provides the discovery check for reporting warnings from log discovery providers.
package discovery

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the discovery check
	CheckName = "discovery"

	refreshInterval = 1 * time.Minute
)

type warningType string

const (
	warningTypeLogFile warningType = "log_file"
)

const (
	errorCodePermissionDenied = "permission-denied"
	errorCodeGeneric          = "generic"
)

// Warning represents a structured warning from discovery providers
type Warning struct {
	Type        warningType `json:"type"`
	Version     int         `json:"version"`
	Resource    string      `json:"resource"`
	ErrorCode   string      `json:"error_code"`
	ErrorString string      `json:"error_string"`
	Message     string      `json:"message"`
}

// Error returns the formatted message as a string for the error interface
func (w *Warning) Error() string {
	jsonData, err := json.Marshal(w)
	if err != nil {
		return w.Message
	}

	return string(jsonData)
}

type warningCollector struct {
	mu       sync.RWMutex
	warnings map[string]Warning
}

var globalWarningCollector = &warningCollector{
	warnings: make(map[string]Warning),
}

func (wc *warningCollector) addWarning(logName string, warning Warning) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.warnings[logName] = warning
}

func (wc *warningCollector) removeWarning(logName string) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	delete(wc.warnings, logName)
}

func (wc *warningCollector) getWarningsAsErrors() []error {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	warnings := make([]error, 0, len(wc.warnings))
	for _, warning := range wc.warnings {
		warnings = append(warnings, &warning)
	}
	return warnings
}

// AddWarning is the global function that providers can call to add warnings
func AddWarning(logName string, err error, message string) {
	var errorCode string
	switch {
	case errors.Is(err, os.ErrPermission):
		errorCode = errorCodePermissionDenied
	default:
		errorCode = errorCodeGeneric
	}

	warning := Warning{
		Type:        warningTypeLogFile,
		Version:     1,
		Resource:    logName,
		ErrorCode:   errorCode,
		ErrorString: err.Error(),
		Message:     message,
	}

	globalWarningCollector.addWarning(logName, warning)
}

// RemoveWarning is the global function that providers can call to remove warnings
func RemoveWarning(resource string) {
	globalWarningCollector.removeWarning(resource)
}

// clearAllWarnings clears all warnings (mainly for testing purposes)
func clearAllWarnings() {
	globalWarningCollector.mu.Lock()
	defer globalWarningCollector.mu.Unlock()
	globalWarningCollector.warnings = make(map[string]Warning)
}

// Check implements the discovery check for reporting warnings from autodiscovery providers
type Check struct {
	corechecks.CheckBase
	warnings []error
}

// Factory returns a factory function for creating new discovery check instances
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck()
	})
}

func newCheck() *Check {
	return &Check{
		CheckBase: corechecks.NewCheckBase(CheckName),
		warnings:  []error{},
	}
}

// Configure configures the discovery check with the provided settings
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, instanceConfig, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, instanceConfig, source); err != nil {
		return err
	}
	return nil
}

// Run executes the discovery check. This does nothing since the only purpose of
// this check is to report warnings from other sources.
func (c *Check) Run() error {
	return nil
}

// GetWarnings returns the collected warnings
func (c *Check) GetWarnings() []error {
	return globalWarningCollector.getWarningsAsErrors()
}

// Interval returns the refresh interval for the discovery check
func (c *Check) Interval() time.Duration {
	return refreshInterval
}
