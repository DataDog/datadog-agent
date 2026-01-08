// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dda

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	baseName = "dda"
)

type datadogAgentWorkload struct {
	ctx             *pulumi.Context
	opts            *agentwithoperatorparams.Params
	name            string
	clusterName     string
	imagePullSecret *corev1.Secret
}

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, params *agentwithoperatorparams.Params, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	if params == nil {
		return nil, nil
	}
	apiKey := e.AgentAPIKey()
	appKey := e.AgentAPPKey()
	clusterName := e.Ctx().Stack()

	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:agent-with-operator", "dda", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	// Create datadog-credentials secret if necessary
	secret, err := corev1.NewSecret(e.Ctx(), "datadog-credentials", &corev1.SecretArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: pulumi.String(params.Namespace),
			Name:      pulumi.Sprintf("%s-datadog-credentials", baseName),
		},
		StringData: pulumi.StringMap{
			"api-key": apiKey,
			"app-key": appKey,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(secret))

	// Create imagePullSecret
	var imagePullSecret *corev1.Secret
	if e.ImagePullRegistry() != "" {
		imagePullSecret, err = utils.NewImagePullSecret(e, params.Namespace, opts...)
		if err != nil {
			return nil, err
		}
		opts = append(opts, utils.PulumiDependsOn(imagePullSecret))
	}

	ddaWorkload := datadogAgentWorkload{
		ctx:             e.Ctx(),
		opts:            params,
		name:            params.DDAConfig.Name,
		clusterName:     clusterName,
		imagePullSecret: imagePullSecret,
	}

	if err = ddaWorkload.buildDDAConfig(opts...); err != nil {
		e.Ctx().Log.Debug(fmt.Sprintf("Error building DDA config: %v", err), nil)
		return nil, err
	}

	return k8sComponent, nil
}

func (d datadogAgentWorkload) buildDDAConfig(opts ...pulumi.ResourceOption) error {
	ctx := d.ctx
	defaultYamlTransformations := d.defaultDDAYamlTransformations()

	if d.opts.DDAConfig.YamlFilePath != "" {
		_, err := yaml.NewConfigGroup(ctx, d.name, &yaml.ConfigGroupArgs{
			Files:           []string{d.opts.DDAConfig.YamlFilePath},
			Transformations: defaultYamlTransformations,
		}, opts...)

		if err != nil {
			d.ctx.Log.Debug(fmt.Sprintf("Error transforming DDAConfig yaml file path: %v", err), nil)
			return err
		}
	} else if d.opts.DDAConfig.YamlConfig != "" {
		_, err := yaml.NewConfigGroup(ctx, d.name, &yaml.ConfigGroupArgs{
			YAML:            []string{d.opts.DDAConfig.YamlConfig},
			Transformations: defaultYamlTransformations,
		}, opts...)

		if err != nil {
			d.ctx.Log.Debug(fmt.Sprintf("Error transforming DDAConfig yaml: %v", err), nil)
			return err
		}
	} else if d.opts.DDAConfig.MapConfig != nil {
		_, err := yaml.NewConfigGroup(ctx, d.name, &yaml.ConfigGroupArgs{
			Objs:            []map[string]interface{}{d.opts.DDAConfig.MapConfig},
			Transformations: defaultYamlTransformations,
		}, opts...)

		if err != nil {
			d.ctx.Log.Debug(fmt.Sprintf("Error transforming DDAConfig map config: %v", err), nil)
			return err
		}
	} else {
		_, err := yaml.NewConfigGroup(ctx, d.name, &yaml.ConfigGroupArgs{
			Objs:            []map[string]interface{}{d.defaultDDAConfig()},
			Transformations: defaultYamlTransformations,
		}, opts...)

		if err != nil {
			d.ctx.Log.Debug(fmt.Sprintf("Error creating default DDA config: %v", err), nil)
			return err
		}

	}
	return nil
}

func (d datadogAgentWorkload) defaultDDAConfig() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "datadoghq.com/v2alpha1",
		"kind":       "DatadogAgent",
		"metadata": map[string]interface{}{
			"name":      d.opts.DDAConfig.Name,
			"namespace": d.opts.Namespace,
		},
		"spec": map[string]interface{}{
			"features": map[string]interface{}{
				"clusterChecks": map[string]interface{}{
					"enabled":                 true,
					"useClusterChecksRunners": true,
				},
			},
		},
	}
}

func (d datadogAgentWorkload) fakeIntakeEnvVars() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":  "DD_DD_URL",
			"value": d.opts.FakeIntake.URL,
		},
		{
			"name":  "DD_PROCESS_CONFIG_PROCESS_DD_URL",
			"value": d.opts.FakeIntake.URL,
		},
		{
			"name":  "DD_APM_DD_URL",
			"value": d.opts.FakeIntake.URL,
		},
		{
			"name":  "DD_LOGS_CONFIG_LOGS_DD_URL",
			"value": d.opts.FakeIntake.URL,
		},
		{
			"name":  "DD_LOGS_CONFIG_USE_HTTP",
			"value": "true",
		},
		{
			"name":  "DD_SERVICE_DISCOVERY_FORWARDER_LOGS_DD_URL",
			"value": d.opts.FakeIntake.URL,
		},
		{
			"name":  "DD_SKIP_SSL_VALIDATION",
			"value": "true",
		},
		{
			"name":  "DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION",
			"value": "true",
		},
	}
}

func (d datadogAgentWorkload) defaultDDAYamlTransformations() []yaml.Transformation {
	return []yaml.Transformation{
		// Configure metadata
		func(state map[string]interface{}, _ ...pulumi.ResourceOption) {
			defaultMetadata := map[string]interface{}{
				"name":      d.opts.DDAConfig.Name,
				"namespace": d.opts.Namespace,
			}
			if state["metadata"] == nil {
				state["metadata"] = defaultMetadata
			} else {
				stateMetadata := state["metadata"].(map[string]interface{})
				err := mergo.Merge(&stateMetadata, defaultMetadata)
				if err != nil {
					d.ctx.Log.Debug(fmt.Sprintf("Error merging DDA metadata YAML: %v", err), nil)
				}

			}
		},
		// Configure global
		func(state map[string]interface{}, _ ...pulumi.ResourceOption) {
			defaultGlobal := map[string]interface{}{
				"clusterName": d.clusterName,
				"credentials": map[string]interface{}{
					"apiSecret": map[string]interface{}{
						"secretName": baseName + "-datadog-credentials",
						"keyName":    "api-key",
					},
					"appSecret": map[string]interface{}{
						"secretName": baseName + "-datadog-credentials",
						"keyName":    "app-key",
					},
				},
			}
			if state["spec"].(map[string]interface{})["global"] == nil {
				state["spec"].(map[string]interface{})["global"] = defaultGlobal
			} else {
				stateGlobal := state["spec"].(map[string]interface{})["global"].(map[string]interface{})
				err := mergo.Map(&stateGlobal, defaultGlobal)
				if err != nil {
					d.ctx.Log.Debug(fmt.Sprintf("Error merging DDA global YAML: %v", err), nil)
				}
			}
		},
		// Configure Fake Intake
		func(state map[string]interface{}, _ ...pulumi.ResourceOption) {
			if d.opts.FakeIntake == nil {
				return
			}
			fakeIntakeOverride := map[string]interface{}{
				"nodeAgent": map[string]interface{}{
					"env": d.fakeIntakeEnvVars(),
				},
				"clusterAgent": map[string]interface{}{
					"env": d.fakeIntakeEnvVars(),
				},
				"clusterChecksRunner": map[string]interface{}{
					"env": d.fakeIntakeEnvVars(),
				},
			}
			if state["spec"].(map[string]interface{})["override"] == nil {
				state["spec"].(map[string]interface{})["override"] = fakeIntakeOverride
			} else {
				stateOverride := state["spec"].(map[string]interface{})["override"].(map[string]interface{})
				err := mergo.Map(&stateOverride, fakeIntakeOverride)
				if err != nil {
					d.ctx.Log.Debug(fmt.Sprintf("Error merging fakeintake override YAML: %v", err), nil)
				}
			}
		},
		//	Configure Image pull secret
		func(state map[string]interface{}, _ ...pulumi.ResourceOption) {
			if d.imagePullSecret == nil {
				return
			}

			imgPullSecretOverride := map[string]interface{}{
				"nodeAgent": map[string]interface{}{
					"image": map[string]interface{}{
						"pullSecrets": []map[string]interface{}{
							{
								"name": d.imagePullSecret.Metadata.Name(),
							},
						},
					},
				},
				"clusterAgent": map[string]interface{}{
					"image": map[string]interface{}{
						"pullSecrets": []map[string]interface{}{
							{
								"name": d.imagePullSecret.Metadata.Name(),
							},
						},
					},
				},
				"clusterChecksRunner": map[string]interface{}{
					"image": map[string]interface{}{
						"pullSecrets": []map[string]interface{}{
							{
								"name": d.imagePullSecret.Metadata.Name(),
							},
						},
					},
				},
			}

			if state["spec"].(map[string]interface{})["override"] == nil {
				state["spec"].(map[string]interface{})["override"] = imgPullSecretOverride
			} else {
				stateOverride := state["spec"].(map[string]interface{})["override"].(map[string]interface{})
				err := mergo.Map(&stateOverride, imgPullSecretOverride)
				if err != nil {
					d.ctx.Log.Debug(fmt.Sprintf("Error merging imagePullSecrets override YAML: %v", err), nil)
				}
			}
		},
	}
}
