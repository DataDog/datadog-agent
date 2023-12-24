// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	"fmt"
)

// TODO: Remove test packages
var (
	testGolangPackage1_21_5 = Package{
		Package: "go",
		Version: "1.21.5",
		SHA256:  "e2bc0b3e4b64111ec117295c088bde5f00eeed1567999ff77bc859d7df70078e",
		URL:     "https://go.dev/dl/go1.21.5.linux-amd64.tar.gz",
	}
	testGolangPackage1_20_12 = Package{
		Package: "go",
		Version: "1.20.12",
		SHA256:  "9c5d48c54dd8b0a3b2ef91b0f92a1190aa01f11d26e98033efa64c46a30bba7b",
		URL:     "https://go.dev/dl/go1.20.12.linux-amd64.tar.gz",
	}
)

// OrgConfig represents the (remote) configuration of an organization.
// More precisely it hides away the RC details to obtain:
// - the catalog of packages
// - the default version of a package and its corresponding catalog entry
type OrgConfig struct {
}

// NewOrgConfig returns a new OrgConfig.
// TODO: Inject RC client.
func NewOrgConfig() (*OrgConfig, error) {
	return &OrgConfig{}, nil
}

// Package represents a downloadable package.
type Package struct {
	Package string `json:"package"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
}

// GetPackage returns the package with the given name and version.
// The function will block until the catalog is received from RC.
// TODO: Implement with RC support.
func (c *OrgConfig) GetPackage(_ context.Context, pkg string, version string) (Package, error) {
	if pkg == "go" && version == "1.21.5" {
		return testGolangPackage1_21_5, nil
	}
	return Package{}, fmt.Errorf("not implemented")
}

// GetDefaultPackage returns the default version for the given package.
// The function will block until the catalog and org preferences are received from RC.
// TODO: Implement with RC support.
func (c *OrgConfig) GetDefaultPackage(_ context.Context, pkg string) (Package, error) {
	if pkg == "go" {
		return testGolangPackage1_20_12, nil
	}
	return Package{}, fmt.Errorf("not implemented")
}
