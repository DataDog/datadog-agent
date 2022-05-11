// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package resolver

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors/common"
)

//go:embed fields_resolver.tmpl
var fieldsResolverTemplate string

// GenerateFieldsResolver generates the fields resolver file
func GenerateFieldsResolver(module *common.Module, output string) error {
	tmpl := template.Must(template.New("tmpl").Parse(fieldsResolverTemplate))
	_ = os.Remove(output)

	// override module name
	module.Name = "probe"

	tmpFile, err := os.CreateTemp(path.Dir(output), "fields_resolver")
	if err != nil {
		return fmt.Errorf("couldn't create temp fields_resolver file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if err = tmpl.Execute(tmpFile, module); err != nil {
		return fmt.Errorf("failed to execute profile: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary fields_resolver file: %w", err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", tmpFile.Name())
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute gofmt: %w", err)
	}

	if err = os.Rename(tmpFile.Name(), output); err != nil {
		return fmt.Errorf("couldn't rename fields_resolver file: %w", err)
	}

	return nil
}
