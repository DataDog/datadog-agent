// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package macos

import (
	"bytes"
	"testing"
)

func TestUninstallScriptIsEmbedded(t *testing.T) {
	script, err := Scripts.ReadFile(UninstallScriptPath)
	if err != nil {
		t.Fatalf("read embedded uninstall script: %v", err)
	}
	if !bytes.HasPrefix(script, []byte("#!/usr/bin/env bash\n")) {
		t.Fatal("embedded uninstall script does not start with the expected shebang")
	}
}
