package validators

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"testing"
)

// Other test cases: array too long
// Too many fields

func TestIsAlwaysTrue(t *testing.T) {
	// if there is a wildcard, check if there is a && that's not a wildcard

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
			name: "path wildcard in array",
			args: args{
				ruleExpression: "open.file.path in [\"/bin/pwd\", ~\"/**\", \"/etc/shadow\"]",
			},
			want: true,
		},
		{
			name: "pattern",
			args: args{
				ruleExpression: "open.file.path =~ ~\"/**\"",
			},
			want: true,
		},
		{
			name: "regex pattern",
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
			name: "always true and",
			args: args{
				ruleExpression: "exec.file.name != \"ls\" && exec.file.path =~ \"/**\"",
			},
			want: true,
		},
		{
			name: "not empty path",
			args: args{
				ruleExpression: "open.file.path != \"\"", // TODO: Need to implement check. Not allow empty string for path or name
			},
			want: true,
		},
		{
			name: "duration",
			args: args{
				ruleExpression: "process.created_at >= 0s", // Do not have event type, so not valid
			},
			want: true,
		},
		{
			name: "file path length",
			args: args{
				ruleExpression: "process.file.path.length >= 0s", // Do not have event type, so not valid
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
