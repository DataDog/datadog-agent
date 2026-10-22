// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	scriptDirectoryName       = "script"
	dependenciesDirectoryName = "dependencies"
)

// LocalArtifact identifies the local files of an extracted authored-script package.
type LocalArtifact struct {
	Directory string
}

func (a LocalArtifact) ScriptDirectory() string {
	return filepath.Join(a.Directory, scriptDirectoryName)
}

func (a LocalArtifact) DependencyDirectory(name string) string {
	return filepath.Join(a.Directory, dependenciesDirectoryName, name)
}

func ValidateDependencyName(name string) error {
	if name == "" || name == "." || !filepath.IsLocal(name) || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("dependency name %q must be a single non-empty path component", name)
	}
	return nil
}
