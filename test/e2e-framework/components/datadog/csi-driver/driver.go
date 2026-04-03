// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

func NewDatadogCSIDriver(e config.Env, kubeProvider *kubernetes.Provider, csiDriverTag string) error {
	opts := []pulumi.ResourceOption{pulumi.Providers(kubeProvider), pulumi.DeletedWith(kubeProvider)}

	// Create namespace if necessary
	ns, err := corev1.NewNamespace(e.Ctx(), CSINamespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(CSINamespace),
		},
	}, opts...)
	if err != nil {
		return err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	csiRepo := "docker.io/datadog/csi-driver"
	registrarRepo := "registry.k8s.io/sig-storage/csi-node-driver-registrar"

	var imgPullSecret *corev1.Secret
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err = utils.NewImagePullSecret(e, CSINamespace, opts...)
		if err != nil {
			return err
		}
		opts = append(opts, utils.PulumiDependsOn(imgPullSecret))
		reg := strings.SplitN(e.ImagePullRegistry(), ",", 2)[0]
		csiRepo = reg + "/dockerhub/datadog/csi-driver"
		registrarRepo = reg + "/sig-storage/csi-node-driver-registrar"
	}

	imageMap := pulumi.Map{
		"repository": pulumi.String(csiRepo),
		"tag":        pulumi.String(csiDriverTag),
	}
	if imgPullSecret != nil {
		imageMap["pullSecrets"] = pulumi.MapArray{
			pulumi.Map{
				"name": imgPullSecret.Metadata.Name(),
			},
		}
	}

	params := &Params{
		HelmValues: HelmValues{
			"registrar": pulumi.Map{
				"image": pulumi.Map{
					"repository": pulumi.String(registrarRepo),
				},
			},
			"image": imageMap,
		},
		Version: "0.3.1",
	}

	_, err = NewHelmInstallation(e, params, opts...)
	if err != nil {
		return err
	}
	return nil
}
