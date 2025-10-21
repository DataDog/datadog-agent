package config

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/resources/aws/ec2"
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
