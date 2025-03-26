// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCheckPoliciesLoaded(t *testing.T) {
	type args struct {
		args   *checkPoliciesCliParams
		client secagent.SecurityModuleClientWrapper
	}
	tests := []struct {
		name               string
		args               args
		wantErr            bool
		reportFromSysProbe string
	}{
		{
			name:    "basic",
			wantErr: false,
			args: args{
				args:   &checkPoliciesCliParams{evaluateAllPolicySources: true},
				client: newMockRSClient(t),
			},
			reportFromSysProbe: `{
	"Policies": {
		"exec": {
			"Mode": "accept",
			"Approvers": null
		},
		"open": {
			"Mode": "deny",
			"Approvers": {
				"open.file.path": [
					{
						"Field": "open.file.path",
						"Value": "/etc/gshadow",
						"Type": "scalar",
						"Mode": 0
					},
					{
						"Field": "open.file.path",
						"Value": "/etc/shadow",
						"Type": "scalar",
						"Mode": 0
					}
				],
				"open.flags": [
					{
						"Field": "open.flags",
						"Value": 64,
						"Type": "scalar",
						"Mode": 0
					}
				]
			}
		}
	}
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := checkPoliciesLoaded(tt.args.client, &output)

			if (err != nil) != tt.wantErr {
				t.Errorf("checkPolicies() error = %v, wantErr %v", err, tt.wantErr)
			}

			assert.Equal(t, tt.reportFromSysProbe, output.String())
		})
	}
}

func TestCheckPoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{testRuntimeCommand(Command(&command.GlobalParams{}))},
		[]string{"runtime", "policy", "check"},
		CheckPolicies,
		func() {})
}
