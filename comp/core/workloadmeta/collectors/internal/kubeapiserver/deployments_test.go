// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func TestDeploymentParser_Parse(t *testing.T) {
	excludeAnnotations := []string{"ignore-annotation"}

	tests := []struct {
		name       string
		expected   []workloadmeta.Entity
		deployment *appsv1.Deployment
	}{
		{
			name: "everything",
			expected: []workloadmeta.Entity{
				&workloadmeta.KubernetesDeployment{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesDeployment,
						ID:   "test-namespace/test-deployment",
					},
					Env:     "env",
					Service: "service",
					Version: "version",
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label":                 "test-value",
							"tags.datadoghq.com/env":     "env",
							"tags.datadoghq.com/service": "service",
							"tags.datadoghq.com/version": "version",
						},
						Annotations: map[string]string{
							"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
							"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
						},
					},
					InjectableLanguages: langUtil.ContainersLanguages{
						*langUtil.NewInitContainer("nginx-cont"): {
							langUtil.Language(languagemodels.Go):     {},
							langUtil.Language(languagemodels.Java):   {},
							langUtil.Language(languagemodels.Python): {},
						},
						*langUtil.NewContainer("nginx-cont"): {
							langUtil.Language(languagemodels.Go):     {},
							langUtil.Language(languagemodels.Java):   {},
							langUtil.Language(languagemodels.Python): {},
						},
					},
				},

				&workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesMetadata,
						ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "test-namespace", "test-deployment")),
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label":                 "test-value",
							"tags.datadoghq.com/env":     "env",
							"tags.datadoghq.com/service": "service",
							"tags.datadoghq.com/version": "version",
						},
						Annotations: map[string]string{
							"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
							"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
						},
					},
					GVR: &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
				},
			},

			deployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label":                 "test-value",
						"tags.datadoghq.com/env":     "env",
						"tags.datadoghq.com/service": "service",
						"tags.datadoghq.com/version": "version",
					},
					Annotations: map[string]string{
						"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
						"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
						"ignore-annotation": "ignore",
					},
				},
			},
		},
		{
			name: "only usm",
			expected: []workloadmeta.Entity{
				&workloadmeta.KubernetesDeployment{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesDeployment,
						ID:   "test-namespace/test-deployment",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label":                 "test-value",
							"tags.datadoghq.com/env":     "env",
							"tags.datadoghq.com/service": "service",
							"tags.datadoghq.com/version": "version",
						},
						Annotations: map[string]string{},
					},
					Env:                 "env",
					Service:             "service",
					Version:             "version",
					InjectableLanguages: make(langUtil.ContainersLanguages),
				},
				&workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesMetadata,
						ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "test-namespace", "test-deployment")),
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label":                 "test-value",
							"tags.datadoghq.com/env":     "env",
							"tags.datadoghq.com/service": "service",
							"tags.datadoghq.com/version": "version",
						},
						Annotations: map[string]string{},
					},
					GVR: &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
				},
			},
			deployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label":                 "test-value",
						"tags.datadoghq.com/env":     "env",
						"tags.datadoghq.com/service": "service",
						"tags.datadoghq.com/version": "version",
					},
					Annotations: map[string]string{
						"ignore-annotation": "ignore",
					},
				},
			},
		},

		{
			name: "only languages",
			expected: []workloadmeta.Entity{
				&workloadmeta.KubernetesDeployment{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesDeployment,
						ID:   "test-namespace/test-deployment",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label": "test-value",
						},
						Annotations: map[string]string{
							"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
							"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
						},
					},
					InjectableLanguages: langUtil.ContainersLanguages{
						*langUtil.NewInitContainer("nginx-cont"): {
							langUtil.Language(languagemodels.Go):     {},
							langUtil.Language(languagemodels.Java):   {},
							langUtil.Language(languagemodels.Python): {},
						},
						*langUtil.NewContainer("nginx-cont"): {
							langUtil.Language(languagemodels.Go):     {},
							langUtil.Language(languagemodels.Java):   {},
							langUtil.Language(languagemodels.Python): {},
						},
					},
				},
				&workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesMetadata,
						ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "test-namespace", "test-deployment")),
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"test-label": "test-value",
						},
						Annotations: map[string]string{
							"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
							"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
						},
					},
					GVR: &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
				},
			},
			deployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label": "test-value",
					},
					Annotations: map[string]string{
						"ignore-annotation": "ignore",
						"internal.dd.datadoghq.com/nginx-cont.detected_langs":      "go,java,  python  ",
						"internal.dd.datadoghq.com/init.nginx-cont.detected_langs": "go,java,  python  ",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := newdeploymentParser(excludeAnnotations)
			require.NoError(t, err)
			parsedEntities := parser.Parse(tt.deployment)

			// Assert that entities are correctly generated
			assert.ElementsMatch(t, tt.expected, parsedEntities)

			// Assert that Annotations and Labels of all entities refer to the same address in memory
			deploymentEntity, ok := parsedEntities[0].(*workloadmeta.KubernetesDeployment)
			require.True(t, ok)

			metadataEntity, ok := parsedEntities[1].(*workloadmeta.KubernetesMetadata)
			require.True(t, ok)

			assert.Truef(t, sameInMemory(deploymentEntity.Annotations, metadataEntity.Annotations), "parsed annotations are duplicated in memory")
			assert.True(t, sameInMemory(deploymentEntity.Labels, metadataEntity.Labels), "parsed labels are duplicated in memory")
		})
	}
}

func Test_DeploymentsFakeKubernetesClient(t *testing.T) {
	tests := []struct {
		name           string
		createResource func(cl *fake.Clientset) error
		deployment     *workloadmeta.KubernetesDeployment
		expected       workloadmeta.EventBundle
	}{
		{
			name: "has env label",
			createResource: func(cl *fake.Clientset) error {
				_, err := cl.AppsV1().Deployments("test-namespace").Create(
					context.TODO(),
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
							Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
						}},
					metav1.CreateOptions{},
				)
				return err
			},
			expected: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{
						Type: workloadmeta.EventTypeSet,
						Entity: &workloadmeta.KubernetesDeployment{
							EntityID: workloadmeta.EntityID{
								ID:   "test-namespace/test-deployment",
								Kind: workloadmeta.KindKubernetesDeployment,
							},
							EntityMeta: workloadmeta.EntityMeta{
								Name:      "test-deployment",
								Namespace: "test-namespace",
								Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
							},
							Env:                 "env",
							InjectableLanguages: make(langUtil.ContainersLanguages),
						},
					},
					{
						Type: workloadmeta.EventTypeSet,
						Entity: &workloadmeta.KubernetesMetadata{
							EntityID: workloadmeta.EntityID{
								ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "test-namespace", "test-deployment")),
								Kind: workloadmeta.KindKubernetesMetadata,
							},
							EntityMeta: workloadmeta.EntityMeta{
								Name:      "test-deployment",
								Namespace: "test-namespace",
								Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
							},
							GVR: &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
						},
					},
				},
			},
		},

		{
			name: "has language annotation",
			createResource: func(cl *fake.Clientset) error {
				_, err := cl.AppsV1().Deployments("test-namespace").Create(
					context.TODO(),
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
							Annotations: map[string]string{"test-label": "test-value",
								"internal.dd.datadoghq.com/nginx.detected_langs":      "go,java",
								"internal.dd.datadoghq.com/init.redis.detected_langs": "go,python"},
						}},
					metav1.CreateOptions{},
				)
				return err
			},
			expected: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{
						Type: workloadmeta.EventTypeSet,
						Entity: &workloadmeta.KubernetesDeployment{
							EntityID: workloadmeta.EntityID{
								ID:   "test-namespace/test-deployment",
								Kind: workloadmeta.KindKubernetesDeployment,
							},
							EntityMeta: workloadmeta.EntityMeta{
								Name:      "test-deployment",
								Namespace: "test-namespace",
								Annotations: map[string]string{"test-label": "test-value",
									"internal.dd.datadoghq.com/nginx.detected_langs":      "go,java",
									"internal.dd.datadoghq.com/init.redis.detected_langs": "go,python",
								},
							},
							InjectableLanguages: langUtil.ContainersLanguages{
								*langUtil.NewContainer("nginx"): {
									langUtil.Language(languagemodels.Go):   {},
									langUtil.Language(languagemodels.Java): {},
								},
								*langUtil.NewInitContainer("redis"): {
									langUtil.Language(languagemodels.Go):     {},
									langUtil.Language(languagemodels.Python): {},
								},
							},
						},
					},
					{
						Type: workloadmeta.EventTypeSet,
						Entity: &workloadmeta.KubernetesMetadata{
							EntityID: workloadmeta.EntityID{
								ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "test-namespace", "test-deployment")),
								Kind: workloadmeta.KindKubernetesMetadata,
							},
							EntityMeta: workloadmeta.EntityMeta{
								Name:      "test-deployment",
								Namespace: "test-namespace",
								Annotations: map[string]string{"test-label": "test-value",
									"internal.dd.datadoghq.com/nginx.detected_langs":      "go,java",
									"internal.dd.datadoghq.com/init.redis.detected_langs": "go,python",
								},
							},
							GVR: &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCollectEvent(t, tt.createResource, newDeploymentStore, tt.expected)
		})
	}
}
