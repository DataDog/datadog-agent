// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"bytes"
	"math"
	"testing"

	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/agent/mocks"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDownloadCommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *downloadPolicyCliParams, params core.BundleParams)
	}{
		{
			name:     "runtime download",
			cliInput: []string{"download"},
			check: func(cliParams *downloadPolicyCliParams, params core.BundleParams) {
				// Verify logger defaults
				require.Equal(t, "SYS-PROBE", params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "log level not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			downloadPolicyCommands(&command.GlobalParams{}),
			test.cliInput,
			downloadPolicy,
			test.check,
		)
	}
}

func newMockRSClient(t *testing.T) secagent.SecurityModuleClientWrapper {
	m := mocks.NewSecurityModuleClientWrapper(t)
	m.On("GetRuleSetReport").Return(&api.GetRuleSetReportResultMessage{
		RuleSetReportMessage: &api.RuleSetReportMessage{
			Policies: []*api.EventTypePolicy{
				{
					EventType: "exec",
					Mode:      1,
					Flags:     math.MaxUint8,
					Approvers: nil,
				},
				{
					EventType: "open",
					Mode:      2,
					Flags:     math.MaxUint8,
					Approvers: &api.Approvers{
						ApproverDetails: []*api.ApproverDetails{
							{
								Field: "open.file.path",
								Value: "/etc/gshadow",
								Type:  1,
							},
							{
								Field: "open.file.path",
								Value: "/etc/shadow",
								Type:  1,
							},
							{
								Field: "open.flags",
								Value: "64",
								Type:  1,
							},
						},
					},
				},
			},
		},
	}, nil)
	return m
}

// go test -v github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime --run="Test_checkPoliciesLoaded"
func Test_checkPoliciesLoaded(t *testing.T) {
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
			"Flags": [
				"basename",
				"flags",
				"mode"
			],
			"Approvers": null
		},
		"open": {
			"Mode": "deny",
			"Flags": [
				"basename",
				"flags",
				"mode"
			],
			"Approvers": {
				"open.file.path": [
					{
						"Field": "open.file.path",
						"Value": "/etc/gshadow",
						"Type": "scalar"
					},
					{
						"Field": "open.file.path",
						"Value": "/etc/shadow",
						"Type": "scalar"
					}
				],
				"open.flags": [
					{
						"Field": "open.flags",
						"Value": 64,
						"Type": "scalar"
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

func TestDumpProcessCacheCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "process-cache", "dump"},
		dumpProcessCache,
		func() {})
}

func TestDumpNetworkNamespaceCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "network-namespace", "dump"},
		dumpNetworkNamespace,
		func() {})
}

func TestDumpDiscardersCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "discarders", "dump"},
		dumpDiscarders,
		func() {})
}

func TestEvalCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "eval", "--rule-id=10", "--event-file=file"},
		evalRule,
		func() {})
}

func TestCheckPoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "check"},
		checkPolicies,
		func() {})
}

func TestReloadRuntimePoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "reload"},
		reloadRuntimePolicies,
		func() {})
}

func TestRunRuntimeSelfTestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "self-test"},
		runRuntimeSelfTest,
		func() {})
}
