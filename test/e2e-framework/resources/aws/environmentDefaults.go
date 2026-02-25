// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
)

const (
	sandboxEnv       = "aws/sandbox"
	agentSandboxEnv  = "aws/agent-sandbox"
	agentQAEnv       = "aws/agent-qa"
	tsePlaygroundEnv = "aws/tse-playground"
)

type environmentDefault struct {
	aws     awsProvider
	ddInfra ddInfra
}

type awsProvider struct {
	region  string
	profile string
}

// FakeintakeLBConfig defines the configuration for a fakeintake load balancer.
type FakeintakeLBConfig struct {
	listenerArn string
	baseHost    string
}

// SubnetConfig defines a subnet with its macOS compatibility flag.
type SubnetConfig struct {
	ID              string `json:"id"`
	MacOSCompatible bool   `json:"macos_compatible"`
}

type ddInfra struct {
	defaultVPCID                   string
	defaultSubnets                 []SubnetConfig
	defaultSecurityGroups          []string
	defaultInstanceType            string
	defaultInstanceProfileName     string
	defaultARMInstanceType         string
	defaultWindowsInstanceType     string
	defaultInstanceStorageSize     int
	defaultShutdownBehavior        string
	defaultInternalRegistry        string
	defaultInternalDockerhubMirror string
	useMacosCompatibleSubnets      bool // Some subnets are not compatible with macOS hosts. macOS hosts are supported only in us-east-1a and us-east-1b

	ecs ddInfraECS
	eks ddInfraEKS
}

type ddInfraECS struct {
	execKMSKeyID                  string
	fargateFakeintakeClusterArn   []string
	defaultFakeintakeLBs          []FakeintakeLBConfig
	taskExecutionRole             string
	taskRole                      string
	instanceProfile               string
	serviceAllocatePublicIP       bool
	fargateCapacityProvider       bool
	linuxECSOptimizedNodeGroup    bool
	linuxECSOptimizedARMNodeGroup bool
	linuxBottlerocketNodeGroup    bool
	windowsLTSCNodeGroup          bool
}

type ddInfraEKS struct {
	accountAdminSSORole                  string
	readOnlySSORole                      string
	podSubnets                           []DDInfraEKSPodSubnets
	allowedInboundSecurityGroups         []string
	allowedInboundPrefixList             []string
	allowedInboundManagedPrefixListNames []string
	fargateNamespace                     string
	linuxNodeGroup                       bool
	linuxARMNodeGroup                    bool
	linuxBottlerocketNodeGroup           bool
	windowsLTSCNodeGroup                 bool
}

// DDInfraEKSPodSubnets defines a pod subnet for EKS clusters.
type DDInfraEKSPodSubnets struct {
	AZ       string `json:"az"`
	SubnetID string `json:"subnet"`
}

func getEnvironmentDefault(envName string) environmentDefault {
	switch envName {
	case sandboxEnv:
		return sandboxDefault()
	case agentSandboxEnv:
		return agentSandboxDefault()
	case agentQAEnv:
		return agentQADefault()
	case tsePlaygroundEnv:
		return tsePlaygroundDefault()
	default:
		panic("Unknown environment: " + envName)
	}
}

func sandboxDefault() environmentDefault {
	return environmentDefault{
		aws: awsProvider{
			region:  string(aws.RegionUSEast1),
			profile: "exec-sso-sandbox-account-admin",
		},
		ddInfra: ddInfra{
			defaultVPCID: "vpc-d1aac1a8",
			defaultSubnets: []SubnetConfig{
				{ID: "subnet-b89e00e2", MacOSCompatible: true},
				{ID: "subnet-8ee8b1c6", MacOSCompatible: false},
				{ID: "subnet-3f5db45b", MacOSCompatible: true},
			},
			defaultSecurityGroups:          []string{"sg-46506837", "sg-7fedd80a", "sg-0e952e295ab41e748"},
			defaultInstanceType:            "t3.medium",
			defaultInstanceProfileName:     "ec2InstanceRole",
			defaultARMInstanceType:         "t4g.medium",
			defaultWindowsInstanceType:     "c5.large",
			defaultInstanceStorageSize:     200,
			defaultShutdownBehavior:        "stop",
			defaultInternalRegistry:        "669783387624.dkr.ecr.us-east-1.amazonaws.com",
			defaultInternalDockerhubMirror: "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub",
			useMacosCompatibleSubnets:      false,

			ecs: ddInfraECS{
				execKMSKeyID:                "arn:aws:kms:us-east-1:601427279990:key/c84f93c2-a562-4a59-a326-918fbe7235c7",
				fargateFakeintakeClusterArn: []string{"arn:aws:ecs:us-east-1:601427279990:cluster/fakeintake-ecs"},
				taskExecutionRole:           "arn:aws:iam::601427279990:role/ecsExecTaskExecutionRole",
				taskRole:                    "arn:aws:iam::601427279990:role/ecsExecTaskRole",
				instanceProfile:             "arn:aws:iam::601427279990:instance-profile/ecsInstanceRole",
				serviceAllocatePublicIP:     false,
				fargateCapacityProvider:     true,
				linuxECSOptimizedNodeGroup:  true,
				linuxBottlerocketNodeGroup:  true,
				windowsLTSCNodeGroup:        true,
			},

			eks: ddInfraEKS{
				allowedInboundSecurityGroups: []string{"sg-46506837", "sg-b9e2ebcb"},
				fargateNamespace:             "",
				linuxNodeGroup:               true,
				linuxARMNodeGroup:            true,
				linuxBottlerocketNodeGroup:   true,
				windowsLTSCNodeGroup:         true,
			},
		},
	}
}

func agentSandboxDefault() environmentDefault {
	return environmentDefault{
		aws: awsProvider{
			region:  string(aws.RegionUSEast1),
			profile: "exec-sso-agent-sandbox-account-admin",
		},
		ddInfra: ddInfra{
			defaultVPCID: "vpc-029c0faf8f49dee8d",
			defaultSubnets: []SubnetConfig{
				{ID: "subnet-0a15f3482cd3f9820", MacOSCompatible: true},
				{ID: "subnet-091570395d476e9ce", MacOSCompatible: true},
				{ID: "subnet-003831c49a10df3dd", MacOSCompatible: false},
			},
			defaultSecurityGroups:          []string{"sg-038231b976eb13d44", "sg-05466e7ce253d21b1"},
			defaultInstanceType:            "t3.medium",
			defaultInstanceProfileName:     "ec2InstanceRole",
			defaultARMInstanceType:         "t4g.medium",
			defaultWindowsInstanceType:     "c5.large",
			defaultInstanceStorageSize:     200,
			defaultShutdownBehavior:        "stop",
			defaultInternalRegistry:        "669783387624.dkr.ecr.us-east-1.amazonaws.com",
			defaultInternalDockerhubMirror: "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub",
			useMacosCompatibleSubnets:      false,
			ecs: ddInfraECS{
				execKMSKeyID:                "arn:aws:kms:us-east-1:376334461865:key/1d1fe533-a4f1-44ee-99ec-225b44fcb9ed",
				fargateFakeintakeClusterArn: []string{"arn:aws:ecs:us-east-1:376334461865:cluster/fakeintake-ecs-2", "arn:aws:ecs:us-east-1:376334461865:cluster/fakeintake-ecs-3", "arn:aws:ecs:us-east-1:376334461865:cluster/fakeintake-ecs"},
				defaultFakeintakeLBs: []FakeintakeLBConfig{
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:376334461865:listener/app/fakeintake/3bbebae6506eb8cb/eea87c947a30f106", baseHost: ".lb1.fi.sandbox.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:376334461865:listener/app/fakeintake2/e514320b44979d84/fc96f7de4b914cbd", baseHost: ".lb2.fi.sandbox.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:376334461865:listener/app/fakeintake3/1af15fb150ca4eb4/041c6a59952354c1", baseHost: ".lb3.fi.sandbox.dda-testing.com"},
				},
				taskExecutionRole:          "arn:aws:iam::376334461865:role/ecsTaskExecutionRole",
				taskRole:                   "arn:aws:iam::376334461865:role/ecsTaskRole",
				instanceProfile:            "arn:aws:iam::376334461865:instance-profile/ecsInstanceRole",
				serviceAllocatePublicIP:    false,
				fargateCapacityProvider:    true,
				linuxECSOptimizedNodeGroup: true,
				linuxBottlerocketNodeGroup: true,
				windowsLTSCNodeGroup:       true,
			},

			eks: ddInfraEKS{
				readOnlySSORole:     "arn:aws:iam::376334461865:role/AWSReservedSSO_read-only_14b5d3ee971c41e7",
				accountAdminSSORole: "arn:aws:iam::376334461865:role/AWSReservedSSO_account-admin_6b545a7026a0a2d4",
				podSubnets: []DDInfraEKSPodSubnets{
					{
						AZ:       "us-east-1a",
						SubnetID: "subnet-0159c891fdb0ab50b",
					},
					{
						AZ:       "us-east-1b",
						SubnetID: "subnet-01cb353bec8f2b3e6",
					},
					{
						AZ:       "us-east-1d",
						SubnetID: "subnet-0ba7fbd4fed03bbdd",
					},
				},
				allowedInboundSecurityGroups:         []string{"sg-038231b976eb13d44"},
				allowedInboundManagedPrefixListNames: []string{"vpn-services-commercial-appgate"},
				fargateNamespace:                     "",
				linuxNodeGroup:                       true,
				linuxARMNodeGroup:                    true,
				linuxBottlerocketNodeGroup:           true,
				windowsLTSCNodeGroup:                 true,
			},
		},
	}
}

func agentQADefault() environmentDefault {
	return environmentDefault{
		aws: awsProvider{
			region:  string(aws.RegionUSEast1),
			profile: "exec-sso-agent-qa-account-admin",
		},
		ddInfra: ddInfra{
			defaultVPCID: "vpc-0097b9307ec2c8139",
			defaultSubnets: []SubnetConfig{
				{ID: "subnet-04bf3124d5c31c2e0", MacOSCompatible: true},  // us-east-1a
				{ID: "subnet-06eecbdafc2dac21e", MacOSCompatible: false}, // us-east-1d
				{ID: "subnet-0dabe4bab92b2b9a7", MacOSCompatible: true},  // us-east-1b
			},
			defaultSecurityGroups:          []string{"sg-05e9573fcc582f22c", "sg-0498c960a173dff1e"},
			defaultInstanceType:            "t3.medium",
			defaultInstanceProfileName:     "ec2InstanceRole",
			defaultARMInstanceType:         "t4g.medium",
			defaultWindowsInstanceType:     "c5.large",
			defaultInstanceStorageSize:     200,
			defaultShutdownBehavior:        "stop",
			defaultInternalRegistry:        "669783387624.dkr.ecr.us-east-1.amazonaws.com",
			defaultInternalDockerhubMirror: "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub",
			useMacosCompatibleSubnets:      false,
			ecs: ddInfraECS{
				execKMSKeyID:                "arn:aws:kms:us-east-1:669783387624:key/384373bc-6d99-4d68-84b5-b76b756b0af3",
				fargateFakeintakeClusterArn: []string{"arn:aws:ecs:us-east-1:669783387624:cluster/fakeintake-ecs", "arn:aws:ecs:us-east-1:669783387624:cluster/fakeintake-ecs-2", "arn:aws:ecs:us-east-1:669783387624:cluster/fakeintake-ecs-3"},
				defaultFakeintakeLBs: []FakeintakeLBConfig{
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:669783387624:listener/app/fakeintake/de7956e70776e471/ddfa738893c2dc0e", baseHost: ".lb1.fi.qa.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:669783387624:listener/app/fakeintake2/d59e26c0a29d8567/52a83f7da0f000ee", baseHost: ".lb2.fi.qa.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:669783387624:listener/app/fakeintake3/f90da6a0eaf5638d/647ea5aff700de43", baseHost: ".lb3.fi.qa.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:669783387624:listener/app/fakeintake4/44edf96cc2aafe05/56abdf1d1deb8309", baseHost: ".lb4.fi.qa.dda-testing.com"},
					{listenerArn: "arn:aws:elasticloadbalancing:us-east-1:669783387624:listener/app/fakeintake5/5aa6bcc44f54eb51/6acfe06ec29c5bd0", baseHost: ".lb5.fi.qa.dda-testing.com"},
				},
				taskExecutionRole:          "arn:aws:iam::669783387624:role/ecsTaskExecutionRole",
				taskRole:                   "arn:aws:iam::669783387624:role/ecsTaskRole",
				instanceProfile:            "arn:aws:iam::669783387624:instance-profile/ecsInstanceRole",
				serviceAllocatePublicIP:    false,
				fargateCapacityProvider:    true,
				linuxECSOptimizedNodeGroup: true,
				linuxBottlerocketNodeGroup: true,
				windowsLTSCNodeGroup:       true,
			},

			eks: ddInfraEKS{
				readOnlySSORole:     "arn:aws:iam::669783387624:role/AWSReservedSSO_read-only_e9a50f8c3009a8ce",
				accountAdminSSORole: "arn:aws:iam::669783387624:role/AWSReservedSSO_account-admin_2730b1ac7bbae8eb",
				podSubnets: []DDInfraEKSPodSubnets{
					{
						AZ:       "us-east-1a",
						SubnetID: "subnet-08233fcbc3198be58",
					},
					{
						AZ:       "us-east-1b",
						SubnetID: "subnet-0d3b82115b032c236",
					},
					{
						AZ:       "us-east-1d",
						SubnetID: "subnet-0c051745b55cce91c",
					},
				},
				allowedInboundSecurityGroups:         []string{"sg-05e9573fcc582f22c", "sg-070023ab71cadf760"},
				allowedInboundPrefixList:             []string{"pl-0a698837099ae16f4"},
				allowedInboundManagedPrefixListNames: []string{"vpn-services-commercial-appgate"},
				fargateNamespace:                     "",
				linuxNodeGroup:                       true,
				linuxARMNodeGroup:                    true,
				linuxBottlerocketNodeGroup:           true,
				windowsLTSCNodeGroup:                 true,
			},
		},
	}
}

func tsePlaygroundDefault() environmentDefault {
	return environmentDefault{
		aws: awsProvider{
			region:  string(aws.RegionUSEast1),
			profile: "exec-sso-tse-playground-account-admin",
		},
		ddInfra: ddInfra{
			defaultVPCID: "vpc-0570ac09560a97693",
			defaultSubnets: []SubnetConfig{
				{ID: "subnet-0ec4b9823cf352b95", MacOSCompatible: true},  // us-east-1a
				{ID: "subnet-0e9c45e996754e357", MacOSCompatible: false}, // us-east-1d
				{ID: "subnet-070e1a6c79f6bc499", MacOSCompatible: true},  // us-east-1b
			},
			defaultSecurityGroups:      []string{"sg-091a00b0944f04fd2", "sg-073f15b823d4bb39a", "sg-0a3ec6b0ee295e826"},
			defaultInstanceType:        "t3.medium",
			defaultARMInstanceType:     "t4g.medium",
			defaultWindowsInstanceType: "c5.large",
			defaultInstanceStorageSize: 200,
			defaultShutdownBehavior:    "stop",
			useMacosCompatibleSubnets:  false,

			ecs: ddInfraECS{
				execKMSKeyID:                "arn:aws:kms:us-east-1:570690476889:key/f1694e5a-bb52-42a7-b414-dfd34fbd6759",
				fargateFakeintakeClusterArn: []string{"arn:aws:ecs:us-east-1:570690476889:cluster/fakeintake-ecs", "arn:aws:ecs:us-east-1:570690476889:cluster/fakeintake-ecs-2", "arn:aws:ecs:us-east-1:570690476889:cluster/fakeintake-ecs-3"},
				taskExecutionRole:           "arn:aws:iam::570690476889:role/ecsExecTaskExecutionRole",
				taskRole:                    "arn:aws:iam::570690476889:role/ecsExecTaskRole",
				instanceProfile:             "arn:aws:iam::570690476889:instance-profile/ecsInstanceRole",
				serviceAllocatePublicIP:     false,
				fargateCapacityProvider:     true,
				linuxECSOptimizedNodeGroup:  true,
				linuxBottlerocketNodeGroup:  true,
				windowsLTSCNodeGroup:        true,
			},

			eks: ddInfraEKS{
				allowedInboundSecurityGroups: []string{"sg-091a00b0944f04fd2", "sg-0a3ec6b0ee295e826"},
				fargateNamespace:             "",
				linuxNodeGroup:               true,
				linuxARMNodeGroup:            true,
				linuxBottlerocketNodeGroup:   true,
				windowsLTSCNodeGroup:         true,
			},
		},
	}
}
