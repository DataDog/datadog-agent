// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"io"
)

// Provisioner defines the interface for a provisioner.
type Provisioner interface {
	ID() string
	Destroy(context.Context, string, io.Writer) error
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

func copyProvisioners(in ProvisionerMap) ProvisionerMap {
	out := make(ProvisionerMap, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
