// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixGccUnsignedCharConsts(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"X Type = 0.000000", "X Type = 0x0"},
		{"X Type = 24.000000", "X Type = 0x18"},
		{"X Type = -1.000000", "X Type = -0x1"},
		// Exactly 6 zeros after the dot is the cgo %f signature; leave anything else alone.
		{"X Type = 1.5", "X Type = 1.5"},
		{"X Type = 1.00000", "X Type = 1.00000"},
	}
	for _, c := range cases {
		assert.Equal(t, c.out, string(fixGccUnsignedCharConsts([]byte(c.in))))
	}
}

func TestRemoveAbsolute(t *testing.T) {
	linuxStr := `// cgo -godefs -- -fsigned-char /git/datadog-agent/pkg/network/driver/types.go`
	assert.Equal(t, "// cgo -godefs -- -fsigned-char types.go", string(removeAbsolutePath([]byte(linuxStr), "linux")))

	linuxStr = `// cgo -godefs -- -I ../../ebpf/c -fsigned-char /git/datadog-agent/pkg/network/driver/types.go`
	assert.Equal(t, "// cgo -godefs -- -I ../../ebpf/c -fsigned-char types.go", string(removeAbsolutePath([]byte(linuxStr), "linux")))

	winStr := `// cgo.exe -godefs -- -fsigned-char C:\dev\go\src\github.com\datadog\datadog-agent\pkg\network\driver\types.go`
	assert.Equal(t, "// cgo.exe -godefs -- -fsigned-char types.go", string(removeAbsolutePath([]byte(winStr), "windows")))

	winStr = `// cgo.exe -godefs -- -I ..\c -fsigned-char C:\dev\go\src\github.com\datadog\datadog-agent\pkg\network\driver\types.go`
	assert.Equal(t, `// cgo.exe -godefs -- -I ..\c -fsigned-char types.go`, string(removeAbsolutePath([]byte(winStr), "windows")))
}
