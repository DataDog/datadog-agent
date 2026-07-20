// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sapabap provisions a single AWS EC2 host that runs the SAP ABAP
// Platform Trial container (SID A4H, instance 00, host vhcala4hci) alongside a
// Datadog Agent container. The container exposes the sapstartsrv SAPControl
// SOAP web service on port 50013; the custom sap_abap check scrapes it for
// work-process, process-state, dispatcher-queue and enqueue-lock metrics.
//
// The SAP ABAP trial image (sapse/abap-cloud-developer-trial) is gated behind a
// Docker Hub login on an account that accepted SAP's EULA on the image page.
// The developer supplies DOCKERHUB_USER + DOCKERHUB_TOKEN (and optionally a
// pinned SAP_ABAP_IMAGE tag) via the process environment at deploy time; these
// are never committed and are passed to the host only through the command
// Environment, never inlined into a command string or logged.
package sapabap

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
	// (roles[sap-abap-host].selected_infra.target): r5.2xlarge (8 vCPU /
	// 64 GiB) satisfies the ABAP trial container's 32 GiB / 4 vCPU minimum with
	// headroom for the co-located Datadog Agent container. The 200 GiB gp3 root
	// volume required by the plan is the framework default
	// (DefaultInstanceStorageSize), so no extra volume knob is needed.
	defaultInstanceType = "r5.2xlarge"

	// defaultImage is the pinned, verified SAP ABAP trial image reference. SAP
	// renames/removes tags, so this MUST be overridable (SAP_ABAP_IMAGE) and
	// kept pinned. The image is NOT anonymously pullable: it requires a Docker
	// Hub login on an EULA-accepted account (see DOCKERHUB_USER/TOKEN).
	defaultImage = "sapse/abap-cloud-developer-trial:ABAPTRIAL_2022_SP01"

	// dockerhubUserEnvVar / dockerhubTokenEnvVar carry the Docker Hub
	// credentials used for `docker login` before the gated pull.
	dockerhubUserEnvVar  = "DOCKERHUB_USER"
	dockerhubTokenEnvVar = "DOCKERHUB_TOKEN"
	// imageEnvVar overrides the pinned image reference.
	imageEnvVar = "SAP_ABAP_IMAGE"

	// sapcontrolUserEnvVar / sapcontrolPasswordEnvVar optionally carry HTTP
	// Basic credentials (OS user a4hadm) for SAPControl read methods that are
	// protected via the profile param service/protectedwebmethods. Empty by
	// default: read methods are commonly unprotected on the trial.
	sapcontrolUserEnvVar     = "SAP_ABAP_SAPCONTROL_USER"
	sapcontrolPasswordEnvVar = "SAP_ABAP_SAPCONTROL_PASSWORD"
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

	// dockerhubUser / dockerhubToken are the Docker Hub credentials used to
	// `docker login` before pulling the EULA-gated SAP ABAP trial image.
	dockerhubUser  string
	dockerhubToken string
	// abapImage is the pinned SAP ABAP trial image reference.
	abapImage string
	// sapcontrolUser / sapcontrolPassword are optional HTTP Basic credentials
	// for protected SAPControl read methods.
	sapcontrolUser     string
	sapcontrolPassword string
}

func newParams() *Params {
	return &Params{
		Name:         vmName,
		instanceType: defaultInstanceType,
		vmOptions:    []ec2.VMOption{ec2.WithOS(compos.Ubuntu2204E2E)},
		deployAgent:  true,
		abapImage:    defaultImage,
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
// environment (ConfigMap-driven flags) plus the Docker Hub / SAP process env
// vars. Instance sizing is capacity-plan-driven (r5.2xlarge default) but still
// honors an explicit ddinfra instance-type override; Agent version/deploy flags
// are honored.
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

	p.dockerhubUser = readEnv(dockerhubUserEnvVar)
	p.dockerhubToken = readEnv(dockerhubTokenEnvVar)
	if img := readEnv(imageEnvVar); img != "" {
		p.abapImage = img
	}
	p.sapcontrolUser = readEnv(sapcontrolUserEnvVar)
	p.sapcontrolPassword = readEnv(sapcontrolPasswordEnvVar)

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

// WithDockerHubCredentials sets the Docker Hub login credentials explicitly (tests).
func WithDockerHubCredentials(user, token string) Option {
	return func(p *Params) error {
		p.dockerhubUser = user
		p.dockerhubToken = token
		return nil
	}
}

// WithImage overrides the pinned SAP ABAP trial image reference.
func WithImage(image string) Option {
	return func(p *Params) error {
		p.abapImage = image
		return nil
	}
}
