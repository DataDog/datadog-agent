// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func TestDeploymentParser_Parse(t *testing.T) {
	excludeAnnotations := []string{"ignore-annotation"}

	tests := []struct {
		name       string
		expected   *workloadmeta.KubernetesDeployment
		deployment *appsv1.Deployment
	}{
		{
			name: "everything",
			expected: &workloadmeta.KubernetesDeployment{
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
			expected: &workloadmeta.KubernetesDeployment{
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
			expected: &workloadmeta.KubernetesDeployment{
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
			entity := parser.Parse(tt.deployment)
			storedDeployment, ok := entity.(*workloadmeta.KubernetesDeployment)
			require.True(t, ok)
			assert.Equal(t, tt.expected, storedDeployment)
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
