// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package provisioners

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// The Pulumi-free provisioner interfaces and helpers now live in the
// [provisioner] package. The aliases below keep every existing import of this
// package working unchanged. Pulumi-backed implementations (pulumi_provisioner,
// the cloud subpackages) stay here, and importing them remains the only way to
// link the Pulumi SDK.

type (
	Diagnosable               = provisioner.Diagnosable
	Provisioner               = provisioner.Provisioner
	UntypedProvisioner        = provisioner.UntypedProvisioner
	TypedProvisioner[Env any] = provisioner.TypedProvisioner[Env]
	ProvisionerMap            = provisioner.ProvisionerMap
	FileProvisioner           = provisioner.FileProvisioner
	RawResources              = provisioner.RawResources
)

// NewFileProvisioner returns a new FileProvisioner.
func NewFileProvisioner(id string, fsys fs.FS) *FileProvisioner {
	return provisioner.NewFileProvisioner(id, fsys)
}

// CopyProvisioners copies a map of provisioners.
func CopyProvisioners(in ProvisionerMap) ProvisionerMap {
	return provisioner.CopyProvisioners(in)
}
