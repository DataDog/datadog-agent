// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kind describes a local kind cluster without importing runtime clients.
package kind

import "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"

type Config struct {
	Version string `yaml:"version,omitempty" pattern:"^([0-9]+[.][0-9]+[.][0-9]+)?$" example:"1.33.0" description:"Kubernetes kindest/node version, not the installed kind CLI version."`
	Nodes   int    `yaml:"nodes" default:"0" minimum:"0" description:"Additional worker nodes; a control-plane node is always created."`
}

var Schema = configschema.Must[Config]()
