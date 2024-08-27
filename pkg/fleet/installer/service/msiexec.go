// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"golang.org/x/sys/windows/registry"
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

// Product represents a software from the Windows Registry
type Product struct {
	// Code is the software product code
	Code string
	// UninstallString is the string that can be executed to uninstall the software. May be empty.
	UninstallString string
}

func findProductCode(name string) (error, *Product) {
	rootPath := "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall"
	reg, err := registry.OpenKey(registry.LOCAL_MACHINE, rootPath, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return err, nil
	}
	defer reg.Close()
	keys, err := reg.ReadSubKeyNames(0)
	if err != nil {
		return err, nil
	}
	for _, key := range keys {
		err, product := processKey(rootPath, key, name)
		if err == nil && product != nil {
			return nil, product
		}
	}
	return nil, nil
}

func processKey(rootPath, key, name string) (error, *Product) {
	subkey, err := registry.OpenKey(registry.LOCAL_MACHINE, rootPath+"\\"+key, registry.QUERY_VALUE)
	if err != nil {
		return err, nil
	}
	defer subkey.Close()

	displayName, _, err := subkey.GetStringValue("DisplayName")
	if err == nil && displayName == name {
		product := &Product{}
		product.UninstallString, _, _ = subkey.GetStringValue("UninstallString")
		product.Code = key
		return nil, product
	}

	return nil, nil
}

// removeProduct uses the registry to try and find a product and use msiexec to remove it.
// It is different from msiexec in that it uses the registry and not the stable/experiment path on disk to
// uninstall the product.
// This is needed because in certain circumstances the installer database stored in the stable/experiment paths does not
// reflect the installed version, and using those installers can lead to undefined behavior (either failure to uninstall,
// or weird bugs from uninstalling a product with an installer from a different version).
func removeProduct(productName string) error {
	err, product := findProductCode(productName)
	if err != nil {
		return fmt.Errorf("error trying to find product %s: %w", productName, err)
	}
	if product != nil {
		cmd := exec.Command("msiexec", "/x", product.Code, "/qn", "MSIFASTINSTALL=7")
		return cmd.Run()
	}
	return fmt.Errorf("product %s not found", productName)
}
