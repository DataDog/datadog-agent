// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// JoinUserDomain joins a user and domain into a single string
func JoinUserDomain(user string, domain string) string {
	if domain == "" {
		return user
	}
	return domain + "\\" + user
}

// GetSIDForUser returns the SID for the given user.
//
// user can be of the following forms
//   - username
//   - hostname\username
//   - domain\username
//   - username@domain
func GetSIDForUser(host *components.RemoteHost, user string) (string, error) {
	// strip leading .\ if present
	user = strings.TrimPrefix(user, ".\\")

	cmd := fmt.Sprintf(`(New-Object System.Security.Principal.NTAccount('%s')).Translate([System.Security.Principal.SecurityIdentifier]).Value.ToString()`, user)
	out, err := host.Execute(cmd)
	return strings.TrimSpace(out), err
}
