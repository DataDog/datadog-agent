// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package versa

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
)

func TestFilterOrganizations(t *testing.T) {
	tts := []struct {
		description  string
		inputOrgs    []client.Organization
		includedOrgs []string
		excludedOrgs []string
		expectedOrgs map[string]struct{}
	}{
		{
			description: "Nil included and excluded organizations",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: nil,
			excludedOrgs: nil,
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Empty included and excluded organizations",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Input and included organizations do not intersect",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org4", "org5"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{},
		},
		{
			description: "Included organizations include some of the input",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org4", "org2", "org1"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
			},
		},
		{
			description: "Included organizations and input match",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org1", "org2", "org3"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Test Case Insensitive Matching",
			inputOrgs: []client.Organization{
				{Name: "ORG1"},
				{Name: "Org2"},
				{Name: "orG3"},
			},
			includedOrgs: []string{"Org1", "ORG2", "oRG3"},
			excludedOrgs: []string{"OrG3"},
			expectedOrgs: map[string]struct{}{
				"ORG1": {},
				"Org2": {},
				// "orG3": {}, // This should be excluded because it's on the included and excluded list
			},
		},
		{
			description: "Excluded organizations match input test case insensitivity",
			inputOrgs: []client.Organization{
				{Name: "ORG1"},
				{Name: "Org2"},
				{Name: "orG3"},
			},
			includedOrgs: []string{},
			excludedOrgs: []string{"ORG2", "Org1"},
			expectedOrgs: map[string]struct{}{
				"orG3": {},
			},
		},
		{
			description: "Both included and excluded organizations match input",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
				{Name: "org4"},
			},
			includedOrgs: []string{"org1", "org2", "org3", "org4"},
			excludedOrgs: []string{"org2", "org3"},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org4": {},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualOrgs := filterOrganizations(test.inputOrgs, test.includedOrgs, test.excludedOrgs)
			if len(actualOrgs) != len(test.expectedOrgs) {
				t.Errorf("Unexpected number of organizations: expected %d: %v, got %d: %v", len(test.expectedOrgs), test.expectedOrgs, len(actualOrgs), actualOrgs)
			}
			actualOrgsSet := make(map[string]struct{})
			for _, org := range actualOrgs {
				actualOrgsSet[org.Name] = struct{}{}
			}
			for expectedOrg := range test.expectedOrgs {
				if _, ok := actualOrgsSet[expectedOrg]; !ok {
					t.Errorf("Expected organization %s not found in actual organizations", expectedOrg)
				}
			}
		})
	}
}
