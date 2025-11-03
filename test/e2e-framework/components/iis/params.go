// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package iis

// SiteDefinition represents an IIS site definition
type SiteDefinition struct {
	Name            string //  name of the site
	BindingPort     string // port to bind to, of the form '*:8081'
	SourceAssetsDir string // directory to copy the assets from on the local host
	TargetAssetsDir string // directory to copy the assets to on the remote host
}

// Configuration represents the Active Directory configuration (domain name, password, users etc...)
type Configuration struct {
	Sites []SiteDefinition
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithSite adds a site to create
func WithSite(site SiteDefinition) func(*Configuration) error {
	return func(p *Configuration) error {
		p.Sites = append(p.Sites, site)
		return nil
	}
}
