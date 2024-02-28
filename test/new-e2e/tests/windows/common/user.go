// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Identity contains the name and SID of an identity (user or group)
type Identity struct {
	Name string
	SID  string
}

// GetName returns the name of the identity
func (i Identity) GetName() string {
	return i.Name
}

// GetSID returns the SID of the identity
func (i Identity) GetSID() string {
	return i.SID
}

// SecurityIdentifier is an interface for objects that have a name and SID
type SecurityIdentifier interface {
	GetName() string
	GetSID() string
}

// LocalGroup contains select properties of a local group
//
// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.localaccounts/get-localgroup
type LocalGroup struct {
	Identity
}

// ADGroup contains select properties of an AD group
//
// https://learn.microsoft.com/en-us/powershell/module/activedirectory/get-adgroup
type ADGroup struct {
	DistinguishedName string
	Identity
}

// MakeDownLevelLogonName joins a user and domain into a single string, e.g. DOMAIN\user
//
// domain is converted to NetBIOS format per the MSDN definition.
//
// If domain is empty then the user is returned as-is. Use caution in this case as the isolated name may be ambiguous.
//
// https://learn.microsoft.com/en-us/windows/win32/secauthn/user-name-formats#down-level-logon-name
func MakeDownLevelLogonName(domain string, user string) string {
	if domain == "" {
		return user
	}
	domain = NameToNetBIOSName(domain)
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
	// NTAccount does not support .\username syntax
	user, err := DotSlashNameToLogonName(host, user)
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf(`(New-Object System.Security.Principal.NTAccount('%s')).Translate([System.Security.Principal.SecurityIdentifier]).Value.ToString()`, user)
	out, err := host.Execute(cmd)
	return strings.TrimSpace(out), err
}

// DotSlashNameToLogonName converts a .\username to a hostname\username.
//
// Simply stripping the .\ prefix is not sufficient because isolated named are ambiguous
// and may resolve to a domain account rather than a local account.
//
// SCM uses .\ to specify the local machine when returning a local service account name.
func DotSlashNameToLogonName(host *components.RemoteHost, user string) (string, error) {
	if !strings.HasPrefix(user, ".\\") {
		return user, nil
	}
	hostname, err := GetHostname(host)
	if err != nil {
		return "", err
	}
	user = strings.TrimPrefix(user, ".\\")
	return MakeDownLevelLogonName(hostname, user), nil
}

// GetADUserGroupMembership returns the list of AD groups that the user is a member of
func GetADUserGroupMembership(host *components.RemoteHost, user string) ([]*ADGroup, error) {
	cmd := fmt.Sprintf(`(Get-ADUser "%s" -Properties MemberOf).MemberOf | Get-ADGroup | Foreach-Object {
		@{
			DistinguishedName=$_.DistinguishedName
			Name = $_.Name
			SID = $_.SID.Value
		}} | ConvertTo-JSON`, user)
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var groups []*ADGroup
	err = json.Unmarshal([]byte(out), &groups)
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// GetLocalUserGroupMembership returns the list of local groups that the user is a member of
func GetLocalUserGroupMembership(host *components.RemoteHost, user string) ([]*LocalGroup, error) {
	sid, err := GetSIDForUser(host, user)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`Get-LocalGroup | Where-Object { "%s" -in ($_ | Get-LocalGroupMember | Select-Object -ExpandProperty SID)} | Foreach-Object {
		@{
			Name = $_.Name
			SID = $_.SID.Value
		}} | ConvertTo-JSON`, sid)
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var groups []*LocalGroup
	err = json.Unmarshal([]byte(out), &groups)
	if err != nil {
		return nil, err
	}
	return groups, nil
}
