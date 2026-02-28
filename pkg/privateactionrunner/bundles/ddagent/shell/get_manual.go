// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package shell

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
)

// GetManualHandler handles requests for the safe shell manual/allowlist.
type GetManualHandler struct{}

// NewGetManualHandler creates a new GetManualHandler.
func NewGetManualHandler() *GetManualHandler {
	return &GetManualHandler{}
}

// GetManualOutputs defines the output contract for the getManual action.
type GetManualOutputs struct {
	AllowedCommands  map[string]verifier.CommandInfo `json:"allowedCommands"`
	BlockedBuiltins  []string                        `json:"blockedBuiltins"`
	DangerousEnvVars []string                        `json:"dangerousEnvVars"`
	AllowedFeatures  []string                        `json:"allowedFeatures"`
	BlockedFeatures  []string                        `json:"blockedFeatures"`
	Limits           map[string]string               `json:"limits"`
}

// Run returns the full safe shell manual.
func (h *GetManualHandler) Run(
	_ context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return &GetManualOutputs{
		AllowedCommands:  verifier.AllowedCommandsWithDescriptions(),
		BlockedBuiltins:  verifier.BlockedBuiltins(),
		DangerousEnvVars: verifier.DangerousEnvVars(),
		AllowedFeatures: []string{
			"pipes (|, &&, ||)",
			"for-in loops (for x in ...; do ...; done)",
			"glob expansion (*.log, /var/log/*.txt)",
			"for-loop variable expansion ($var in loop body)",
			"single-quoted and double-quoted strings (no expansion in either)",
			"negation (! command)",
		},
		BlockedFeatures: []string{
			"redirections (>, >>, <, heredocs)",
			"command substitution ($(cmd), backticks)",
			"process substitution (<(cmd), >(cmd))",
			"subshells ((cmd))",
			"function declarations",
			"background execution (&)",
			"coprocesses",
			"variable assignment (x=value)",
			"parameter expansion ($VAR, ${VAR:-default}) except for-loop variable",
			"arithmetic expansion ($((expr)))",
			"if/elif/else conditionals",
			"while/until loops",
			"case statements",
			"block commands ({ ...; })",
			"test commands ([[ ]], test, [)",
			"declare/local/export/readonly",
			"C-style for loops",
			"select statements",
			"let, time, coproc",
			"brace expansion",
			"extended glob patterns",
		},
		Limits: map[string]string{
			"defaultTimeout": "30s",
			"maxOutputBytes": "1048576",
		},
	}, nil
}
