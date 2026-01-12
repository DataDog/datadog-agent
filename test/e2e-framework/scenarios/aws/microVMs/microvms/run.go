// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	commonConfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	ec2Scn "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

type InstanceEnvironment struct {
	*commonConfig.CommonEnvironment
	*aws.Environment
}

type Instance struct {
	e             *InstanceEnvironment
	instance      *remoteComp.Host
	Arch          string
	instanceNamer namer.Namer
	runner        command.Runner
	libvirtURI    pulumi.StringOutput
}

func (i *Instance) IsMacOSHost() bool {
	return runtime.GOOS == "darwin" && i.Arch == LocalVMSet
}

type sshKeyPair struct {
	privateKey string
	publicKey  string
}

const (
	LocalVMSet              = "local"
	defaultShutdownPeriod   = 360 // minutes
	libvirtSSHPrivateKeyX86 = "libvirt_rsa-x86"
	libvirtSSHPrivateKeyArm = "libvirt_rsa-arm"
)

//go:embed files/_datadog.yaml
var datadogAgentConfig string

//go:embed files/_system-probe.yaml
var systemProbeConfig string

//go:embed files/oom_kill.yaml
var oomKillConfig string

var SSHKeyFileNames = map[string]string{
	ec2.AMD64Arch: libvirtSSHPrivateKeyX86,
	ec2.ARM64Arch: libvirtSSHPrivateKeyArm,
}

// GetWorkingDirectory is a function that returns the working directory for the kernel matrix testing, given the architecture (local or non-local).
var GetWorkingDirectory func(string) string

func getKernelVersionTestingWorkingDir(m *config.DDMicroVMConfig) func(string) string {
	return func(arch string) string {
		if arch == LocalVMSet {
			return m.GetStringWithDefault(m.MicroVMConfig, config.DDMicroVMLocalWorkingDirectory, "/tmp")
		}
		return m.GetStringWithDefault(m.MicroVMConfig, config.DDMicroVMRemoteWorkingDirectory, "/tmp")
	}
}

func getSSHKeyPairFiles(m *config.DDMicroVMConfig, arch string) sshKeyPair {
	var pair sshKeyPair
	pair.privateKey = m.GetStringWithDefault(
		m.MicroVMConfig,
		config.SSHKeyConfigNames[arch],
		defaultLibvirtSSHKey(SSHKeyFileNames[arch]),
	)
	pair.publicKey = fmt.Sprintf(
		"%s.pub",
		m.GetStringWithDefault(
			m.MicroVMConfig,
			config.SSHKeyConfigNames[arch],
			defaultLibvirtSSHKey(SSHKeyFileNames[arch]),
		),
	)

	return pair
}

func buildUserData(instanceEnv *InstanceEnvironment, m *config.DDMicroVMConfig) string {
	var sb strings.Builder

	if instanceEnv.DefaultShutdownBehavior() == "terminate" {
		shutdownPeriod := time.Duration(m.GetIntWithDefault(m.MicroVMConfig, config.DDMicroVMShutdownPeriod, defaultShutdownPeriod)) * time.Minute
		sb.WriteString(fmt.Sprintf("#!/bin/bash\nsudo shutdown -P +%d\n", int(shutdownPeriod.Minutes())))
	}

	return sb.String()
}

func newMetalInstance(instanceEnv *InstanceEnvironment, name, arch string, m config.DDMicroVMConfig) (*Instance, error) {
	var instanceType string
	var ami string

	awsEnv := instanceEnv.Environment
	if awsEnv == nil {
		panic("no aws environment setup")
	}

	namer := namer.NewNamer(awsEnv.Ctx(), fmt.Sprintf("%s-%s", awsEnv.Ctx().Stack(), arch))
	if arch == ec2.AMD64Arch {
		instanceType = awsEnv.DefaultInstanceType()
		ami = m.GetStringWithDefault(m.MicroVMConfig, config.DDMicroVMX86AmiID, "")
	} else if arch == ec2.ARM64Arch {
		instanceType = awsEnv.DefaultARMInstanceType()
		ami = m.GetStringWithDefault(m.MicroVMConfig, config.DDMicroVMArm64AmiID, "")
	} else {
		return nil, fmt.Errorf("unsupported arch: %s", arch)
	}

	userData := buildUserData(instanceEnv, &m)
	awsInstance, err := ec2Scn.NewVM(*awsEnv, name,
		ec2Scn.WithInstanceType(instanceType),
		ec2Scn.WithAMI(ami, os.UbuntuDefault, os.Architecture(arch)),
		ec2Scn.WithUserData(userData),
	)
	if err != nil {
		return nil, err
	}

	// Deploy an agent on the launched instance.
	// In the context of KMT, this agent runs on the host environment. As such,
	// it has no knowledge of the individual test VMs, other than as processes in the host machine.
	if awsEnv.AgentDeploy() {
		agentOptions := []agentparams.Option{agentparams.WithAgentConfig(datadogAgentConfig), agentparams.WithSystemProbeConfig(systemProbeConfig), agentparams.WithIntegration("oom_kill", oomKillConfig)}

		if awsEnv.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(awsEnv.AgentFlavor()))
		}

		_, err := agent.NewHostAgent(awsEnv, awsInstance, agentOptions...)
		if err != nil {
			awsEnv.Ctx().Log.Warn(fmt.Sprintf("failed to deploy datadog agent on host instance: %v", err), nil)
		}
	}

	return &Instance{
		e:             instanceEnv,
		instance:      awsInstance,
		Arch:          arch,
		instanceNamer: namer,
	}, nil
}

func newInstance(instanceEnv *InstanceEnvironment, arch string, m config.DDMicroVMConfig) (*Instance, error) {
	name := instanceEnv.Ctx().Stack() + "-" + arch
	if arch != LocalVMSet {
		return newMetalInstance(instanceEnv, name, arch, m)
	}

	namer := namer.NewNamer(instanceEnv.Ctx(), fmt.Sprintf("%s-%s", instanceEnv.Ctx().Stack(), arch))
	return &Instance{
		e:             instanceEnv,
		Arch:          arch,
		instanceNamer: namer,
	}, nil
}

type ScenarioDone struct {
	Instances    []*Instance
	Dependencies []pulumi.Resource
}

func defaultLibvirtSSHKey(keyname string) string {
	return "/tmp/" + keyname
}

func configureInstance(instance *Instance, m *config.DDMicroVMConfig) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource
	var url pulumi.StringOutput
	var err error

	env := *instance.e
	osCommand := command.NewUnixOSCommand()
	localRunner := command.NewLocalRunner(env, command.LocalRunnerArgs{
		OSCommand: osCommand,
	})
	if instance.Arch != LocalVMSet {
		instance.runner = instance.instance.OS.Runner()
	} else {
		instance.runner = localRunner
	}

	shouldProvision := m.GetBoolWithDefault(m.MicroVMConfig, config.DDMicroVMProvisionEC2Instance, true)
	if shouldProvision && instance.Arch != LocalVMSet {
		waitProvision, err := provisionMetalInstance(instance)
		if err != nil {
			return nil, err
		}

		waitFor = append(waitFor, waitProvision...)
	}

	if instance.Arch != LocalVMSet {
		pair := getSSHKeyPairFiles(m, instance.Arch)
		prepareSSHKeysDone, err := prepareLibvirtSSHKeys(
			instance.runner,
			localRunner,
			instance.instanceNamer,
			pair,
			nil,
		)
		if err != nil {
			return nil, err
		}
		waitFor = append(waitFor, prepareSSHKeysDone...)

		privkey := m.GetStringWithDefault(
			m.MicroVMConfig,
			config.SSHKeyConfigNames[instance.Arch],
			defaultLibvirtSSHKey(SSHKeyFileNames[instance.Arch]),
		)
		url = pulumi.Sprintf(
			"qemu+ssh://ubuntu@%s/system?sshauth=privkey&keyfile=%s&known_hosts_verify=ignore",
			instance.instance.Address,
			privkey,
		)
	} else if runtime.GOOS == "darwin" {
		url = pulumi.Sprintf("qemu:///system?socket=/opt/homebrew/var/run/libvirt/libvirt-sock")
	} else {
		url = pulumi.Sprintf("qemu:///system")
	}

	instance.libvirtURI = url

	return waitFor, err
}

// exportVMInformation exports a JSON formatted stack output mapping microvms to host instances
func exportVMInformation(ctx *pulumi.Context, instances map[string]*Instance, vmCollections *[]*VMCollection) {
	output := make(map[string]pulumi.Output)

	for arch, instance := range instances {
		var vms []pulumi.Output
		for _, collection := range *vmCollections {
			if collection.instance.Arch != arch {
				continue
			}
			for _, dls := range collection.domains {
				for _, domain := range dls {
					var tags []pulumi.Output
					for _, tag := range domain.vmset.Tags {
						tags = append(tags, pulumi.ToOutput(tag))
					}
					vms = append(vms, pulumi.ToMapOutput(map[string]pulumi.Output{
						"id":           pulumi.ToOutput(domain.domainID),
						"ip":           pulumi.ToOutput(domain.ip),
						"tag":          pulumi.ToOutput(domain.tag),
						"vmset-tags":   pulumi.ToArrayOutput(tags),
						"ssh-key-path": pulumi.ToOutput(filepath.Join(GetWorkingDirectory(domain.vmset.Arch), "ddvm_rsa")),
						"gdb-port":     pulumi.ToOutput(domain.gdbPort),
					}))
				}
			}
		}

		address := pulumi.Sprintf(LocalVMSet)
		if arch != LocalVMSet {
			address = instance.instance.Address
		}
		output[arch] = pulumi.ToMapOutput(map[string]pulumi.Output{
			"ip":       address,
			"microvms": pulumi.ToArrayOutput(vms),
		})
	}

	ctx.Export("kmt-stack", pulumi.JSONMarshal(pulumi.ToMapOutput(output)))
}

func run(e commonConfig.CommonEnvironment) (*ScenarioDone, error) {
	var waitFor []pulumi.Resource
	var scenarioReady ScenarioDone
	var vmCollections []*VMCollection

	m := config.NewMicroVMConfig(e)
	cfg, err := vmconfig.LoadConfigFile(
		m.GetStringWithDefault(m.MicroVMConfig, config.DDMicroVMConfigFile, "./test.json"),
	)
	if err != nil {
		return nil, err
	}

	GetWorkingDirectory = getKernelVersionTestingWorkingDir(&m)

	archs := make(map[string]bool)
	for _, set := range cfg.VMSets {
		if _, ok := archs[set.Arch]; ok {
			continue
		}
		archs[set.Arch] = true
	}

	instanceEnv := &InstanceEnvironment{&e, nil}
	// We only setup an AWS environment if we need to launch
	// a remote ec2 instance. This is determined by whether there
	// is a non-local vmset in the configuration file. The following
	// loop checks for this.
	for arch := range archs {
		if arch != LocalVMSet {
			awsEnv, err := aws.NewEnvironment(instanceEnv.Ctx(), aws.WithCommonEnvironment(&e))
			if err != nil {
				return nil, err
			}
			instanceEnv.Environment = &awsEnv
			break
		}
	}

	instances := make(map[string]*Instance)
	for arch := range archs {
		instance, err := newInstance(instanceEnv, arch, m)
		if err != nil {
			return nil, err
		}

		instances[arch] = instance
	}

	defer exportVMInformation(instanceEnv.Ctx(), instances, &vmCollections)

	for _, instance := range instances {
		configureDone, err := configureInstance(instance, &m)
		if err != nil {
			return nil, fmt.Errorf("failed to configure instance: %w", err)
		}
		scenarioReady.Instances = append(scenarioReady.Instances, instance)

		waitFor = append(waitFor, configureDone...)
	}

	vmCollections, waitFor, err = BuildVMCollections(instances, cfg.VMSets, waitFor)
	if err != nil {
		return nil, err
	}
	scenarioReady.Dependencies, err = LaunchVMCollections(vmCollections, waitFor)
	if err != nil {
		return nil, err
	}

	shouldProvisionDomain := m.GetBoolWithDefault(m.MicroVMConfig, config.DDMicroVMProvisionDomain, true)
	if shouldProvisionDomain {
		if _, err := provisionRemoteMicroVMs(vmCollections, instanceEnv); err != nil {
			return nil, err
		}

		if _, err := provisionLocalMicroVMs(vmCollections); err != nil {
			return nil, err
		}
	}

	return &scenarioReady, nil
}

func RunAndReturnInstances(e commonConfig.CommonEnvironment) (*ScenarioDone, error) {
	return run(e)
}

func Run(ctx *pulumi.Context) error {
	commonEnv, err := commonConfig.NewCommonEnvironment(ctx)
	if err != nil {
		return err
	}

	_, err = run(commonEnv)
	return err
}
