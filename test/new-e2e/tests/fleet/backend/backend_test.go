// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backend

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// crtDecode reimplements the Microsoft C runtime command-line parser
// (the rules CommandLineToArgvW implements) and splits a command line into its
// arguments. It is the inverse of windowsCRTEscape and is used to verify that an
// escaped argument round-trips back to its original bytes. It implements the
// backslash/quote toggle rules; the "" -> " in-quotes rule is intentionally not
// implemented because windowsCRTEscape never emits a bare "" sequence.
func crtDecode(cmdline string) []string {
	var args []string
	var cur strings.Builder
	inQuotes := false
	hasArg := false
	for i := 0; i < len(cmdline); {
		c := cmdline[i]
		switch {
		case c == '\\':
			j := i
			for j < len(cmdline) && cmdline[j] == '\\' {
				j++
			}
			nb := j - i
			if j < len(cmdline) && cmdline[j] == '"' {
				cur.WriteString(strings.Repeat(`\`, nb/2))
				hasArg = true
				if nb%2 == 1 {
					cur.WriteByte('"')
				} else {
					inQuotes = !inQuotes
				}
				i = j + 1
			} else {
				cur.WriteString(strings.Repeat(`\`, nb))
				hasArg = true
				i = j
			}
		case c == '"':
			inQuotes = !inQuotes
			hasArg = true
			i++
		case (c == ' ' || c == '\t') && !inQuotes:
			if hasArg {
				args = append(args, cur.String())
				cur.Reset()
				hasArg = false
			}
			i++
		default:
			cur.WriteByte(c)
			hasArg = true
			i++
		}
	}
	if hasArg {
		args = append(args, cur.String())
	}
	return args
}

func TestWindowsCRTEscapeRoundTrip(t *testing.T) {
	cases := []string{
		"datadog-agent",
		"7.82.0-devel.git.147.abc.pipeline.120173372-1",
		`{"deployment_id":"123-0","file_operations":[{"file_op":"merge-patch","file_path":"/datadog.yaml","patch":{"extra_tags":["debug:step-0"]}}]}`,
		`{"packages":[{"package":"datadog-agent","version":"7.80.2-1","url":"oci://install.datadoghq.com/agent-package:7.80"}]}`,
		`{"deployment_id":"jq-replace-tag","file_operations":[{"file_op":"jq","file_path":"/datadog.yaml","transform":".tags |= map(if . == \"env:staging\" then $new_env else . end)","arguments":{"new_env":"env:prod"}}]}`,
		`{"log_level": "debug"}`,
		`value with spaces and "quotes"`,
		`back\slash and \"escaped\" quote`,
		`trailing-backslash\`,
	}

	for _, arg := range cases {
		// windowsCRTEscape emits a fully-quoted argument that PowerShell passes
		// through to the native command line verbatim (it does not re-quote an
		// argument that already contains double quotes). The C runtime must decode
		// it back to exactly the original argument.
		require.Equal(t, []string{arg}, crtDecode(windowsCRTEscape(arg)),
			"argument should round-trip through PowerShell + C runtime for %q", arg)
	}
}
