// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentbuild declares provider-owned CLI input types, never build logic.
package agentbuild

import "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"

type ExistingImage struct {
	Reference string `yaml:"reference" config:"required" description:"Image already present in the local Docker daemon" example:"registry.datadoghq.com/agent:7.83.0"`
	Manifest  string `yaml:"manifest,omitempty" description:"Optional previously generated artifact receipt"`
}
type InvokeImage struct {
	Repository        string   `yaml:"repository" config:"required" description:"Absolute source checkout root" example:"/home/user/datadog-agent"`
	Reference         string   `yaml:"reference" config:"required" description:"Output image reference" example:"localhost/datadog-agent:7.99.0-dev"`
	BaseImage         string   `yaml:"base-image" default:"registry.datadoghq.com/agent:7.83.0" description:"Released base image"`
	RebuildComponents []string `yaml:"rebuild-components,omitempty" description:"Additional components rebuilt rather than inherited"`
	Race              bool     `yaml:"race,omitempty" description:"Build with race instrumentation"`
}
type ExistingBinary struct {
	Manifest string `yaml:"manifest" config:"required" description:"Absolute verified binary bundle receipt" example:"/home/user/artifacts/result.json"`
}
type InvokeBinary struct {
	Repository string `yaml:"repository" config:"required" description:"Absolute source checkout root" example:"/home/user/datadog-agent"`
	Race       bool   `yaml:"race,omitempty" description:"Build with race instrumentation"`
}
type ExistingPackage struct {
	Path     string `yaml:"path" config:"required" description:"Exact absolute datadog-agent DEB path" example:"/home/user/artifacts/agent.deb"`
	Manifest string `yaml:"manifest,omitempty" description:"Previously generated artifact receipt"`
}
type OmnibusRepackage struct {
	Repository        string `yaml:"repository" config:"required" description:"Absolute source checkout root" example:"/home/user/datadog-agent"`
	BuildImage        string `yaml:"build-image" config:"required" description:"Locally available isolated native Omnibus build image with dda" example:"registry.example.com/agent-build:latest"`
	BasePackageURL    string `yaml:"base-package-url" config:"required" description:"Explicit credential-free HTTPS base DEB URL" example:"https://example.com/datadog-agent.deb"`
	BasePackageSHA256 string `yaml:"base-package-sha256" config:"required" description:"SHA256 of the exact base package"`
}

var ExistingImageSchema = configschema.Must[ExistingImage]()
var InvokeImageSchema = configschema.Must[InvokeImage]()
var ExistingBinarySchema = configschema.Must[ExistingBinary]()
var InvokeBinarySchema = configschema.Must[InvokeBinary]()
var ExistingPackageSchema = configschema.Must[ExistingPackage]()
var OmnibusRepackageSchema = configschema.Must[OmnibusRepackage]()
