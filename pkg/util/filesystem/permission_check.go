// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

// CheckOwnerIsTrusted verifies that path is owned by a trusted user:
// root or dd-agent (nix), Administrators/SYSTEM/dd-agent (Windows).
func (p *Permission) CheckOwnerIsTrusted(path string) error {
	return p.checkOwner(path)
}

// CheckOwnerAndPermissionsAreRestricted verifies that path satisfies the agent's security requirements:
//
//   - Owner: must be root or dd-agent (nix) - or Administrators, SYSTEM, or dd-agent (Windows).
//   - Permissions: group and others must have no access rights.
func (p *Permission) CheckOwnerAndPermissionsAreRestricted(path string) error {
	if err := p.checkOwner(path); err != nil {
		return err
	}
	return CheckRights(path, false)
}
