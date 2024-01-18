// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/catalog.json
var rawCatalog []byte

//go:embed data/defaults.json
var rawDefaults []byte

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
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
}

type catalog struct {
	Packages []Package `json:"packages"`
}

// GetPackage returns the package with the given name and version.
// The function will block until the catalog is received from RC.
// TODO: Implement with RC support instead of hardcoded files.
func (c *OrgConfig) GetPackage(_ context.Context, pkg string, version string) (Package, error) {
	var catalog catalog
	err := json.Unmarshal(rawCatalog, &catalog)
	if err != nil {
		return Package{}, fmt.Errorf("could not unmarshal catalog: %w", err)
	}
	for _, p := range catalog.Packages {
		if p.Name == pkg && p.Version == version {
			return p, nil
		}
	}
	return Package{}, fmt.Errorf("package %s version %s not found", pkg, version)
}

// GetDefaultPackage returns the default version for the given package.
// The function blocks until the catalog and org preferences are received from RC.
// TODO: Implement with RC support instead of hardcoded files.
func (c *OrgConfig) GetDefaultPackage(ctx context.Context, pkg string) (Package, error) {
	var defaults map[string]string
	err := json.Unmarshal(rawDefaults, &defaults)
	if err != nil {
		return Package{}, fmt.Errorf("could not unmarshal defaults: %w", err)
	}
	return c.GetPackage(ctx, pkg, defaults[pkg])
}
