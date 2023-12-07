// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import "context"

type Provisioner interface {
	ID() string
	Delete(string, context.Context) error
}

type UntypedProvisioner interface {
	Provisioner
	Provision(string, context.Context) (RawResources, error)
}

type TypedProvisioner[Env any] interface {
	Provisioner
	ProvisionEnv(string, context.Context, *Env) (RawResources, error)
}
