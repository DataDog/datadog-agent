// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestCleanReturnNames(t *testing.T) {
	makeVars := func(names ...string) []*ir.Variable {
		vars := make([]*ir.Variable, len(names))
		for i, name := range names {
			vars[i] = &ir.Variable{Name: name, Role: ir.VariableRoleReturn}
		}
		return vars
	}

	tests := []struct {
		name     string
		vars     []*ir.Variable
		expected []string
	}{
		{
			name:     "single unnamed return",
			vars:     makeVars("~r0"),
			expected: nil, // single returns are not transformed; @return is used directly
		},
		{
			name:     "single named return",
			vars:     makeVars("result"),
			expected: nil,
		},
		{
			name:     "multiple unnamed returns",
			vars:     makeVars("~r0", "~r1", "~r2"),
			expected: []string{"r0", "r1", "r2"},
		},
		{
			name:     "multiple named returns",
			vars:     makeVars("result", "result2"),
			expected: []string{"result", "result2"},
		},
		{
			name:     "mixed named and unnamed returns",
			vars:     makeVars("~r0", "result2", "~r2"),
			expected: []string{"r0", "result2", "r2"},
		},
		{
			name:     "conflict: user name collides with stripped name",
			vars:     makeVars("r0", "~r0"),
			expected: []string{"r0", "_r0"},
		},
		{
			name:     "conflict: double collision requires double underscore",
			vars:     makeVars("r0", "_r0", "~r0"),
			expected: []string{"r0", "_r0", "__r0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanReturnNames(tt.vars)
			if tt.expected == nil {
				require.Nil(t, got,
					"single return should return nil (no renaming needed)")
				return
			}
			require.Equal(t, len(tt.vars), len(got))
			for i, v := range tt.vars {
				require.Equal(t, tt.expected[i], got[v],
					"variable %q at index %d", v.Name, i)
			}
		})
	}
}
