// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agent

import (
	"testing"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubernetesSubchartImages(t *testing.T) {
	public := HelmValues{}
	public.configureKubernetesImages("")
	assert.Empty(t, public)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		values := HelmValues{}
		values.configureKubernetesImages("cache.example.com,another.example.com")
		secret, err := corev1.NewSecret(ctx, "pull-secret", &corev1.SecretArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("pull-secret")}})
		if err != nil {
			return err
		}
		values.configureImagePullSecret(secret)
		pulumi.All(pulumi.Map(values)).ApplyT(func(args []interface{}) error {
			resolved := args[0].(map[string]interface{})["datadog-csi-driver"].(map[string]interface{})
			image := resolved["registrar"].(map[string]interface{})["image"].(map[string]interface{})
			assert.Equal(t, "cache.example.com/k8s/sig-storage/csi-node-driver-registrar", image["repository"])
			assert.NotContains(t, image, "tag")
			assert.Equal(t, []map[string]interface{}{{"name": new("pull-secret")}}, resolved["image"].(map[string]interface{})["pullSecrets"])
			return nil
		})
		return nil
	}, pulumi.WithMocks("project", "stack", noopMocks{}))
	require.NoError(t, err)
}
