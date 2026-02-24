// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import "testing"

func TestIsPathAbsolute_Windows(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// test cases match stdlib filepath.IsAbs test cases
		// https://github.com/golang/go/blob/master/src/path/filepath/path_test.go#L1048-L1063
		{`C:\`, true},
		{`c\`, false},
		{`c::`, false},
		{`c:`, false},
		{`/`, false},
		{`\`, false},
		{`\Windows`, false},
		{`c:a\b`, false},
		{`c:\a\b`, true},
		{`c:/a/b`, true},
		{`\\host\share`, true},
		{`\\host\share\`, true},
		{`\\host\share\foo`, true},
		{`//host/share/foo/bar`, true},
	}

	var res bool
	command := NewWindowsOSCommand()
	for _, test := range tests {
		res = command.IsPathAbsolute(test.path)
		if res != test.want {
			t.Errorf("CheckIsAbsPath(\"%s\") evaluated wrong - want: %t, got: %t", test.path, res, test.want)
		}
	}
}
