// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lustre

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
)

const (
	// serverVMName is the Pulumi resource name for the Lustre server host.
	// vm.Export() emits it as stack output dd-Host-aws-lustre-server (task role
	// "aws-lustre-server", user-facing alias "server").
	serverVMName = "lustre-server"
	// clientVMName MUST be "agent-host" so vm.Export() emits dd-Host-aws-agent-host,
	// matching the task module's REMOTE_HOSTNAME ("aws-agent-host", user-facing
	// aliases "client"/"agent").
	clientVMName = "agent-host"

	// Instance types match the lab's capacity plan.
	defaultServerInstanceType = "m5.xlarge" // 4 vCPU / 16 GiB — combined MGS+MDS+OSS ldiskfs
	defaultClientInstanceType = "m5.large"  // 2 vCPU / 8 GiB — client + Datadog Agent

	// clientOSAlmaLinux810 / clientOSAlmaLinux94 are the accepted values for the
	// clientOS param. The default preserves today's behavior (EL8.10 client).
	clientOSAlmaLinux810 = "almalinux-8.10"
	clientOSAlmaLinux94  = "almalinux-9.4"

	// defaultClientLustreVersion / defaultClientOS are the stable lab config used
	// when no params are passed: a Lustre 2.15.6 client on AlmaLinux 8.10.
	defaultClientLustreVersion = "2.15.6"
	defaultClientOS            = clientOSAlmaLinux810

	// Pulumi config keys (ddinfra: namespace) to opt into a newer client, e.g. a
	// 2.16.1 client on AlmaLinux 9.4:
	//   -c ddinfra:lustre/clientLustreVersion=2.16.1
	//   -c ddinfra:lustre/clientOS=almalinux-9.4
	clientLustreVersionParam = "lustre/clientLustreVersion"
	clientOSParam            = "lustre/clientOS"
)

// Params holds the run parameters for the Lustre multi-host scenario.
type Params struct {
	ServerInstanceType string
	ClientInstanceType string

	// ClientLustreVersion is the Whamcloud Lustre release for the CLIENT only
	// (default 2.15.6). Set to 2.16.1 (with ClientOS=almalinux-9.4) to test a
	// newer client. The SERVER is always 2.15.6 ldiskfs and is not affected.
	ClientLustreVersion string
	// ClientOS selects the client VM's AlmaLinux image: clientOSAlmaLinux810
	// (default) or clientOSAlmaLinux94. It drives both the booted AMI and the
	// Whamcloud client repo el-path (el8.10 / el9.4).
	ClientOS string

	// agentOptions is nil when the Agent should not be deployed (ddagent:deploy=false).
	agentOptions []agentparams.Option
}

// ParamsFromEnvironment builds Params from the AWS environment, honoring the
// standard ddagent flags while sizing the hosts from the capacity plan.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := &Params{
		ServerInstanceType:  defaultServerInstanceType,
		ClientInstanceType:  defaultClientInstanceType,
		ClientLustreVersion: e.GetStringWithDefault(e.InfraConfig, clientLustreVersionParam, defaultClientLustreVersion),
		ClientOS:            e.GetStringWithDefault(e.InfraConfig, clientOSParam, defaultClientOS),
		agentOptions:        []agentparams.Option{},
	}

	// Honor ddagent:deploy. Agent version/pipeline/flavor are read from the env
	// automatically by agentparams.NewParams.
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	return p
}
