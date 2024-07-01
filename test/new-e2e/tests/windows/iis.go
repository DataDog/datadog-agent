// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package windows contains helpers for Windows E2E tests
package windows

import (
	_ "embed"
	"fmt"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// IISSiteDefinition represents an IIS site definition
type IISSiteDefinition struct {
	Name        string //  name of the site
	BindingPort string // port to bind to, of the form '*:8081'
	AssetsDir   string // directory to copy for assets
}

var (
	//go:embed scripts/installiis.ps1
	installIISScript []byte
)

// InstallIIS installs IIS on the target machine
func InstallIIS(host *components.RemoteHost) error {

	scriptFile := `c:\temp\install-iis.ps1`
	err := host.MkdirAll("C:\\temp")
	if err != nil {
		return err
	}
	_, err = host.WriteFile(scriptFile, installIISScript)
	if err != nil {
		return err
	}
	// still need to figure out if we need to reboot
	output, err := host.Execute(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to install IIS: %w\n%s", err, output)
	}
	return nil
}

// CreateIISSite creates an IIS site on the target machine
func CreateIISSite(host *components.RemoteHost, site []IISSiteDefinition) error {

	for _, s := range site {

		// create the site directory
		//tgtpath := fmt.Sprintf("c:\\tmp\\inetpub\\%s", s.Name)
		tgtpath := path.Join("c:", "tmp", "inetpub", s.Name)
		err := host.MkdirAll(tgtpath)
		if err != nil {
			return err
		}

		if s.AssetsDir != "" {
			// copy the assets
			if err := host.CopyFolder(s.AssetsDir, tgtpath); err != nil {
				return err
			}

		}
		script := `
		if ((get-iissite -name %s).State -ne "Started") {
			New-IISSite -ErrorAction SilentlyContinue -Name %s -BindingInformation '%s' -PhysicalPath %s
		}`
		wintgtpath := strings.Replace(tgtpath, "/", "\\", -1)
		cmd := fmt.Sprintf(script, s.Name, s.Name, s.BindingPort, wintgtpath)
		output, err := host.Execute(cmd)
		if err != nil {
			return fmt.Errorf("failed to create IIS site: %w\n%s", err, output)
		}
	}
	return nil
}
