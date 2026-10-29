// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestValidateJournalQueryAcceptsBoundedScopes(t *testing.T) {
	require.NoError(t, validateJournalQuery(builtins.JournalQuery{Kernel: true, MaxEntries: builtins.MaxJournalQueryEntries}))
	require.NoError(t, validateJournalQuery(builtins.JournalQuery{Units: []string{"api.service"}, MaxEntries: 1}))
}

func TestValidateJournalQueryRejectsInvalidScopes(t *testing.T) {
	tooManyUnits := make([]string, builtins.MaxJournalQueryUnits+1)
	for index := range tooManyUnits {
		tooManyUnits[index] = "api.service"
	}
	tests := []struct {
		name  string
		query builtins.JournalQuery
		match string
	}{
		{name: "negative limit", query: builtins.JournalQuery{Kernel: true, MaxEntries: -1}, match: "entry limit"},
		{name: "large limit", query: builtins.JournalQuery{Kernel: true, MaxEntries: builtins.MaxJournalQueryEntries + 1}, match: "entry limit"},
		{name: "missing scope", query: builtins.JournalQuery{MaxEntries: 1}, match: "requires a kernel or unit scope"},
		{name: "mixed scopes", query: builtins.JournalQuery{Kernel: true, Units: []string{"api.service"}, MaxEntries: 1}, match: "cannot combine"},
		{name: "too many units", query: builtins.JournalQuery{Units: tooManyUnits, MaxEntries: 1}, match: "too many units"},
		{name: "empty unit", query: builtins.JournalQuery{Units: []string{""}, MaxEntries: 1}, match: "invalid unit name"},
		{name: "long unit", query: builtins.JournalQuery{Units: []string{strings.Repeat("a", 257)}, MaxEntries: 1}, match: "invalid unit name"},
		{name: "nul unit", query: builtins.JournalQuery{Units: []string{"api\x00.service"}, MaxEntries: 1}, match: "invalid unit name"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateJournalQuery(test.query)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}
