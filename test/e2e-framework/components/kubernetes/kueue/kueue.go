// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package kueue deploys Kueue (https://kueue.sigs.k8s.io) on a Kubernetes cluster, with one
// ClusterQueue and a LocalQueue per workload namespace.
//
// Kueue only admits Jobs that select a LocalQueue with the QueueNameLabel label: until then it
// keeps them suspended. To admit GPU workloads on a CPU-only cluster, combine it with the mokka package.
package kueue

import (
	"fmt"
	"slices"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const (
	chartRef   = "oci://registry.k8s.io/kueue/charts/kueue"
	apiVersion = "kueue.x-k8s.io/v1beta2"

	// DefaultVersion is the pinned version of the Kueue Helm chart.
	DefaultVersion = "0.20.6"
	// DefaultNamespace is the namespace Kueue is installed in. The Datadog chart expects the
	// Kueue metrics service there by default (datadog.kueue.metricsService.namespace).
	DefaultNamespace = "kueue-system"
	// DefaultClusterQueue is the name of the ClusterQueue every LocalQueue points to.
	DefaultClusterQueue = "e2e-cluster-queue"
	// DefaultResourceFlavor is the name of the ResourceFlavor of the ClusterQueue.
	DefaultResourceFlavor = "default-flavor"

	// QueueNameLabel is the Job (or pod) label selecting the LocalQueue of a workload.
	QueueNameLabel = "kueue.x-k8s.io/queue-name"
)

// LocalQueue is a Kueue LocalQueue, created in its own namespace.
type LocalQueue struct {
	Namespace string
	Name      string
}

// Params holds the configuration of a Kueue installation.
type Params struct {
	Version        string
	Namespace      string
	ClusterQueue   string
	ResourceFlavor string
	// Quotas are the nominal quotas of the ClusterQueue, by resource name.
	Quotas map[string]string
	// LocalQueues point workload namespaces to the ClusterQueue. Their namespaces are created.
	LocalQueues []LocalQueue
}

// Option configures a Kueue installation.
type Option = func(*Params) error

// NewParams returns the Kueue parameters with defaults applied.
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		Version:        DefaultVersion,
		Namespace:      DefaultNamespace,
		ClusterQueue:   DefaultClusterQueue,
		ResourceFlavor: DefaultResourceFlavor,
		Quotas: map[string]string{
			"cpu":            "8",
			"memory":         "16Gi",
			"nvidia.com/gpu": "8",
		},
	}
	return common.ApplyOption(params, options)
}

// WithVersion overrides the pinned Kueue chart version.
func WithVersion(version string) Option {
	return func(p *Params) error {
		p.Version = version
		return nil
	}
}

// WithQuotas overrides the nominal quotas of the ClusterQueue, by resource name. Workloads
// requesting a resource missing from the quotas are never admitted.
func WithQuotas(quotas map[string]string) Option {
	return func(p *Params) error {
		p.Quotas = quotas
		return nil
	}
}

// WithLocalQueue creates a LocalQueue named name in namespace (created too), pointing to the ClusterQueue.
func WithLocalQueue(namespace, name string) Option {
	return func(p *Params) error {
		p.LocalQueues = append(p.LocalQueues, LocalQueue{Namespace: namespace, Name: name})
		return nil
	}
}

// Component is a Kueue installation and its queues.
type Component struct {
	pulumi.ResourceState

	Release *helm.Release
}

// NewInstallation installs Kueue and its queues on the cluster behind kubeProvider.
func NewInstallation(e config.Env, params *Params, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	comp := &Component{}
	if err := e.Ctx().RegisterComponentResource("dd:kueue", "kueue", comp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(comp), pulumi.Provider(kubeProvider), pulumi.DeletedWith(kubeProvider))

	var err error
	comp.Release, err = helm.NewRelease(e.Ctx(), e.CommonNamer().ResourceName("kueue"), &helm.ReleaseArgs{
		Name:            pulumi.String("kueue"),
		Namespace:       pulumi.String(params.Namespace),
		CreateNamespace: pulumi.Bool(true),
		Chart:           pulumi.String(chartRef),
		Version:         pulumi.String(params.Version),
		// The queues below are validated by the Kueue webhooks, which must be serving first.
		Timeout: pulumi.Int(600),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("kueue: release: %w", err)
	}

	flavor, err := apiextensions.NewCustomResource(e.Ctx(), e.CommonNamer().ResourceName("kueue-flavor", params.ResourceFlavor), &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String(apiVersion),
		Kind:       pulumi.String("ResourceFlavor"),
		Metadata:   metav1.ObjectMetaArgs{Name: pulumi.String(params.ResourceFlavor)},
	}, withDependencies(opts, comp.Release)...)
	if err != nil {
		return nil, fmt.Errorf("kueue: resource flavor: %w", err)
	}

	resources := pulumi.Array{}
	coveredResources := pulumi.StringArray{}
	// Sorted, so that the ClusterQueue spec is stable across runs.
	resourceNames := make([]string, 0, len(params.Quotas))
	for name := range params.Quotas {
		resourceNames = append(resourceNames, name)
	}
	slices.Sort(resourceNames)
	for _, name := range resourceNames {
		coveredResources = append(coveredResources, pulumi.String(name))
		resources = append(resources, pulumi.Map{"name": pulumi.String(name), "nominalQuota": pulumi.String(params.Quotas[name])})
	}
	clusterQueue, err := apiextensions.NewCustomResource(e.Ctx(), e.CommonNamer().ResourceName("kueue-cq", params.ClusterQueue), &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String(apiVersion),
		Kind:       pulumi.String("ClusterQueue"),
		Metadata:   metav1.ObjectMetaArgs{Name: pulumi.String(params.ClusterQueue)},
		OtherFields: map[string]interface{}{
			"spec": pulumi.Map{
				// Admit workloads from every namespace.
				"namespaceSelector": pulumi.Map{},
				"resourceGroups": pulumi.Array{pulumi.Map{
					"coveredResources": coveredResources,
					"flavors": pulumi.Array{pulumi.Map{
						"name":      pulumi.String(params.ResourceFlavor),
						"resources": resources,
					}},
				}},
			},
		},
	}, withDependencies(opts, comp.Release, flavor)...)
	if err != nil {
		return nil, fmt.Errorf("kueue: cluster queue: %w", err)
	}

	namespaces := map[string]*corev1.Namespace{}
	for _, lq := range params.LocalQueues {
		ns, ok := namespaces[lq.Namespace]
		if !ok {
			ns, err = corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("kueue-ns", lq.Namespace), &corev1.NamespaceArgs{
				Metadata: metav1.ObjectMetaArgs{Name: pulumi.String(lq.Namespace)},
			}, opts...)
			if err != nil {
				return nil, fmt.Errorf("kueue: namespace %s: %w", lq.Namespace, err)
			}
			namespaces[lq.Namespace] = ns
		}
		_, err = apiextensions.NewCustomResource(e.Ctx(), e.CommonNamer().ResourceName("kueue-lq", lq.Namespace, lq.Name), &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String(apiVersion),
			Kind:       pulumi.String("LocalQueue"),
			Metadata:   metav1.ObjectMetaArgs{Name: pulumi.String(lq.Name), Namespace: pulumi.String(lq.Namespace)},
			OtherFields: map[string]interface{}{
				"spec": pulumi.Map{"clusterQueue": pulumi.String(params.ClusterQueue)},
			},
		}, withDependencies(opts, comp.Release, clusterQueue, ns)...)
		if err != nil {
			return nil, fmt.Errorf("kueue: local queue %s/%s: %w", lq.Namespace, lq.Name, err)
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

// AgentHelmValues returns the Datadog Helm chart values enabling the Kueue integration: the
// Cluster Agent collects Kueue objects (with the RBAC it needs), the node Agents tag pods with
// them, and the Kueue check runs as a cluster check against the controller.
func AgentHelmValues() string {
	return `datadog:
  kueue:
    enabled: true
`
}
