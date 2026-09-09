// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dellpowerflex

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// bootstrapScript is the operator-driven, DEFERRED nested-cluster bootstrap. It
// is staged onto the host (not executed) so an engineer can drive the PFMP
// first-boot wizard and nested MDM/SDS cluster build over `virsh console`
// during live exploration. See bootstrap.sh for the documented sequence.
//
//go:embed bootstrap.sh
var bootstrapScript string

// bootstrapRemotePath is where the script lands on the host. It is staged in
// the default SSH user's home so no privileged copy is required; an operator
// runs it with sudo during live exploration.
const bootstrapRemotePath = "/home/ec2-user/dell-powerflex-bootstrap.sh"

// stageBootstrapScript writes the deferred bootstrap script onto the host and
// marks it executable. It does NOT run it. Idempotent: the file is overwritten
// on every apply.
func stageBootstrapScript(e config.Env, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	runner := host.OS.Runner()
	namer := e.CommonNamer().WithPrefix("pflex")

	copyScript, err := host.OS.FileManager().CopyInlineFile(
		pulumi.String(bootstrapScript),
		bootstrapRemotePath,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return runner.Command(
		namer.ResourceName("chmod-bootstrap-script"),
		&command.Args{
			// CopyInlineFile sudo-moves the file into place (root-owned), so chmod
			// must run with sudo or it fails with "Operation not permitted".
			Create: pulumi.Sprintf("sudo chmod 0755 %s", bootstrapRemotePath),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(copyScript))...,
	)
}
