// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"reflect"
	"testing"

	"go.uber.org/fx"
	fakek8sclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Test_webhook_isDeprecated(t *testing.T) {
	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		want deprecationInfoType
	}{
		{
			name: "deprecated HPA v2beta2",
			gvk:  schema.GroupVersionKind{Group: "autoscaling", Version: "v2beta2", Kind: "HorizontalPodAutoscaler"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 23},
				removalVersion:         semVersionType{Major: 1, Minor: 26},
				recommendedReplacement: schema.GroupVersionKind{Kind: "HorizontalPodAutoscaler", Version: "v2", Group: "autoscaling"},
			},
		},
		{
			name: "non deprecated HPA v2",
			gvk:  schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"},
			want: deprecationInfoType{
				isDeprecated: false,
			},
		},
		{
			name: "non deprecated CronJob v1beta1",
			gvk:  schema.GroupVersionKind{Group: "batch", Version: "v1beta1", Kind: "CronJob"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 21},
				removalVersion:         semVersionType{Major: 1, Minor: 25},
				recommendedReplacement: schema.GroupVersionKind{Kind: "CronJob", Version: "v1", Group: "batch"},
			},
		},
		{
			name: "non deprecated extensions Ingress v1beta1",
			gvk:  schema.GroupVersionKind{Group: "extensions", Version: "v1beta1", Kind: "Ingress"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 14},
				removalVersion:         semVersionType{Major: 1, Minor: 22},
				recommendedReplacement: schema.GroupVersionKind{Kind: "Ingress", Version: "v1", Group: "networking.k8s.io"},
			},
		},
		{
			name: "deprecated PriorityLevelConfiguration v1beta3",
			gvk:  schema.GroupVersionKind{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta3", Kind: "PriorityLevelConfiguration"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 29},
				removalVersion:         semVersionType{Major: 1, Minor: 32},
				recommendedReplacement: schema.GroupVersionKind{Kind: "PriorityLevelConfiguration", Version: "v1", Group: "flowcontrol.apiserver.k8s.io"},
			},
		},
		{
			name: "deprecated PriorityLevelConfiguration v1beta1",
			gvk:  schema.GroupVersionKind{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta1", Kind: "PriorityLevelConfiguration"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 23},
				removalVersion:         semVersionType{Major: 1, Minor: 26},
				recommendedReplacement: schema.GroupVersionKind{Kind: "PriorityLevelConfiguration", Version: "v1beta3", Group: "flowcontrol.apiserver.k8s.io"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock demultiplexer and sender
			demultiplexerMock := createDemultiplexer(t)
			mockSender := mocksender.NewMockSenderWithSenderManager(eventType, demultiplexerMock)
			err := demultiplexerMock.SetSender(mockSender, eventType)
			assert.NoError(t, err)

			// Mock Datadog Config
			datadogConfigMock := fxutil.Test[config.Component](t, core.MockBundle())
			datadogConfigMock.SetWithoutSource("kubernetes_deprecated_resources_collection.enabled", true)

			// Mock CRD client
			fakeClient := fakek8sclient.NewSimpleClientset()
			w := NewWebhook(datadogConfigMock, demultiplexerMock, fakeClient, false).(*webhook)
			if got := w.isDeprecated(tt.gvk); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("webhook.isDeprecated() = %v, want %v", got, tt.want)
			}
		})
	}
}

// createDemultiplexer creates a demultiplexer for testing
func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t, fx.Provide(func() log.Component { return logmock.New(t) }), logscompression.MockModule(), metricscompression.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
}
