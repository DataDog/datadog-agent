// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScrubCommandText covers the common shapes of secrets we expect to see
// in a raw command line before it is attached to the rshell.run.command
// telemetry tag. Each case documents the real-world tool/pattern it mimics.
func TestScrubCommandText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "long flag with equals value",
			in:   "curl --password=hunter2 https://example.com",
			want: "curl --password=******** https://example.com",
		},
		{
			name: "long flag with space value",
			in:   "curl --token ghp_abcdEFGH12345678 https://example.com",
			want: "curl --token ******** https://example.com",
		},
		{
			name: "long flag with quoted value containing spaces",
			in:   `mycmd --password="my secret pass" --verbose`,
			want: `mycmd --password="********" --verbose`,
		},
		{
			name: "long flag with single-quoted value",
			in:   "mycmd --secret 'top secret value' --verbose",
			want: "mycmd --secret '********' --verbose",
		},
		{
			name: "bare env assignment prefix",
			in:   "API_KEY=abcdef123456 ./run.sh",
			want: "API_KEY=******** ./run.sh",
		},
		{
			name: "export env assignment",
			in:   "export DB_PASSWORD=hunter2",
			want: "export DB_PASSWORD=********",
		},
		{
			name: "aws-style compound env var name",
			in:   "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY aws s3 ls",
			want: "AWS_SECRET_ACCESS_KEY=******** aws s3 ls",
		},
		{
			name: "authorization bearer header",
			in:   `curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig" https://api.example.com`,
			want: `curl -H "Authorization: Bearer ********" https://api.example.com`,
		},
		{
			name: "authorization basic header",
			in:   `curl -H "Authorization: Basic dXNlcjpwYXNz" https://api.example.com`,
			want: `curl -H "Authorization: Basic ********" https://api.example.com`,
		},
		{
			name: "curl basic auth flag user colon pass",
			in:   "curl -u admin:hunter2 https://example.com",
			want: "curl -u ******** https://example.com",
		},
		{
			name: "url embedded credentials",
			in:   "curl https://admin:hunter2@example.com/path",
			want: "curl https://admin:********@example.com/path",
		},
		{
			name: "git clone with token in url",
			in:   "git clone https://x-access-token:ghp_abcdEFGH12345678@github.com/org/repo.git",
			want: "git clone https://x-access-token:********@github.com/org/repo.git",
		},
		{
			name: "aws access key id literal",
			in:   "echo AKIAIOSFODNN7EXAMPLE",
			want: "echo ********",
		},
		{
			name: "bare jwt looking token",
			in:   "echo eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			want: "echo ********",
		},
		{
			name: "nested key=value inside another flag value",
			in:   "kubectl create secret generic db --from-literal=password=hunter2",
			want: "kubectl create secret generic db --from-literal=password=********",
		},
		{
			name: "no secrets present",
			in:   "ls -la /var/log && echo done",
			want: "ls -la /var/log && echo done",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "credential keyword",
			in:   "mycmd --credential=s3cr3t123",
			want: "mycmd --credential=********",
		},
		{
			name: "session id keyword",
			in:   "mycmd --session-id=abc123def456",
			want: "mycmd --session-id=********",
		},
		{
			name: "client secret keyword",
			in:   "mycmd --client-secret=abc123def456",
			want: "mycmd --client-secret=********",
		},
		{
			name: "private key keyword",
			in:   "mycmd --private-key=abc123def456",
			want: "mycmd --private-key=********",
		},
		{
			name: "multiple secrets in one command",
			in:   "mycmd --password=hunter2 --token=ghp_abcdEFGH12345678",
			want: "mycmd --password=******** --token=********",
		},
		{
			name: "value immediately followed by escaped quote is not swallowed",
			in:   `echo "curl -H \"X-Api-Key: sk_live_FAKE1234567890abcdef\" https://api.example.com"`,
			want: `echo "curl -H \"X-Api-Key: ********\" https://api.example.com"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scrubCommandText(tt.in))
		})
	}
}

// TestScrubCommandTextDoesNotLeakInputValues is a defence-in-depth check that,
// for every case above with a non-empty "want" difference from "in", the
// secret value itself never survives in the scrubbed output.
func TestScrubCommandTextDoesNotLeakInputValues(t *testing.T) {
	secrets := []string{
		"hunter2",
		"ghp_abcdEFGH12345678",
		"my secret pass",
		"top secret value",
		"abcdef123456",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"AKIAIOSFODNN7EXAMPLE",
		"s3cr3t123",
	}
	inputs := []string{
		"curl --password=hunter2 https://example.com",
		"curl --token ghp_abcdEFGH12345678 https://example.com",
		`mycmd --password="my secret pass" --verbose`,
		"mycmd --secret 'top secret value' --verbose",
		"API_KEY=abcdef123456 ./run.sh",
		"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY aws s3 ls",
		"curl -u admin:hunter2 https://example.com",
		"echo AKIAIOSFODNN7EXAMPLE",
		"mycmd --credential=s3cr3t123",
	}
	for _, in := range inputs {
		out := scrubCommandText(in)
		for _, secret := range secrets {
			assert.NotContains(t, out, secret, "scrubbed output must not contain raw secret %q; input=%q output=%q", secret, in, out)
		}
	}
}
