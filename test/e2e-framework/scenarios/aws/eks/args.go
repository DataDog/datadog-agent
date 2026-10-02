// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
)

type Params struct {
	LinuxNodeGroup        bool
	LinuxARMNodeGroup     bool
	BottleRocketNodeGroup bool
	WindowsNodeGroup      bool
	GPUNodeGroup          bool
	GPUInstanceType       string
	DisableFargate        bool
	AutoMode              bool
	WithoutInternetAccess bool
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{}
	params, err := common.ApplyOption(version, options)
	if err != nil {
		return nil, err
	}

	// Under EKS Auto Mode AWS owns the data plane and provisions nodes on demand, so
	// managed node groups cannot coexist with it. Reject the combination instead of
	// silently dropping either side: a caller that asked for both gets contradictory
	// infrastructure (for example a Windows Agent with no Windows nodes to run on).
	if params.AutoMode {
		var conflicting []string
		for _, c := range []struct {
			name    string
			enabled bool
		}{
			{"WithLinuxNodeGroup", params.LinuxNodeGroup},
			{"WithLinuxARMNodeGroup", params.LinuxARMNodeGroup},
			{"WithBottlerocketNodeGroup", params.BottleRocketNodeGroup},
			{"WithWindowsNodeGroup", params.WindowsNodeGroup},
			{"WithGPUNodeGroup", params.GPUNodeGroup},
		} {
			if c.enabled {
				conflicting = append(conflicting, c.name)
			}
		}
		if len(conflicting) > 0 {
			return nil, fmt.Errorf(
				"WithAutoMode is incompatible with managed node groups, remove %s: EKS Auto Mode provisions nodes itself",
				strings.Join(conflicting, ", "),
			)
		}

		// Blocking internet egress relies on attaching a per-stack security group to the
		// node group launch templates and the pod ENIs, neither of which exists under Auto
		// Mode: its nodes are managed by AWS and would keep internet access while the
		// caller believes it is blocked.
		if params.WithoutInternetAccess {
			return nil, errors.New(
				"WithAutoMode is incompatible with WithoutInternetAccess: EKS Auto Mode nodes are managed by AWS, " +
					"so the no-internet security group cannot be attached to them",
			)
		}
	}

	return params, nil
}

func WithLinuxNodeGroup() Option {
	return func(p *Params) error {
		p.LinuxNodeGroup = true
		return nil
	}
}

func WithLinuxARMNodeGroup() Option {
	return func(p *Params) error {
		p.LinuxARMNodeGroup = true
		return nil
	}
}

func WithBottlerocketNodeGroup() Option {
	return func(p *Params) error {
		p.BottleRocketNodeGroup = true
		return nil
	}
}

func WithWindowsNodeGroup() Option {
	return func(p *Params) error {
		p.WindowsNodeGroup = true
		return nil
	}
}

// WithGPUNodeGroup enables creation of a GPU-enabled node group.
// instanceType should be a GPU instance type (e.g., "g4dn.xlarge", "g4dn.12xlarge", "g5.xlarge").
// If instanceType is empty, it defaults to "g4dn.xlarge" (1x NVIDIA T4 GPU, cheapest option).
func WithGPUNodeGroup(instanceType string) Option {
	return func(p *Params) error {
		p.GPUNodeGroup = true
		if instanceType == "" {
			instanceType = "g4dn.xlarge" // Default: 1x T4 GPU, ~$0.526/hr on-demand
		}
		p.GPUInstanceType = instanceType
		return nil
	}
}

// WithoutFargate disables the Fargate profile. Prevents DaemonSets from
// accumulating stuck-Pending pods on Fargate's NoSchedule-tainted nodes.
func WithoutFargate() Option {
	return func(p *Params) error {
		p.DisableFargate = true
		return nil
	}
}

// WithAutoMode enables EKS Auto Mode. Auto Mode manages nodes via Karpenter,
// so it is mutually exclusive with managed node groups and Fargate.
// See https://docs.aws.amazon.com/eks/latest/userguide/automode.html
func WithAutoMode() Option {
	return func(p *Params) error {
		p.AutoMode = true
		return nil
	}
}

// WithoutInternetAccess blocks internet egress for the cluster nodes: the account's
// default security groups are replaced by a per-stack security group whose egress is
// restricted to the VPC CIDR, on every security group we create and attach to nodes,
// pod ENIs and the control plane.
//
// This is opt-in: internet access remains the default, and a suite only uses this once
// its image pulls work without internet (e.g. ECR VPC endpoints and pull-through cache
// rules for the public registries it pulls from).
//
// Limitation: EKS-Fargate pods keep internet access, because their ENIs carry the
// AWS-managed cluster security group, which we do not control and which allows all
// egress.
func WithoutInternetAccess() Option {
	return func(p *Params) error {
		p.WithoutInternetAccess = true
		return nil
	}
}

// WithInternetAccess explicitly opts the cluster nodes into internet access, overriding
// a WithoutInternetAccess option set earlier in the options list.
func WithInternetAccess() Option {
	return func(p *Params) error {
		p.WithoutInternetAccess = false
		return nil
	}
}

func buildClusterOptionsFromConfigMap(e aws.Environment) []Option {
	clusterOptions := []Option{}

	// Node groups are enabled by default (see environmentDefaults.go). Under Auto Mode the
	// defaulted ones are left out rather than passed on and rejected by NewParams, since
	// nothing expressed an intent to combine them. Node groups that were explicitly
	// enabled in the config are a real conflict and are reported: the error is carried by
	// an Option so that it surfaces from NewParams like any other invalid combination.
	if e.EKSAutoMode() {
		if conflicting := e.EKSExplicitlyEnabledNodeGroups(); len(conflicting) > 0 {
			return []Option{func(*Params) error {
				return fmt.Errorf(
					"%s is incompatible with managed node groups, disable %s: EKS Auto Mode provisions nodes itself",
					aws.DDInfraEksAutoMode, strings.Join(conflicting, ", "),
				)
			}}
		}
		return append(clusterOptions, WithAutoMode())
	}

	// Add the cluster options from the config map
	if e.EKSWindowsNodeGroup() {
		clusterOptions = append(clusterOptions, WithWindowsNodeGroup())
	}
	if e.EKSLinuxARMNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxARMNodeGroup())
	}
	if e.EKSLinuxNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxNodeGroup())
	}
	if e.EKSBottlerocketNodeGroup() {
		clusterOptions = append(clusterOptions, WithBottlerocketNodeGroup())
	}
	if e.EKSGPUNodeGroup() {
		clusterOptions = append(clusterOptions, WithGPUNodeGroup(e.EKSGPUInstanceType()))
	}
	return clusterOptions
}
