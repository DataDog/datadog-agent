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

// IISApplicationDefinition represents an IIS application definition
type IISApplicationDefinition struct {
	Name string // name of the application.  this will also be the path
				// e.g. app1/dir2
	PhysicalPath string // physical path to the application
						// this must exist prior to creation of the application, or it will fail
}
// IISSiteDefinition represents an IIS site definition
type IISSiteDefinition struct {
	Name        string //  name of the site
	BindingPort string // port to bind to, of the form '*:8081'
	SiteDir		string // directory to create for the site
					   // can be empty for default.
	AssetsDir   string // directory to copy for assets
	Applications []IISApplicationDefinition
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
		var tgtpath string
		if s.SiteDir == "" {
			tgtpath = path.Join("c:", "tmp", "inetpub", s.Name)
		} else {
			tgtpath = s.SiteDir
		}
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
		for _, app := range s.Applications {
			// create the application
			script := `
			$res = Get-WebApplication -Name %s
			if ($res -eq $null) {
				New-WebApplication -Site %s -Name %s -PhysicalPath %s
			}`
			physpath := strings.Replace(app.PhysicalPath, "/", "\\", -1)

			// make the physical path first since it's required 
			err := host.MkdirAll(physpath)
			if err != nil {
				return err
			}
			if s.AssetsDir != "" {
				// copy the assets
				if err := host.CopyFolder(s.AssetsDir, physpath); err != nil {
					return err
				}
			}
			cmd := fmt.Sprintf(script, app.Name, s.Name, app.Name, physpath)
			output, err := host.Execute(cmd)
			if err != nil {
				return fmt.Errorf("failed to create IIS app %s in site: %w\n%s", app.Name, err, output)
			}
		}
	}
	return nil
}
