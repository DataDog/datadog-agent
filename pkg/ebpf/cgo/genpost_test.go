// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
