// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type secretsStatus struct {
	resolver *secretResolver
}

// Name returns the name of the component for status reporting
func (s secretsStatus) Name() string {
	return "Secrets"
}

// Section returns the section name for status reporting
func (s secretsStatus) Section() string {
	return "secrets"
}

func (s secretsStatus) populateStatus(stats map[string]interface{}) {
	r := s.resolver

	stats["enabled"] = r.enabled

	if !r.enabled {
		return
	}

	if r.backendCommand == "" {
		stats["message"] = "No secret_backend_command set: secrets feature is not enabled\n"
		return
	}

	stats["executable"] = r.backendCommand

	correctPermission := true
	permissionMsg := "OK, the executable has the correct permissions"
	err := checkRights(r.backendCommand, r.commandAllowGroupExec)
	if err != nil {
		correctPermission = false
		permissionMsg = fmt.Sprintf("error: %s", err)
	}
	stats["executable_correct_permissions"] = correctPermission
	stats["executable_permissions_message"] = permissionMsg

	handleMap := make(map[string][][]string)
	orderedHandles := make([]string, 0, len(r.origin))
	for handle := range r.origin {
		orderedHandles = append(orderedHandles, handle)
	}
	sort.Strings(orderedHandles)

	for _, handle := range orderedHandles {
		contexts := r.origin[handle]
		details := make([][]string, 0, len(contexts))
		for _, context := range contexts {
			details = append(details, []string{context.origin, strings.Join(context.path, "/")})
		}
		handleMap[handle] = details
	}
	stats["handles"] = handleMap
}

// JSON populates the status map
func (s secretsStatus) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

// Text renders the text output
func (s secretsStatus) Text(_ bool, buffer io.Writer) error {
	s.resolver.GetDebugInfo(buffer)
	return nil
}

// HTML renders the html output
func (s secretsStatus) HTML(_ bool, buffer io.Writer) error {
	s.resolver.GetDebugInfo(buffer)
	return nil
}
