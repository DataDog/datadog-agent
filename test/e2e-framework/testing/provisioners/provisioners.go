// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package provisioners contains the definitions of the different provisioners that can be used in a test to setup an environment.
package provisioners

import (
	"context"
	"io"
	"maps"
)

// Diagnosable defines the interface for a diagnosable provisioner.
type Diagnosable interface {
	Diagnose(ctx context.Context, stackName string) (string, error)
}

// Provisioner defines the interface for a provisioner.
type Provisioner interface {
	ID() string
	Destroy(context.Context, string, io.Writer) error
}

// RawResources is the common types returned by provisioners
type RawResources map[string][]byte

// Merge merges two RawResources maps
func (rr RawResources) Merge(in RawResources) {
	maps.Copy(rr, in)
}

// UntypedProvisioner defines the interface for a provisioner without env binding
type UntypedProvisioner interface {
	Provisioner
	Provision(context.Context, string, io.Writer) (RawResources, error)
}

// TypedProvisioner defines the interface for a provisioner with env binding
type TypedProvisioner[Env any] interface {
	Provisioner
	ProvisionEnv(context.Context, string, io.Writer, *Env) (RawResources, error)
}

// ProvisionerMap is a map of provisioners.
type ProvisionerMap map[string]Provisioner

// CopyProvisioners copies a map of provisioners
func CopyProvisioners(in ProvisionerMap) ProvisionerMap {
	out := make(ProvisionerMap, len(in))
	maps.Copy(out, in)
	return out
}
