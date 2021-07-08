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

	previousEnforceStatus string
	currentEnforceStatus  string
}

// NewSELinuxResolver returns a new SELinux resolver
func NewSELinuxResolver() *SELinuxResolver {
	return &SELinuxResolver{}
}

// BeginNewResolveStep starts a new resolver step by converting current state to previous state
func (r *SELinuxResolver) BeginNewResolveStep() {
	r.Lock()
	defer r.Unlock()

	r.previousEnforceStatus = r.currentEnforceStatus
	r.currentEnforceStatus = ""
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
