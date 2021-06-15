// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"errors"
	"os/exec"
	"strings"
	"sync"
)

// SELinuxResolver resolved SELinux context
type SELinuxResolver struct {
	sync.RWMutex
	currentBoolValues map[string]bool
	pendingBoolValues map[string]bool

	previousEnforceStatus string
	currentEnforceStatus  string
}

// NewSELinuxResolver returns a new SELinux resolver
func NewSELinuxResolver() *SELinuxResolver {
	return &SELinuxResolver{
		currentBoolValues: make(map[string]bool),
		pendingBoolValues: make(map[string]bool),
	}
}

// flushPendingBools flushes currently pending bools so that their values can be retrieved through `GetCurrentBoolValue`
func (r *SELinuxResolver) flushPendingBools() {
	for k, v := range r.pendingBoolValues {
		r.currentBoolValues[k] = v
		delete(r.pendingBoolValues, k)
	}
}

// BeginNewResolveStep starts a new resolver step by converting current state to previous state
func (r *SELinuxResolver) BeginNewResolveStep() {
	r.Lock()
	defer r.Unlock()

	r.flushPendingBools()
	r.previousEnforceStatus = r.currentEnforceStatus
	r.currentEnforceStatus = ""
}

// GetCurrentBoolValue returns the current value of the provided SELinux boolean
func (r *SELinuxResolver) GetCurrentBoolValue(boolName string) (bool, error) {
	r.RLock()
	defer r.RUnlock()

	val, ok := r.currentBoolValues[boolName]
	if !ok {
		return false, errors.New("no current value")
	}
	return val, nil
}

// SetCurrentBoolValue sets the current value of the provided SELinux boolean
func (r *SELinuxResolver) SetCurrentBoolValue(boolName string, value bool) {
	r.Lock()
	defer r.Unlock()

	r.pendingBoolValues[boolName] = value
}

// ResolvePreviousEnforceStatus returns the previous SELinux enforcement status, one of "enforcing", "permissive", "disabled" or "" if it was not set
func (r *SELinuxResolver) ResolvePreviousEnforceStatus() string {
	r.RLock()
	defer r.RUnlock()

	return r.previousEnforceStatus
}

// ResolveCurrentEnforceStatus returns the current SELinux enforcement status, one of "enforcing", "permissive", "disabled"
func (r *SELinuxResolver) ResolveCurrentEnforceStatus() (string, error) {
	r.Lock()
	defer r.Unlock()

	if len(r.currentEnforceStatus) != 0 {
		return r.currentEnforceStatus, nil
	}

	output, err := exec.Command("getenforce").Output()
	if err != nil {
		return "", err
	}

	status := strings.ToLower(strings.TrimSpace(string(output)))
	switch status {
	case "enforcing", "permissive", "disabled":
		r.currentEnforceStatus = status
		return status, nil
	default:
		return "", errors.New("failed to parse getenforce output")
	}
}
