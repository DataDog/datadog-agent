// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"os/exec"
	"path/filepath"
)

func msiexec(target, product, operation string, args []string) (err error) {
	updaterPath := filepath.Join(paths.PackagesPath, product, target)
	msis, err := filepath.Glob(filepath.Join(updaterPath, fmt.Sprintf("%s-*-1-x86_64.msi", product)))
	if err != nil {
		return err
	}
	if len(msis) > 1 {
		return fmt.Errorf("too many MSIs in package")
	} else if len(msis) == 0 {
		return fmt.Errorf("no MSIs in package")
	}

	cmd := exec.Command("msiexec", append([]string{operation, msis[0], "/qn", "MSIFASTINSTALL=7"}, args...)...)
	return cmd.Run()
}
