// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package macos

import (
	"bytes"
	"testing"
)

func TestScriptsAreEmbedded(t *testing.T) {
	for _, path := range []string{InstallScriptPath, UninstallScriptPath} {
		script, err := Scripts.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded script %s: %v", path, err)
		}
		if !bytes.HasPrefix(script, []byte("#!/usr/bin/env bash\n")) {
			t.Fatalf("embedded script %s does not start with the expected shebang", path)
		}
	}
}
