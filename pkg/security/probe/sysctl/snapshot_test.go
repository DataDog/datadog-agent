// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSnapshotEvent_ToJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   Snapshot
		want    string
		wantErr bool
	}{
		{
			name:  "empty_snapshot",
			input: NewSnapshot(),
			want:  "{\"sysctl\":{}}",
		},
		{
			name: "full_snapshot",
			input: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
				Sys: map[string]interface{}{
					"kernel": map[string]interface{}{
						"security": map[string]interface{}{
							"lockdown": []string{"none", "[integrity]", "confidentiality"},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}},\"sys\":{\"kernel\":{\"security\":{\"lockdown\":[\"none\",\"[integrity]\",\"confidentiality\"]}}}}}",
		},
		{
			name: "proc_only",
			input: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}}}}",
		},
		{
			name: "sys_only",
			input: Snapshot{
				Sys: map[string]interface{}{
					"kernel": map[string]interface{}{
						"security": map[string]interface{}{
							"lockdown": []string{"none", "[integrity]", "confidentiality"},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"sys\":{\"kernel\":{\"security\":{\"lockdown\":[\"none\",\"[integrity]\",\"confidentiality\"]}}}}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SnapshotEvent{
				Sysctl: tt.input,
			}
			got, err := s.ToJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.JSONEqf(t, tt.want, string(got), "ToJSON() error")
		})
	}
}

func TestSnapshot_InsertSnapshotEntry(t *testing.T) {
	type args struct {
		file  string
		value string
	}
	tests := []struct {
		name     string
		snapshot Snapshot
		args     args
		output   string
	}{
		{
			name:     "empty_snapshot_insert_nothing",
			snapshot: NewSnapshot(),
			args:     args{},
			output:   "{}",
		},
		{
			name:     "empty_snapshot_insert_string_single_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "sys",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string_root_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/",
				value: "123",
			},
			output: "{\"proc\":{\"\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string_empty_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "",
				value: "123",
			},
			output: "{\"proc\":{\"\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string",
			snapshot: NewSnapshot(),
			args: args{
				file:  "sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_space",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123 ",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_tab",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123\t",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_newline",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123\n",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_insert_uuid_string",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"},\"yama\":{\"ptrace_scope\":\"2\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_insert_hello_string_spaces",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/hello/world",
				value: " 123 ",
			},
			output: "{\"proc\":{\"hello\":{\"world\":\"123\"},\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_override",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/sys/kernel/yama/ptrace_scope",
				value: "0",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"0\"}}}}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.snapshot.InsertSnapshotEntry(tt.snapshot.Proc, tt.args.file, tt.args.value)
			got, err := tt.snapshot.ToJSON()
			if err != nil {
				t.Errorf("InsertSnapshotEntry - ToJSON error: %v", err)
				return
			}
			assert.JSONEqf(t, string(got), tt.output, "InsertSnapshotEntry error")
		})
	}
}
