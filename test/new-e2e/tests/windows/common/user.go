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

// GetADGroupMembers returns the list of members of the given AD group
func GetADGroupMembers(host *components.RemoteHost, group string) ([]Identity, error) {
	cmd := fmt.Sprintf(`ConvertTo-JSON -InputObject @(Get-ADGroupMember -Identity "%s" | Foreach-Object {
		@{
			Name = $_.Name
			SID = $_.SID.Value
		}})`, group)
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var members []Identity
	err = json.Unmarshal([]byte(out), &members)
	if err != nil {
		return nil, err
	}
	return members, nil
}

// GetLocalGroupMembers returns the list of members of the given local group
func GetLocalGroupMembers(host *components.RemoteHost, group string) ([]Identity, error) {
	cmd := fmt.Sprintf(`ConvertTo-JSON -InputObject @(Get-LocalGroupMember -Name "%s" | Foreach-Object {
		@{
			Name = $_.Name
			SID = $_.SID.Value
		}})`, group)
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var members []Identity
	err = json.Unmarshal([]byte(out), &members)
	if err != nil {
		return nil, err
	}
	return members, nil
}

// GetUserRights returns a map of user rights to a list of users that have them
//
// https://learn.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/user-rights-assignment
func GetUserRights(host *components.RemoteHost) (map[string][]string, error) {
	outFile, err := GetTemporaryFile(host)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`secedit /export /areas USER_RIGHTS /cfg %s`, outFile)
	_, err = host.Execute(cmd)
	if err != nil {
		return nil, err
	}

	c, err := host.ReadFile(outFile)
	if err != nil {
		return nil, err
	}
	c, err = ConvertUTF16ToUTF8(c)
	if err != nil {
		return nil, err
	}
	content := string(c)

	result := make(map[string][]string)

	// The file is in INI syntax, Go doesn't have a built-in INI parser
	// but going line by line is sufficient for our needs
	for _, line := range strings.Split(content, "\r\n") {
		if strings.HasPrefix(line, "Se") {
			// example: SeDenyNetworkLogonRight = *S-1-5-18,ddagentuser
			parts := strings.Split(line, "=")
			if len(parts) != 2 {
				return nil, fmt.Errorf("unexpected line format: %s", line)
			}
			rightName := strings.TrimSpace(parts[0])
			users := strings.TrimSpace(parts[1])
			userList := strings.Split(users, ",")
			for i, user := range userList {
				user = strings.TrimSpace(user)
				// SIDs are given as *S-1-5-32-544
				user = strings.TrimLeft(user, "*")
				userList[i] = user
			}
			result[rightName] = userList
		}
	}
	return result, nil
}

// GetUserRightsForUser returns a list of user rights for the given user
func GetUserRightsForUser(host *components.RemoteHost, user string) ([]string, error) {
	sid, err := GetSIDForUser(host, user)
	if err != nil {
		return nil, err
	}
	rights, err := GetUserRights(host)
	if err != nil {
		return nil, err
	}
	var result []string
	var sidCache = make(map[string]string)
	sidCache[user] = sid
	for right, users := range rights {
		for _, u := range users {
			var s string
			if strings.HasPrefix(u, "S-1-") {
				s = u
			} else {
				// not a SID, look up the SID for the username
				var ok bool
				s, ok = sidCache[u]
				if !ok {
					s, err = GetSIDForUser(host, u)
					if err != nil {
						return nil, err
					}
					sidCache[u] = s
				}
			}
			// check if SID or username matches
			if strings.EqualFold(s, sid) || strings.EqualFold(u, user) {
				result = append(result, right)
				break
			}
		}
	}
	return result, nil
}

// RemoveLocalUser Removes a local user account
// NOTE: this does not remove the user profile, which without a reboot is probably locked by the system.
func RemoveLocalUser(host *components.RemoteHost, user string) error {
	cmd := fmt.Sprintf(`Remove-LocalUser -Name "%s"`, user)
	_, err := host.Execute(cmd)
	return err
}
