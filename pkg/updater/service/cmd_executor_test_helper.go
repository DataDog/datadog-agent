// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildHelperForTests builds the updater-helper binary for test
func BuildHelperForTests(pkgDir, binPath string, skipUIDCheck bool) error {
	updaterHelper = filepath.Join(binPath, "/updater-helper")
	localPath, _ := filepath.Abs(".")
	targetDir := "datadog-agent/pkg"
	index := strings.Index(localPath, targetDir)
	pkgPath := localPath[:index+len(targetDir)]
	helperPath := filepath.Join(pkgPath, "updater", "service", "helper", "main.go")
	cmd := exec.Command("go", "build", fmt.Sprintf(`-ldflags=-X main.pkgDir=%s -X main.testSkipUID=%v`, pkgDir, skipUIDCheck), "-o", updaterHelper, helperPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
