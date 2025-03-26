// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/agent/mocks"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
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
				require.Equal(t, "SYS-PROBE", params.LoggerName(), "logger name not matching")
				require.Equal(t, "off", params.LogLevelFn(nil), "log level not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			[]*cobra.Command{DownloadPolicyCommand(&command.GlobalParams{})},
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
	}, nil)
	return m
}
