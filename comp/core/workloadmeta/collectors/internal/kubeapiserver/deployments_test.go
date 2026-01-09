// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func Test_DeploymentsFakeKubernetesClient(t *testing.T) {
	tests := []struct {
		name          string
		createObjects func() []runtime.Object
		deployment    *workloadmeta.KubernetesDeployment
		expected      workloadmeta.EventBundle
	}{
		{
			name: "has env label",
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&metav1.PartialObjectMetadata{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
							Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
						},
					},
				}
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
							InjectableLanguages: make(languagemodels.ContainersLanguages),
						},
					},
				},
			},
		},

		{
			name: "has language annotation",
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&metav1.PartialObjectMetadata{
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
						},
					},
				}
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
							InjectableLanguages: languagemodels.ContainersLanguages{
								*languagemodels.NewContainer("nginx"): {
									languagemodels.Go:   {},
									languagemodels.Java: {},
								},
								*languagemodels.NewInitContainer("redis"): {
									languagemodels.Go:     {},
									languagemodels.Python: {},
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
			t.Parallel()

			newDeploymentStoreFunc := func(ctx context.Context, wlm workloadmeta.Component, cfg config.Reader, metadataclient metadata.Interface, _ schema.GroupVersionResource) (*cache.Reflector, *reflectorStore) {
				return newDeploymentStore(ctx, wlm, cfg, nil, metadataclient)
			}

			testCollectMetadataEventWithStore(t, tt.createObjects, deploymentsGVR, newDeploymentStoreFunc, tt.expected)
		})
	}
}
