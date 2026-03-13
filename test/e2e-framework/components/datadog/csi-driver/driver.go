// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	"errors"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

const (
	csiRepository       = "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/csi-driver"
	registrarRepository = " 669783387624.dkr.ecr.us-east-1.amazonaws.com/sig-storage/csi-node-driver-registrar"
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

	var imgPullSecret *corev1.Secret
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err = utils.NewImagePullSecret(e, CSINamespace, opts...)
		if err != nil {
			return err
		}
		opts = append(opts, utils.PulumiDependsOn(imgPullSecret))
	}

	if imgPullSecret == nil {
		return errors.New("nil imgPullSecret")
	}

	params := &Params{
		HelmValues: HelmValues{
			"registrar": pulumi.Map{
				"image": pulumi.Map{
					"repository": pulumi.String(registrarRepository),
				},
			},
			"image": pulumi.Map{
				"repository": pulumi.String(csiRepository),
				"tag":        pulumi.String(csiDriverTag),
				"pullSecrets": pulumi.MapArray{
					pulumi.Map{
						"name": imgPullSecret.Metadata.Name(),
					},
				},
			},
		},
		Version: "0.3.1",
	}

	_, err = NewHelmInstallation(e, params, opts...)
	if err != nil {
		return err
	}
	return nil
}
