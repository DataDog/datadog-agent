// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package validators holds validators related files
package validators

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// go test -v github.com/DataDog/datadog-agent/pkg/security/secl/validators --run=TestHasBareWildcardInField
// These test cases were originally written for an AlwaysTrue rule check. A more complex AlwaysTrue rule check is currently tabled in favor of a more naive bare wildcard check.
func TestHasBareWildcardInField(t *testing.T) {
	type args struct {
		ruleExpression string
	}
	tests := []struct {
		name       string
		args       args
		want       bool
		errMessage string
	}{
		{
			name: "valid wildcard",
			args: args{
				ruleExpression: "open.file.name in [~\"*.sh\", ~\"*.c\", ~\"*.so\", ~\"*.ko\"]",
			},
			want: false,
		},
		{
			name: "valid wildcard",
			args: args{
				ruleExpression: "chmod.file.path in [ ~\"/var/spool/cron/**\", ~\"/etc/cron.*/**\", ~\"/etc/crontab\" ]",
			},
			want: false,
		},
		{
			name: "root path with wildcard",
			args: args{
				ruleExpression: "open.file.path =~ \"/**\"",
			},
			want: true,
		},
		{
			name: "root path wildcard pattern",
			args: args{
				ruleExpression: "open.file.path =~ ~\"/**\"",
			},
			want: true,
		},
		{
			name: "bare wildcard",
			args: args{
				ruleExpression: "exec.file.name == \"*\"",
			},
			want: true,
		},
		{
			name: "bare wildcard in array",
			args: args{
				ruleExpression: "exec.file.name in [\"pwd\", \"*\", \"ls\"]",
			},
			want: true,
		},
		{
			name: "root path wildcard in array",
			args: args{
				ruleExpression: "open.file.path in [\"/bin/pwd\", ~\"/**\", \"/etc/shadow\"]",
			},
			want: true,
		},
		{
			name: "bare wildcard regex",
			args: args{
				ruleExpression: "dns.question.name =~ r\".*\"", // matches any character (except for line terminators) >= 0 times
			},
			want: true,
		},
		{
			name: "always true or",
			args: args{
				ruleExpression: "exec.file.path =~ \"/**\" || exec.file.name == \"ls\"",
			},
			want: true,
		},
		{
			name: "not always true chained",
			args: args{
				ruleExpression: "exec.file.path =~ \"/**\" && exec.file.name != \"ls\" || open.file.name == \"myfile.txt\"",
			},
			want: true,
		},
		{
			name: "always true chained",
			args: args{
				ruleExpression: "exec.file.path =~ \"/**\" && open.file.name == \"*\" || exec.file.path != \"/bin/ls\"",
			},
			want: true,
		},
		{
			name: "parentheses",
			args: args{
				ruleExpression: "exec.file.path =~ \"/**\" && (exec.file.name != \"ls\" || exec.file.name == \"*\")",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleToEval := eval.NewRule(tt.name, tt.args.ruleExpression, &eval.Opts{})

			got, err := HasBareWildcardInField(ruleToEval)

			if err != nil {
				t.Errorf("Error message is `%s`, wanted it to contain `%s`", err.Error(), tt.errMessage)
			}

			if got != tt.want {
				t.Errorf("HasBareWildcardInField() = %v, want %v", got, tt.want)
			}
		})
	}
}
