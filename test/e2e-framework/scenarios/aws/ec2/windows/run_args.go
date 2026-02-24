// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windows

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/fipsmode"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/testsigning"
)

const defaultVMName = "vm"

// RunParams is a set of parameters for the Windows Host scenario.
type RunParams struct {
	Name string

	instanceOptions        []ec2.VMOption
	agentOptions           []agentparams.Option
	agentClientOptions     []agentclientparams.Option
	fakeintakeOptions      []fakeintake.Option
	activeDirectoryOptions []activedirectory.Option
	defenderoptions        []defender.Option
	fipsModeOptions        []fipsmode.Option
	testsigningOptions     []testsigning.Option
}

// RunOption modifies RunParams
type RunOption func(*RunParams) error

func newRunParams() *RunParams {
	return &RunParams{
		Name:                   defaultVMName,
		instanceOptions:        []ec2.VMOption{},
		agentOptions:           []agentparams.Option{},
		agentClientOptions:     []agentclientparams.Option{},
		fakeintakeOptions:      []fakeintake.Option{},
		activeDirectoryOptions: []activedirectory.Option{},
		// Disable Windows Defender on VMs by default
		defenderoptions:    []defender.Option{defender.WithDefenderDisabled()},
		fipsModeOptions:    []fipsmode.Option{},
		testsigningOptions: []testsigning.Option{},
	}
}

// GetRunParams returns RunParams after applying options
func GetRunParams(opts ...RunOption) *RunParams {
	p := newRunParams()

	// Enable TestSigning when testsigned drivers are requested via environment
	// this is used for kicking off testsigned windows drivers on CI
	if v := os.Getenv("WINDOWS_DDNPM_DRIVER"); v == "testsigned" {
		p.testsigningOptions = append(p.testsigningOptions, testsigning.WithTestSigningEnabled())
	}
	if v := os.Getenv("WINDOWS_DDPROCMON_DRIVER"); v == "testsigned" {
		p.testsigningOptions = append(p.testsigningOptions, testsigning.WithTestSigningEnabled())
	}

	if err := optional.ApplyOptions(p, opts); err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return p
}

// ParamsFromEnvironment maps environment to Windows EC2 scenario params
func ParamsFromEnvironment(e aws.Environment) *RunParams {
	p := newRunParams()

	// Agent installation toggle
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	// Fakeintake options
	if e.AgentUseFakeintake() {
		fi := []fakeintake.Option{}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fi = append(fi, fakeintake.WithLoadBalancer())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fi = append(fi, fakeintake.WithRetentionPeriod(retention))
		}
		p.fakeintakeOptions = fi
	} else {
		p.fakeintakeOptions = nil
	}

	// Enable TestSigning when testsigned drivers are requested via environment
	if v := os.Getenv("WINDOWS_DDNPM_DRIVER"); v == "testsigned" {
		p.testsigningOptions = append(p.testsigningOptions, testsigning.WithTestSigningEnabled())
	}
	if v := os.Getenv("WINDOWS_DDPROCMON_DRIVER"); v == "testsigned" {
		p.testsigningOptions = append(p.testsigningOptions, testsigning.WithTestSigningEnabled())
	}

	return p
}

// WithName sets the VM name
func WithName(name string) RunOption {
	return func(p *RunParams) error { p.Name = name; return nil }
}

// WithEC2InstanceOptions adds EC2 VM options
func WithEC2InstanceOptions(opts ...ec2.VMOption) RunOption {
	return func(p *RunParams) error { p.instanceOptions = append(p.instanceOptions, opts...); return nil }
}

// WithAgentOptions adds Agent options
func WithAgentOptions(opts ...agentparams.Option) RunOption {
	return func(p *RunParams) error { p.agentOptions = append(p.agentOptions, opts...); return nil }
}

// WithoutAgent disables agent creation
func WithoutAgent() RunOption {
	return func(p *RunParams) error { p.agentOptions = nil; return nil }
}

// WithAgentClientOptions adds agent client options
func WithAgentClientOptions(opts ...agentclientparams.Option) RunOption {
	return func(p *RunParams) error {
		p.agentClientOptions = append(p.agentClientOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds FakeIntake options
func WithFakeIntakeOptions(opts ...fakeintake.Option) RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = append(p.fakeintakeOptions, opts...); return nil }
}

// WithoutFakeIntake disables FakeIntake creation
func WithoutFakeIntake() RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = nil; return nil }
}

// WithActiveDirectoryOptions configures Active Directory on the VM
func WithActiveDirectoryOptions(opts ...activedirectory.Option) RunOption {
	return func(p *RunParams) error {
		p.activeDirectoryOptions = append(p.activeDirectoryOptions, opts...)
		return nil
	}
}

// WithDefenderOptions configures Windows Defender
func WithDefenderOptions(opts ...defender.Option) RunOption {
	return func(p *RunParams) error {
		p.defenderoptions = append(p.defenderoptions, opts...)
		return nil
	}
}

// WithFIPSModeOptions configures FIPS mode
func WithFIPSModeOptions(opts ...fipsmode.Option) RunOption {
	return func(p *RunParams) error {
		p.fipsModeOptions = append(p.fipsModeOptions, opts...)
		return nil
	}
}

// WithTestSigningOptions configures TestSigning
func WithTestSigningOptions(opts ...testsigning.Option) RunOption {
	return func(p *RunParams) error {
		p.testsigningOptions = append(p.testsigningOptions, opts...)
		return nil
	}
}
