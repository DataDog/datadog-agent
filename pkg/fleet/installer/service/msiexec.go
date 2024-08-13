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

// removeProduct uses the registry to try and find a product and use msiexec to remove it.
// It is different from msiexec in that it uses the registry and not the stable/experiment path on disk to
// uninstall the product.
// This is needed because in certain circumstances the installer database stored in the stable/experiment paths does not
// reflect the installed version, and using those installers can lead to undefined behavior (either failure to uninstall,
// or weird bugs from uninstalling a product with an installer from a different version).
func removeProduct(product string) (err error) {
	return exec.Command("powershell", fmt.Sprintf(`{
	$installerList = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*" | Where-Object {$_.DisplayName -like '%s'}
	if (($installerList | measure).Count -gt 0) {
		msiexec /x $installerList.PSChildName /qn MSIFASTINSTALL=7
	}
}`, product)).Run()
}
