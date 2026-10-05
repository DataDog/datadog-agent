// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sbomtargets deploys container images used as targets for SBOM scanning.
package sbomtargets

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Namespace is where the SBOM target workloads are deployed.
const Namespace = "sbom-workloads"

// Target is an SBOM scanning target image.
type Target struct {
	// Name is the workload (deployment) name.
	Name string
	// Image is the pinned container image reference.
	Image string
	// ShortImage is the image name without registry/tag, used by tests to match SBOM payloads.
	ShortImage string
}

// Targets are the images scanned for SBOMs. Each is pinned by tag AND digest
// (repo:tag@sha256:...) so every run scans byte-identical image content, which
// keeps the component set (and therefore the counts) deterministic, while the
// tag still yields a RepoTag/image_tag in the SBOM. The pinned digest is the
// same one the test asserts as the expected RepoDigest; the test also
// cross-checks the resolved ImageID and DiffIDs, so any registry drift surfaces
// as a failure. They cover distinct package ecosystems:
//
//	node          - Debian (dpkg) + npm
//	golang-alpine - Alpine (apk, musl) + Go
//	ubi9          - RHEL (rpm), single-layer image
//	ubi9/python   - RHEL (rpm) + pip (pypi), multi-layer image
//	python        - Debian (dpkg) + pip (pypi)
//	ruby          - Debian (dpkg) + gem (rubygems)
//	ubuntu        - Ubuntu 24.04 LTS (dpkg), usr-merged
//	alpine        - Alpine (apk, musl), apk-owned setuid ping
var Targets = []Target{
	{Name: "sbom-node", Image: "node:26.2.0@sha256:980c5420a7a2ddcb44037726977f2a349e5c7b64217516c7488dce4c74d71583", ShortImage: "node"},
	{Name: "sbom-golang", Image: "golang:1.26.3-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d", ShortImage: "golang"},
	{Name: "sbom-ubi9", Image: "registry.access.redhat.com/ubi9/ubi:9.8-1780376557@sha256:80b1f4c34a7eed1b03a05d12b55768f3e522eef6ec294c6fbd5fa47b6b2892ee", ShortImage: "ubi"},                     // RHEL rpm, single-layer
	{Name: "sbom-ubi-python", Image: "registry.access.redhat.com/ubi9/python-312:9.8-1779945122@sha256:52d1ffcda3b9552934f947b7d41fb0cb66973bdc0d7e91814facadc126f68663", ShortImage: "python-312"}, // RHEL rpm + pypi, multi-layer
	{Name: "sbom-python", Image: "python:3.14.5@sha256:250e5c97be05e1eb2272fbdbd810dfd638f9012e1e6f65c99390ad3239943a08", ShortImage: "python"},
	{Name: "sbom-ruby", Image: "ruby:3.3.4-bookworm@sha256:d4233f4242ea25346f157709bb8417c615e7478468e2699c8e86a4e1f0156de8", ShortImage: "ruby"},
	{Name: "sbom-ubuntu", Image: "ubuntu:24.04@sha256:786a8b558f7be160c6c8c4a54f9a57274f3b4fb1491cf65146521ae77ff1dc54", ShortImage: "ubuntu"},                              // Ubuntu 24.04 LTS (dpkg), usr-merged
	{Name: "sbom-alpine", Image: "wbitt/network-multitool:latest@sha256:db2810fe2c8d36db074eab5d98fbf861c8ed55e0786d648d3477b3de9135632e", ShortImage: "network-multitool"}, // Alpine (apk, musl), apk-owned setuid ping
}

// K8sAppDefinition deploys one Deployment per SBOM target. Each container is kept
// alive with `tail -f /dev/null` so its image stays resident in the host
// containerd for the Agent to scan. `sleep infinity` is not used because the
// busybox sleep in the golang:alpine target rejects it and crash loops the pod.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "sbom-targets", k8sComponent, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), Namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(Namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	var imagePullSecrets corev1.LocalObjectReferenceArray
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err := utils.NewImagePullSecret(e, Namespace, opts...)
		if err != nil {
			return nil, err
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReferenceArgs{
			Name: imgPullSecret.Metadata.Name(),
		})
	}

	for _, t := range Targets {
		if _, err := appsv1.NewDeployment(e.Ctx(), t.Name, &appsv1.DeploymentArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(t.Name),
				Namespace: pulumi.String(Namespace),
				Labels:    pulumi.StringMap{"app": pulumi.String(t.Name)},
			},
			Spec: &appsv1.DeploymentSpecArgs{
				Replicas: pulumi.Int(1),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{"app": pulumi.String(t.Name)},
				},
				Template: &corev1.PodTemplateSpecArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Labels: pulumi.StringMap{"app": pulumi.String(t.Name)},
					},
					Spec: &corev1.PodSpecArgs{
						ImagePullSecrets: imagePullSecrets,
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:    pulumi.String("main"),
								Image:   pulumi.String(mirroredImage(e, t.Image)),
								Command: pulumi.StringArray{pulumi.String("tail"), pulumi.String("-f"), pulumi.String("/dev/null")},
								Resources: &corev1.ResourceRequirementsArgs{
									Limits: pulumi.StringMap{
										"cpu":    pulumi.String("100m"),
										"memory": pulumi.String("64Mi"),
									},
									Requests: pulumi.StringMap{
										"cpu":    pulumi.String("10m"),
										"memory": pulumi.String("32Mi"),
									},
								},
							},
						},
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	}

	return k8sComponent, nil
}

// mirroredImage routes a docker.io image reference through the internal ECR
// mirror on the datadog-agent-qa account when an image pull registry is
// configured. Upstream registries (docker.io especially) have been a recurring
// source of pull flakiness in e2e, so pulling through the mirror is more
// reliable. References that already carry an explicit registry host (for example
// the ubi images on registry.access.redhat.com) are left unchanged, as are runs
// with no configured registry (local, where the mirror is not reachable).
func mirroredImage(e config.Env, image string) string {
	reg := strings.SplitN(e.ImagePullRegistry(), ",", 2)[0]
	if reg == "" {
		return image
	}
	// A component before the first "/" that contains "." or ":" is a registry
	// host, so the reference is not a docker.io image and must be left alone.
	if i := strings.IndexByte(image, '/'); i >= 0 && strings.ContainsAny(image[:i], ".:") {
		return image
	}
	// docker.io official images (no namespace) live under library/.
	repo := image
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return reg + "/dockerhub/" + repo
}
