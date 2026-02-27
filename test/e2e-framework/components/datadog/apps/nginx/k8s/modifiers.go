// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package k8s

import (
	"errors"
	"maps"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeploymentModifier is a function that operates on a DeploymentArgs struct
type DeploymentModifier func(args *appsv1.DeploymentArgs) error

// mergeStringMaps merges two pulumi.StringMaps into one
func mergeStringMaps(left, right pulumi.StringMap) pulumi.StringMap {
	// get everything from left map
	merged := maps.Clone(left)

	// get everything from right map, this will overwrite if
	// keys are duplicated
	maps.Copy(merged, right)

	return merged
}

// ensureDeploymentPodTemplateSpec performs nil and type checks
// on a DeploymentArgs struct and returns PodSpecArgs
func ensureDeploymentPodTemplateSpec(d *appsv1.DeploymentArgs) (*corev1.PodSpecArgs, error) {
	// nil check spec
	if d.Spec == nil {
		d.Spec = &appsv1.DeploymentSpecArgs{}
	}
	deploymentSpecPtr := d.Spec

	// type check spec
	spec, ok := deploymentSpecPtr.(*appsv1.DeploymentSpecArgs)
	if !ok {
		return nil, errors.New("type check failed for spec")
	}

	// nil check spec.Template
	if spec.Template == nil {
		spec.Template = &corev1.PodTemplateSpecArgs{}
	}
	podTemplatePtr := spec.Template

	// type check spec.Template
	podTemplate, ok := podTemplatePtr.(*corev1.PodTemplateSpecArgs)
	if !ok {
		return nil, errors.New("type check failed for spec.Template")
	}

	// nil check spec.Template.Spec
	if podTemplate.Spec == nil {
		podTemplate.Spec = &corev1.PodSpecArgs{}
	}
	podTemplateSpecPtr := podTemplate.Spec

	// type check spec.Template.Spec
	podTemplateSpec, ok := podTemplateSpecPtr.(*corev1.PodSpecArgs)
	if !ok {
		return nil, errors.New("type check failed for spec.Template.Spec")
	}

	return podTemplateSpec, nil
}

// WithRuntimeClass sets a deployment's RuntimeClassName
func WithRuntimeClass(rtc string) DeploymentModifier {
	return func(d *appsv1.DeploymentArgs) error {
		podTemplateSpec, err := ensureDeploymentPodTemplateSpec(d)
		if err != nil {
			return err
		}

		podTemplateSpec.RuntimeClassName = runtimeClassToPulumi(rtc)
		return nil
	}
}

// WithServiceAccount sets a deployment's ServiceAccount
func WithServiceAccount(serviceAccount *corev1.ServiceAccount) DeploymentModifier {
	return func(d *appsv1.DeploymentArgs) error {
		podTemplateSpec, err := ensureDeploymentPodTemplateSpec(d)
		if err != nil {
			return err
		}

		podTemplateSpec.ServiceAccount = serviceAccount.Metadata.Name()
		return nil
	}
}

func ensureDeploymentPodMetadata(d *appsv1.DeploymentArgs) (*metav1.ObjectMetaArgs, error) {
	// nil check spec
	if d.Spec == nil {
		d.Spec = &appsv1.DeploymentSpecArgs{}
	}
	specPtr := d.Spec

	// type check spec
	spec, ok := specPtr.(*appsv1.DeploymentSpecArgs)
	if !ok {
		return nil, errors.New("type check failed for spec")
	}

	// nil check spec.Template
	if spec.Template == nil {
		spec.Template = &corev1.PodTemplateSpecArgs{}
	}
	templatePtr := spec.Template

	// type check spec.Template
	template, ok := templatePtr.(*corev1.PodTemplateSpecArgs)
	if !ok {
		return nil, errors.New("type check failed for spec.Template")
	}

	// nil check template.Metadata
	if template.Metadata == nil {
		template.Metadata = &metav1.ObjectMetaArgs{}
	}
	metadataPtr := template.Metadata

	metadata, ok := metadataPtr.(*metav1.ObjectMetaArgs)
	if !ok {
		return nil, errors.New("type check failed for spec.Template.Metadata")
	}

	return metadata, nil
}

// WithLabels appends/ovewrites a Deployment template's labels
func WithLabels(labels map[string]string) DeploymentModifier {
	return func(d *appsv1.DeploymentArgs) error {
		metadata, err := ensureDeploymentPodMetadata(d)
		if err != nil {
			return err
		}

		// If labels is nil initialize it
		if metadata.Labels == nil {
			metadata.Labels = pulumi.StringMap{}
		}

		// merge the existing labels with new ones
		merged := mergeStringMaps(metadata.Labels.(pulumi.StringMap), pulumi.ToStringMap(labels))

		// reassign
		metadata.Labels = merged
		return nil
	}
}

// WithAnnotations appends/ovewrites a Deployment template's annotations
func WithAnnotations(annotations map[string]string) DeploymentModifier {
	return func(d *appsv1.DeploymentArgs) error {
		metadata, err := ensureDeploymentPodMetadata(d)
		if err != nil {
			return err
		}

		// If annotations is nil initialize it
		if metadata.Annotations == nil {
			metadata.Annotations = pulumi.StringMap{}
		}
		// merge the existing annotations with new ones
		merged := mergeStringMaps(metadata.Annotations.(pulumi.StringMap), pulumi.ToStringMap(annotations))
		// reassign
		metadata.Annotations = merged

		return nil
	}
}

// WithConfigMap adds a ConfigMap volume and volume mount to the nginx container.
func WithConfigMap() DeploymentModifier {
	return func(d *appsv1.DeploymentArgs) error {
		podTemplateSpec, err := ensureDeploymentPodTemplateSpec(d)
		if err != nil {
			return err
		}

		containers, ok := podTemplateSpec.Containers.(corev1.ContainerArray)
		if !ok {
			return errors.New("type check failed for Containers array")
		}

		// Add the ConfigMap volume mount to the nginx container
		if len(containers) > 0 {
			container, ok := containers[0].(*corev1.ContainerArgs)
			if !ok {
				return errors.New("type check failed for container")
			}

			if container.VolumeMounts != nil {
				volumeMounts, ok := container.VolumeMounts.(*corev1.VolumeMountArray)
				if !ok {
					return errors.New("type check failed for VolumeMounts")
				}

				*volumeMounts = append(corev1.VolumeMountArray{
					&corev1.VolumeMountArgs{
						Name:      pulumi.String("conf"),
						MountPath: pulumi.String("/etc/nginx/nginx.conf"),
						SubPath:   pulumi.String("nginx.conf"),
					},
				}, *volumeMounts...)
			}
		}

		volumes, ok := podTemplateSpec.Volumes.(corev1.VolumeArray)
		if !ok {
			return errors.New("type check failed for Volumes array")
		}

		podTemplateSpec.Volumes = append(corev1.VolumeArray{
			&corev1.VolumeArgs{
				Name: pulumi.String("conf"),
				ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
					Name: pulumi.String("nginx"),
				},
			},
		}, volumes...)

		return nil
	}
}
