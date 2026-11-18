// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package kaischeduler deploys the NVIDIA KAI Scheduler (https://github.com/kai-scheduler/KAI-Scheduler)
// and a set of KAI queues on a Kubernetes cluster.
//
// KAI only schedules pods that set `schedulerName: kai-scheduler` and select a leaf queue with the
// QueueLabel label. To schedule GPU workloads on a CPU-only cluster, combine it with the mokka package.
package kaischeduler

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const (
	chartRef = "oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler"

	// DefaultVersion is the pinned version of the KAI Scheduler Helm chart.
	DefaultVersion = "v0.19.0"
	// DefaultNamespace is the namespace KAI is installed in. KAI must not be used to
	// schedule workloads of its own namespace.
	DefaultNamespace = "kai-scheduler"
	// DefaultRootQueue is the parent of the queues created with WithQueues.
	DefaultRootQueue = "e2e-root"

	// SchedulerName is the schedulerName of the pods scheduled by KAI.
	SchedulerName = "kai-scheduler"
	// QueueLabel is the pod (or pod template) label selecting the KAI queue of a workload.
	QueueLabel = "kai.scheduler/queue"

	// MinDatadogChartVersion is the first Datadog Helm chart version supporting
	// datadog.kaiScheduler. Older charts silently ignore AgentHelmValues.
	MinDatadogChartVersion = "3.267.0"
)

// Params holds the configuration of a KAI Scheduler installation.
type Params struct {
	Version   string
	Namespace string
	RootQueue string
	// Queues are the leaf queues created under RootQueue, with unlimited quotas.
	Queues []string
}

// Option configures a KAI Scheduler installation.
type Option = func(*Params) error

// NewParams returns the KAI Scheduler parameters with defaults applied.
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		Version:   DefaultVersion,
		Namespace: DefaultNamespace,
		RootQueue: DefaultRootQueue,
	}
	return common.ApplyOption(params, options)
}

// WithVersion overrides the pinned KAI Scheduler chart version.
func WithVersion(version string) Option {
	return func(p *Params) error {
		p.Version = version
		return nil
	}
}

// WithNamespace sets the namespace KAI is installed in.
func WithNamespace(namespace string) Option {
	return func(p *Params) error {
		p.Namespace = namespace
		return nil
	}
}

// WithQueues creates leaf queues with unlimited quotas under the root queue.
func WithQueues(names ...string) Option {
	return func(p *Params) error {
		p.Queues = append(p.Queues, names...)
		return nil
	}
}

// Component is a KAI Scheduler installation and its queues.
type Component struct {
	pulumi.ResourceState

	Release *helm.Release
}

// NewInstallation installs the KAI Scheduler and its queues on the cluster behind kubeProvider.
func NewInstallation(e config.Env, params *Params, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	comp := &Component{}
	if err := e.Ctx().RegisterComponentResource("dd:kaischeduler", "kai-scheduler", comp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(comp), pulumi.Provider(kubeProvider), pulumi.DeletedWith(kubeProvider))

	var err error
	comp.Release, err = helm.NewRelease(e.Ctx(), e.CommonNamer().ResourceName("kai-scheduler"), &helm.ReleaseArgs{
		Name:            pulumi.String("kai-scheduler"),
		Namespace:       pulumi.String(params.Namespace),
		CreateNamespace: pulumi.Bool(true),
		Chart:           pulumi.String(chartRef),
		Version:         pulumi.String(params.Version),
		Timeout:         pulumi.Int(600),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("kaischeduler: release: %w", err)
	}

	// Queues are custom resources, whose CRDs come with the chart.
	root, err := newQueue(e, params.RootQueue, "", withDependencies(opts, comp.Release)...)
	if err != nil {
		return nil, err
	}
	leafOpts := withDependencies(opts, comp.Release, root)
	for _, name := range params.Queues {
		if _, err := newQueue(e, name, params.RootQueue, leafOpts...); err != nil {
			return nil, err
		}
	}

	if err := e.Ctx().RegisterResourceOutputs(comp, pulumi.Map{"releaseStatus": comp.Release.Status}); err != nil {
		return nil, err
	}
	return comp, nil
}

// withDependencies returns a copy of opts that also depends on deps, without aliasing opts.
func withDependencies(opts []pulumi.ResourceOption, deps ...pulumi.Resource) []pulumi.ResourceOption {
	out := make([]pulumi.ResourceOption, 0, len(opts)+1)
	out = append(out, opts...)
	return append(out, pulumi.DependsOn(deps))
}

// newQueue creates a cluster-scoped KAI queue with unlimited quotas.
func newQueue(e config.Env, name, parent string, opts ...pulumi.ResourceOption) (*apiextensions.CustomResource, error) {
	unlimited := pulumi.Map{"quota": pulumi.Int(-1), "limit": pulumi.Int(-1), "overQuotaWeight": pulumi.Int(1)}
	spec := pulumi.Map{
		"resources": pulumi.Map{"cpu": unlimited, "gpu": unlimited, "memory": unlimited},
	}
	if parent != "" {
		spec["parentQueue"] = pulumi.String(parent)
	}

	queue, err := apiextensions.NewCustomResource(e.Ctx(), e.CommonNamer().ResourceName("kai-queue", name), &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("scheduling.run.ai/v2"),
		Kind:       pulumi.String("Queue"),
		Metadata:   metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		OtherFields: map[string]interface{}{
			"spec": spec,
		},
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("kaischeduler: queue %s: %w", name, err)
	}
	return queue, nil
}

// AgentHelmValues returns the Datadog Helm chart values enabling the KAI integration: the Cluster
// Agent collects KAI PodGroups (with the RBAC it needs) and the node Agents tag pods with them.
// They require a Datadog chart version of at least MinDatadogChartVersion.
func AgentHelmValues() string {
	return `datadog:
  kaiScheduler:
    enabled: true
`
}
