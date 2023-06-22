// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validators

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"testing"
)

// Other test cases: array too long
// Too many fields

func TestIsAlwaysTrue(t *testing.T) {
	// TODO: Handle macros and variables and parentheses in chained boolean expressions

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
			want: false,
		},
		{
			name: "always true chained",
			args: args{
				ruleExpression: "exec.file.path =~ \"/**\" && open.file.name == \"*\" || exec.file.path != \"/bin/ls\"",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleToEval := eval.NewRule(tt.name, tt.args.ruleExpression, &eval.Opts{})

			got, err := IsAlwaysTrue(ruleToEval)

			if err != nil {
				t.Errorf("Error message is `%s`, wanted it to contain `%s`", err.Error(), tt.errMessage)
			}

			if got != tt.want {
				t.Errorf("IsAlwaysTrue() = %v, want %v", got, tt.want)
			}
		})
	}
}
