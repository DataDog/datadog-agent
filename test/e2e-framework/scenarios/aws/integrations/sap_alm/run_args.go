// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sapalm provisions a single Docker Datadog Agent host on AWS whose
// custom sap_alm check polls the external SAP Cloud ALM Analytics public
// sandbox (https://sandbox.api.sap.com/SAPCALM). No SAP workload is deployed:
// the sandbox is the external metric producer. The developer-supplied SAP
// Business Accelerator Hub APIKey is injected into the Agent container at
// deploy time via the SAP_ALM_API_KEY environment variable and is never
// committed.
package sapalm

import (
	"fmt"

	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	// vmName is the standard Agent host role. vm.Export() emits the Pulumi key
	// dd-Host-aws-agent-host, which the rendered invoke task resolves via
	// get_host("aws-agent-host"). Do NOT rename this after the integration.
	vmName = "agent-host"

	// defaultInstanceType comes from the capacity plan
	// (roles[agent-docker-host].selected_infra.target): 2 vCPU / 4 GiB is
	// sufficient for a Docker Agent host that only polls an external HTTPS API.
	defaultInstanceType = "t3.medium"

	// apiKeyEnvVar is the process environment variable the developer exports
	// before `dda inv aws.integrations.sap_alm.create`. It flows through the
	// Pulumi subprocess environment and is injected into the Agent container.
	apiKeyEnvVar = "SAP_ALM_API_KEY"
)

// Params contains all parameters needed to create the environment.
type Params struct {
	Name         string
	instanceType string
	vmOptions    []ec2.VMOption
	// deployAgent mirrors ddagent:deploy; when false the Agent is not installed.
	deployAgent bool
	// agentImageTag mirrors ddagent:version when set.
	agentImageTag string
	// agentFullImagePath mirrors ddagent:fullImagePath when set.
	agentFullImagePath string
	// sapAPIKey is the SAP Business Accelerator Hub APIKey read from the
	// SAP_ALM_API_KEY process environment variable at deploy time.
	sapAPIKey string
}

func newParams() *Params {
	return &Params{
		Name:         vmName,
		instanceType: defaultInstanceType,
		vmOptions:    []ec2.VMOption{ec2.WithOS(compos.Ubuntu2204E2E)},
		deployAgent:  true,
	}
}

// GetParams builds Params from options.
func GetParams(opts ...Option) *Params {
	params := newParams()
	if err := optional.ApplyOptions(params, opts); err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment builds Params by reading configuration from the AWS
// environment (ConfigMap-driven flags) plus the SAP_ALM_API_KEY process env var.
// Instance sizing is capacity-plan-driven (t3.medium default) but still honors an
// explicit ddinfra instance-type override; Agent version/deploy flags are honored.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	// OS/AMI selection (honors ddinfra:osDescriptor / osImageID overrides).
	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.Ubuntu2204E2E)
	if img := e.InfraOSImageID(); img != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithAMI(img, osDesc, osDesc.Architecture))
	} else if e.InfraOSDescriptor() != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))
	}

	// Instance type: capacity-plan default, overridable via ddinfra:instanceType.
	if it := e.DefaultInstanceType(); it != "" {
		p.instanceType = it
	}
	p.vmOptions = append(p.vmOptions, ec2.WithInstanceType(p.instanceType))

	// Agent install + image selection (honors ddagent:deploy / version / fullImagePath).
	p.deployAgent = e.AgentDeploy()
	if p.deployAgent {
		if full := e.AgentFullImagePath(); full != "" {
			p.agentFullImagePath = full
		} else if v := e.AgentVersion(); v != "" {
			p.agentImageTag = v
		}
	}

	p.sapAPIKey = readAPIKey()

	return p
}

// Option modifies Params.
type Option = func(*Params) error

// WithName sets the VM name.
func WithName(name string) Option {
	return func(p *Params) error {
		p.Name = name
		return nil
	}
}

// WithInstanceType overrides the instance type.
func WithInstanceType(instanceType string) Option {
	return func(p *Params) error {
		p.instanceType = instanceType
		return nil
	}
}

// WithSAPAPIKey sets the SAP Business Accelerator Hub APIKey explicitly (tests).
func WithSAPAPIKey(key string) Option {
	return func(p *Params) error {
		p.sapAPIKey = key
		return nil
	}
}
