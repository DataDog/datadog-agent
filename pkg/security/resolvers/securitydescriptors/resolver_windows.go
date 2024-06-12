// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package securitydescriptors holds security descriptors related files
package securitydescriptors

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
)

// Resolver defines a resolver
type Resolver struct {
	userGroupResolver *usergroup.Resolver
}

// NewResolver returns a new process resolver
func NewResolver() (*Resolver, error) {

	userGroupResolver, err := usergroup.NewResolver()
	if err != nil {
		return nil, err
	}
	r := &Resolver{
		userGroupResolver: userGroupResolver,
	}
	return r, nil
}

// Map of access masks initials to their human-readable names
var accessMaskMap = map[string]string{
	"GA": "Generic All (Full Control)",
	"GR": "Generic Read",
	"GW": "Generic Write",
	"GX": "Generic Execute",
	"RC": "Read Control",
	"SD": "Delete",
	"WD": "Write DAC",
	"WO": "Write Owner",
	"CC": "Create Child",
	"DC": "Delete Child",
	"LC": "List Child",
	"SW": "Self Write",
	"RP": "Read Property",
	"WP": "Write Property",
	"LO": "List Object",
	"DT": "Delete Tree",
	"CR": "Control Access",
	"FA": "File All Access",
	"FR": "File Read",
	"FW": "File Write",
	"FX": "File Execute",
	"KA": "Key All Access",
	"KR": "Key Read",
	"KW": "Key Write",
	"KX": "Key Execute",
}

// Translate ACE types to a human-readable format
func translateAceType(aceType string) string {
	switch aceType {
	case "A":
		return "Allow"
	case "D":
		return "Deny"
	case "AU":
		return "Audit"
	case "AL":
		return "Alarm"
	default:
		return "Unknown"
	}
}

// Convert the access mask from string to a human-readable string
func accessMaskToString(mask string) string {
	var rights []string

	for i := 0; i < len(mask)+1; i += 2 {
		combined := mask[i : i+2]
		if right, ok := accessMaskMap[combined]; ok {
			rights = append(rights, right)
		}
	}

	if len(rights) == 0 {
		return fmt.Sprintf("Custom (%s)", mask)
	}

	return strings.Join(rights, ", ")
}

// GetHumanReadableSD parse SDDL string to extract and translate the owner, group, and DACL
func (resolver *Resolver) GetHumanReadableSD(sddl string) (string, error) {
	var builder strings.Builder

	// Extract the owner and group SIDs
	owner, group := extractOwnerGroup(sddl)
	fmt.Println("---------------IN GetHumanReadableSD, owner", owner)
	fmt.Println("---------------IN GetHumanReadableSD, group", group)
	if owner != "" {
		ownerName := resolver.userGroupResolver.GetUser(owner)
		if ownerName == "" {
			ownerName = owner // Fallback to SID string if account lookup fails
		}
		builder.WriteString(fmt.Sprintf("Owner: %s\n", ownerName))
	}
	if group != "" {
		groupName := resolver.userGroupResolver.GetUser(group)
		if groupName == "" {
			groupName = group // Fallback to SID string if account lookup fails
		}
		builder.WriteString(fmt.Sprintf("Group: %s\n", groupName))
	}

	// Use regex to find all ACEs
	re := regexp.MustCompile(`\(([^\)]+)\)`)
	matches := re.FindAllStringSubmatch(sddl, -1)
	if matches == nil {
		return "", fmt.Errorf("no ACEs found in DACL")
	}

	builder.WriteString("DACL:\n")
	for _, match := range matches {
		if len(match) != 2 {
			return "", fmt.Errorf("invalid ACE format")
		}
		ace := match[1]
		fields := strings.Split(ace, ";")
		if len(fields) != 6 {
			return "", fmt.Errorf("invalid ACE format")
		}

		aceType := fields[0]
		permissions := fields[2]
		trustee := fields[5]
		fmt.Println("---------------IN GetHumanReadableSD, aceType", aceType)
		fmt.Println("---------------IN GetHumanReadableSD, permissions", permissions)
		fmt.Println("---------------IN GetHumanReadableSD, trustee", trustee)

		translatedType := translateAceType(aceType)
		translatedPermissions := accessMaskToString(permissions)

		accountName := resolver.userGroupResolver.GetUser(trustee)
		if accountName == "" {
			accountName = trustee // Fallback to SID string if account lookup fails
		}

		builder.WriteString(fmt.Sprintf("  - %s\n", translatedType))
		builder.WriteString(fmt.Sprintf("    Permissions: %s\n", translatedPermissions))
		builder.WriteString(fmt.Sprintf("    Trustee: %s\n", accountName))

	}

	builder.WriteString(fmt.Sprintf("    Sddl: %s\n", sddl))

	return builder.String(), nil
}

// Extract owner and group SIDs from the SDDL string
func extractOwnerGroup(sddl string) (string, string) {
	owner := ""
	group := ""

	if strings.Contains(sddl, "O:") {
		parts := strings.Split(sddl, "O:")
		if len(parts) > 1 {
			ownerPart := parts[1]
			endIndex := strings.IndexAny(ownerPart, "G:DS")
			if endIndex == -1 {
				owner = ownerPart
			} else {
				owner = ownerPart[:endIndex]
			}
		}
	}

	if strings.Contains(sddl, "G:") {
		parts := strings.Split(sddl, "G:")
		if len(parts) > 1 {
			groupPart := parts[1]
			endIndex := strings.IndexAny(groupPart, "ODS")
			if endIndex == -1 {
				group = groupPart
			} else {
				group = groupPart[:endIndex]
			}
		}
	}

	return owner, group
}
