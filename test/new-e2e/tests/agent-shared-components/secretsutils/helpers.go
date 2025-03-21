// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsutils contains utilities to manage secrets for e2e tests.
package secretsutils

import (
	"bytes"
	_ "embed"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	perms "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/filepermissions"

	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

//go:embed fixtures/secret-resolver.py
var secretResolverScript string

// WithUnixSecretSetupScript returns an agent param that setups a secret resolver script with correct permissions.
func WithUnixSecretSetupScript(path string, allowGroupExec bool) func(*agentparams.Params) error {
	return agentparams.WithFileWithPermissions(path, secretResolverScript, true, WithUnixSecretPermissions(allowGroupExec))
}

// WithUnixSecretPermissions returns an UnixPermissions object containing correct permissions for a secret backend script.
func WithUnixSecretPermissions(allowGroupExec bool) optional.Option[perms.FilePermissions] {
	if allowGroupExec {
		return perms.NewUnixPermissions(perms.WithPermissions("0750"), perms.WithOwner("dd-agent"), perms.WithGroup("root"))
	}

	return perms.NewUnixPermissions(perms.WithPermissions("0700"), perms.WithOwner("dd-agent"), perms.WithGroup("dd-agent"))
}

//go:embed fixtures/secret_wrapper.bat
var secretWrapperScript string

// WithWindowsSecretSetupScript returns a list of agent params that setups a secret resolver script with correct permissions.
func WithWindowsSecretSetupScript(wrapperPath string, allowGroupExec bool) []func(*agentparams.Params) error {
	// On Windows we're using a wrapper around the python script because we can't execute python scripts directly
	// (this would require modifying permissions of the python binary)
	// Basically the setup looks like this:
	// <path>/
	// ├── secret.py
	// └── secret_wrapper.bat (specific permissions)

	wrapperPath = strings.ReplaceAll(wrapperPath, `\`, `/`)

	dir, _ := filepath.Split(wrapperPath)
	pythonScriptPath := filepath.Join(dir, "secret.py")
	secretWrapperContent := fillSecretWrapperTemplate(strings.ReplaceAll(pythonScriptPath, "/", "\\"))

	return []func(*agentparams.Params) error{
		agentparams.WithFileWithPermissions(wrapperPath, secretWrapperContent, true, WithWindowsSecretPermissions(allowGroupExec)),
		agentparams.WithFile(pythonScriptPath, secretResolverScript, true),
	}
}

// WithWindowsSecretPermissions returns a WindowsPermissions object containing correct permissions for a secret backend script.
func WithWindowsSecretPermissions(allowGroupExec bool) optional.Option[perms.FilePermissions] {
	icaclsCmd := `/grant "ddagentuser:(RX)"`
	if allowGroupExec {
		icaclsCmd += ` "Administrators:(RX)"`
	}

	return perms.NewWindowsPermissions(perms.WithIcaclsCommand(icaclsCmd), perms.WithDisableInheritance())
}

// fillSecretWrapperTemplate fills the secret wrapper template with the correct path to the python script.
func fillSecretWrapperTemplate(pythonScriptPath string) string {
	var buffer bytes.Buffer
	var templateVars = map[string]string{
		"PythonScriptPath": pythonScriptPath,
	}

	tmpl, err := template.New("").Parse(secretWrapperScript)
	if err != nil {
		panic("Could not parse secret wrapper template")
	}

	err = tmpl.Execute(&buffer, templateVars)
	if err != nil {
		panic("Could not fill variables in secret wrapper template")
	}

	return buffer.String()
}
