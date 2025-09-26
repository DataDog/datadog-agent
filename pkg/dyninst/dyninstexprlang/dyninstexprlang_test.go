// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninstexprlang

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestCollectSegmentVariables(t *testing.T) {
	testCases := []struct {
		name          string
		msg           string
		subprogram    *ir.Subprogram
		wantVars      []ir.Variable
		wantSupported bool
	}{
		{
			name: "simple ref to parameter",
			msg:  `{"ref": "s"}`,
			subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{{Name: "s", IsParameter: true}},
			},
			wantVars:      []ir.Variable{{Name: "s"}},
			wantSupported: true,
		},
		{
			name: "ref to non-existent parameter",
			msg:  `{"ref": "x"}`,
			subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{{Name: "s", IsParameter: true}},
			},
			wantVars:      []ir.Variable{{Name: "x"}},
			wantSupported: false,
		},
		{
			name: "empty ref value",
			msg:  `{"ref": ""}`,
			subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{{Name: "s", IsParameter: true}},
			},
			wantVars:      nil,
			wantSupported: false,
		},
		{
			name: "unsupported instruction",
			msg:  `{"foo": "bar"}`,
			subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{{Name: "s", IsParameter: true}},
			},
			wantVars:      nil,
			wantSupported: false,
		},
		{
			name: "empty AST",
			msg:  `{}`,
			subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{{Name: "s", IsParameter: true}},
			},
			wantVars:      nil,
			wantSupported: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			variables := CollectSegmentVariables(json.RawMessage(tc.msg), tc.subprogram)
			require.Equal(t, tc.wantVars, variables)
			supported := ExpressionIsSupported(json.RawMessage(tc.msg), tc.subprogram)
			require.Equal(t, tc.wantSupported, supported)
		})
	}
}
