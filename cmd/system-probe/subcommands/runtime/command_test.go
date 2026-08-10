// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"bytes"
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
			check: func(_ *downloadPolicyCliParams, params core.BundleParams) {
				// Verify logger defaults
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "off", params.LogLevelFn(nil), "log level not matching")
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

func newMockRSClient(t *testing.T) secagent.SecurityModuleCmdClientWrapper {
	m := mocks.NewSecurityModuleCmdClientWrapper(t)
	m.On("GetRuleSetReport").Return(&api.GetRuleSetReportMessage{
		RuleSetReportMessage: &api.RuleSetReportMessage{
			Filters: &api.FilterReport{
				Approvers: []*api.ApproverReport{
					{
						EventType: "exec",
						Mode:      1,
						Approvers: nil,
					},
					{
						EventType: "open",
						Mode:      2,
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
		},
	}, nil)
	return m
}

// go test -v github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime --run="Test_checkPoliciesLoaded"
func Test_checkPoliciesLoaded(t *testing.T) {
	type args struct {
		args   *checkPoliciesCliParams
		client secagent.SecurityModuleCmdClientWrapper
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
	"approvers": {
		"open": {
			"mode": "deny",
			"approvers": {
				"open.file.path": [
					{
						"field": "open.file.path",
						"value": "/etc/gshadow",
						"type": "scalar",
						"mode": 0
					},
					{
						"field": "open.file.path",
						"value": "/etc/shadow",
						"type": "scalar",
						"mode": 0
					}
				],
				"open.flags": [
					{
						"field": "open.flags",
						"value": 64,
						"type": "scalar",
						"mode": 0
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

func Test_writeRuleCoverage(t *testing.T) {
	report := `{
		"total_paths": 6,
		"covered_paths": 1,
		"tracked_rules": 2,
		"untracked_rules": 0,
		"rules": [
			{
				"rule_id": "covered_rule",
				"event_type": "mkdir",
				"coverage": {
					"expression": "mkdir.file.path == \"/tmp/foo\"",
					"skeleton": "A",
					"total_paths": 2,
					"covered_paths": 2,
					"evaluations": 3,
					"leaves": [{"name": "A", "expression": "mkdir.file.path == \"/tmp/foo\"", "offset": 0, "length": 28, "true": 1, "false": 2}],
					"paths": [
						{"conditions": [{"leaf": "A", "value": false}], "result": false, "hits": 2},
						{"conditions": [{"leaf": "A", "value": true}], "result": true, "hits": 1}
					]
				}
			},
			{
				"rule_id": "partially_covered_rule",
				"event_type": "open",
				"coverage": {
					"expression": "open.file.path == \"/etc/passwd\" && process.uid == 0",
					"skeleton": "A && B",
					"total_paths": 4,
					"covered_paths": 1,
					"evaluations": 1,
					"leaves": [
						{"name": "A", "expression": "open.file.path == \"/etc/passwd\"", "offset": 0, "length": 30, "true": 1, "false": 0},
						{"name": "B", "expression": "process.uid == 0", "offset": 34, "length": 16, "true": 1, "false": 0}
					],
					"paths": [
						{"conditions": [{"leaf": "A", "value": false}], "result": false, "hits": 0},
						{"conditions": [{"leaf": "A", "value": true}, {"leaf": "B", "value": false}], "result": false, "hits": 0},
						{"conditions": [{"leaf": "A", "value": true}, {"leaf": "B", "value": true}], "result": true, "hits": 1}
					]
				}
			}
		]
	}`

	newClient := func(t *testing.T) secagent.SecurityModuleCmdClientWrapper {
		m := mocks.NewSecurityModuleCmdClientWrapper(t)
		m.On("DumpRuleCoverage", false).Return(&api.DumpRuleCoverageMessage{Report: report}, nil)
		return m
	}

	t.Run("summary", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, writeRuleCoverage(newClient(t), &ruleCoverageCliParams{}, &output))

		assert.Contains(t, output.String(), "Rule coverage: 1/6 paths (16.7%) over 2 rules")
		assert.Contains(t, output.String(), "covered_rule: 2/2 paths, 3 evaluation(s)")
		assert.Contains(t, output.String(), "partially_covered_rule: 1/4 paths, 1 evaluation(s)")
		assert.Contains(t, output.String(), "[ ]          0  A=true B=false => false")
		assert.Contains(t, output.String(), "[x]          1  A=true B=true  => true")
	})

	t.Run("uncovered only", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, writeRuleCoverage(newClient(t), &ruleCoverageCliParams{uncovered: true}, &output))

		assert.NotContains(t, output.String(), "covered_rule: 2/2 paths")
		assert.Contains(t, output.String(), "partially_covered_rule: 1/4 paths")
	})

	t.Run("single rule", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, writeRuleCoverage(newClient(t), &ruleCoverageCliParams{ruleID: "covered_rule"}, &output))

		assert.Contains(t, output.String(), "covered_rule: 2/2 paths")
		assert.NotContains(t, output.String(), "partially_covered_rule")
	})

	t.Run("json", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, writeRuleCoverage(newClient(t), &ruleCoverageCliParams{asJSON: true}, &output))

		assert.Equal(t, report+"\n", output.String())
	})
}

func TestDumpRuleCoverageCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "rule-coverage", "dump"},
		dumpRuleCoverage,
		func() {})
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

func TestDumpPoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "dump"},
		dumpLoadedPolicies,
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
