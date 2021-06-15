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
}

// NewSELinuxResolver returns a new SELinux resolver
func NewSELinuxResolver() *SELinuxResolver {
	return &SELinuxResolver{
		currentBoolValues: make(map[string]bool),
		pendingBoolValues: make(map[string]bool),
	}
}

// FlushPendingBools flushes currently pending bools so that their values can be retrived through `GetCurrentBoolValue`
func (r *SELinuxResolver) FlushPendingBools() {
	r.Lock()
	defer r.Unlock()

	for k, v := range r.pendingBoolValues {
		r.currentBoolValues[k] = v
	}

	for k := range r.pendingBoolValues {
		delete(r.pendingBoolValues, k)
	}
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

// ResolveBoolValueChange resolves the change state of boolean value for SELinux events, while updating the old values map
func (r *SELinuxResolver) ResolveBoolValueChange(boolName string, newValue bool) bool {
	currentValue, err := r.GetCurrentBoolValue(boolName)
	if err != nil {
		return true // if error, let's assume the value has changed
	}
	return currentValue != newValue
}

// GetCurrentEnforceStatus returns the current SELinux enforcement status, one of "enforcing", "permissive", "disabled"
func (r *SELinuxResolver) GetCurrentEnforceStatus() (string, error) {
	output, err := exec.Command("getenforce").Output()
	if err != nil {
		return "", err
	}

	status := strings.ToLower(strings.TrimSpace(string(output)))
	switch status {
	case "enforcing", "permissive", "disabled":
		return status, nil
	default:
		return "", errors.New("failed to parse getenforce output")
	}
}
