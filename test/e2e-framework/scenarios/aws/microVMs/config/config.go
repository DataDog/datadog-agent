// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	sdkconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	ddMicroVMNamespace            = "microvm"
	ddMicroVMX86LibvirtSSHKeyFile = "libvirtSSHKeyFileX86"
	ddMicroVMArmLibvirtSSHKeyFile = "libvirtSSHKeyFileArm"

	DDMicroVMProvisionEC2Instance   = "provision-instance"
	DDMicroVMProvisionDomain        = "provision-microvms"
	DDMicroVMX86AmiID               = "x86AmiID"
	DDMicroVMArm64AmiID             = "arm64AmiID"
	DDMicroVMConfigFile             = "microVMConfigFile"
	DDMicroVMLocalWorkingDirectory  = "localWorkingDir"
	DDMicroVMRemoteWorkingDirectory = "remoteWorkingDir"
	DDMicroVMShutdownPeriod         = "shutdownPeriod"
	DDMicroVMSetupGDB               = "setupGDB"
)

var SSHKeyConfigNames = map[string]string{
	ec2.AMD64Arch: ddMicroVMX86LibvirtSSHKeyFile,
	ec2.ARM64Arch: ddMicroVMArmLibvirtSSHKeyFile,
}

type DDMicroVMConfig struct {
	MicroVMConfig *sdkconfig.Config
	config.CommonEnvironment
}

func NewMicroVMConfig(e config.CommonEnvironment) DDMicroVMConfig {
	return DDMicroVMConfig{
		sdkconfig.New(e.Ctx(), ddMicroVMNamespace),
		e,
	}
}
