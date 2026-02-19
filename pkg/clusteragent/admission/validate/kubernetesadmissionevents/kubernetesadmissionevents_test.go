// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesadmissionevents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	sourceTypeName = "kubernetes admission"
	eventType      = "kubernetes_admission_events"
)

// compareText compares text while ignoring the timestamp
func compareText(expected, actual event.Event) bool {
	re := regexp.MustCompile(`(?m)^\*\*Time:\*\*.*$`)
	expectedText := re.ReplaceAllString(expected.Text, "**Time:** <TIME>\n")
	actualText := re.ReplaceAllString(actual.Text, "**Time:** <TIME>\n")
	return expectedText == actualText
}

// TestKubernetesAdmissionEvents tests the KubernetesAdmissionEvents webhook.
func TestKubernetesAdmissionEvents(t *testing.T) {
	// Mock Datadog Config
	datadogConfigMock := config.NewMock(t)
	datadogConfigMock.SetInTest("admission_controller.kubernetes_admission_events.enabled", true)

	tests := []struct {
		name                    string
		supportsMatchConditions bool
		expectedEmitted         bool
		request                 admission.Request
		expectedEvent           event.Event
	}{
		{
			name:                    "Pod creation",
			supportsMatchConditions: true,
			expectedEmitted:         true,
			request: admission.Request{
				UID:       "000",
				Name:      "pod",
				Namespace: "namespace",
				Kind: metav1.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Operation: admissionregistrationv1.Create,
				UserInfo:  &authenticationv1.UserInfo{Username: "username"},
				Object: func() []byte {
					marshalledObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledObject
				}(),
				OldObject: func() []byte {
					marshalledOldObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledOldObject
				}(),
				DynamicClient: nil,
				APIClient:     nil,
			},
			expectedEvent: event.Event{
				Title: fmt.Sprintf("%s Event for %s %s/%s by %s", "CREATE", "Pod", "namespace", "pod", "username"),
				Text: "%%%" +
					"**Kind:** " + "Pod" + "\\\n" +
					"**Resource:** " + "namespace" + "/" + "pod" + "\\\n" +
					"**Username:** " + "username" + "\\\n" +
					"**Operation:** " + "CREATE" + "\\\n" +
					"**Time:** " + time.Now().UTC().Format("January 02, 2006 at 03:04:05 PM MST") + "\\\n" +
					"**Request UID:** " + "000" +
					"%%%",
				Ts:       0,
				Priority: event.PriorityNormal,
				Tags: []string{
					"uid:" + "000",
					"kube_username:" + "username",
					"kube_kind:" + "Pod",
					"kube_namespace:" + "namespace",
					"kube_deployment:" + "pod",
					"operation:" + "CREATE",
				},
				AlertType:      event.AlertTypeInfo,
				SourceTypeName: sourceTypeName,
				EventType:      eventType,
			},
		},
		{
			name:                    "Pod update",
			supportsMatchConditions: true,
			expectedEmitted:         true,
			request: admission.Request{
				UID:       "000",
				Name:      "pod",
				Namespace: "namespace",
				Kind: metav1.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Operation: admissionregistrationv1.Update,
				UserInfo:  &authenticationv1.UserInfo{Username: "username"},
				Object: func() []byte {
					marshalledObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledObject
				}(),
				OldObject: func() []byte {
					marshalledOldObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledOldObject
				}(),
				DynamicClient: nil,
				APIClient:     nil,
			},
			expectedEvent: event.Event{
				Title: fmt.Sprintf("%s Event for %s %s/%s by %s", "UPDATE", "Pod", "namespace", "pod", "username"),
				Text: "%%%" +
					"**Kind:** " + "Pod" + "\\\n" +
					"**Resource:** " + "namespace" + "/" + "pod" + "\\\n" +
					"**Username:** " + "username" + "\\\n" +
					"**Operation:** " + "UPDATE" + "\\\n" +
					"**Time:** " + time.Now().UTC().Format("January 02, 2006 at 03:04:05 PM MST") + "\\\n" +
					"**Request UID:** " + "000" +
					"%%%",
				Ts:       0,
				Priority: event.PriorityNormal,
				Tags: []string{
					"uid:" + "000",
					"kube_username:" + "username",
					"kube_kind:" + "Pod",
					"kube_namespace:" + "namespace",
					"kube_deployment:" + "pod",
					"operation:" + "UPDATE",
				},
				AlertType:      event.AlertTypeInfo,
				SourceTypeName: sourceTypeName,
				EventType:      eventType,
			},
		},
		{
			name:                    "Pod deletion",
			supportsMatchConditions: true,
			expectedEmitted:         true,
			request: admission.Request{
				UID:       "000",
				Name:      "pod",
				Namespace: "namespace",
				Kind: metav1.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Operation: admissionregistrationv1.Delete,
				UserInfo:  &authenticationv1.UserInfo{Username: "username"},
				Object: func() []byte {
					marshalledObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledObject
				}(),
				OldObject: func() []byte {
					marshalledOldObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledOldObject
				}(),
				DynamicClient: nil,
				APIClient:     nil,
			},
			expectedEvent: event.Event{
				Title: fmt.Sprintf("%s Event for %s %s/%s by %s", "DELETE", "Pod", "namespace", "pod", "username"),
				Text: "%%%" +
					"**Kind:** " + "Pod" + "\\\n" +
					"**Resource:** " + "namespace" + "/" + "pod" + "\\\n" +
					"**Username:** " + "username" + "\\\n" +
					"**Operation:** " + "DELETE" + "\\\n" +
					"**Time:** " + time.Now().UTC().Format("January 02, 2006 at 03:04:05 PM MST") + "\\\n" +
					"**Request UID:** " + "000" +
					"%%%",
				Ts:       0,
				Priority: event.PriorityNormal,
				Tags: []string{
					"uid:" + "000",
					"kube_username:" + "username",
					"kube_kind:" + "Pod",
					"kube_namespace:" + "namespace",
					"kube_deployment:" + "pod",
					"operation:" + "DELETE",
				},
				AlertType:      event.AlertTypeInfo,
				SourceTypeName: sourceTypeName,
				EventType:      eventType,
			},
		},
		{
			name:                    "Pod creation by system user without match conditions",
			supportsMatchConditions: false,
			expectedEmitted:         false,
			request: admission.Request{
				UID:       "000",
				Name:      "pod",
				Namespace: "namespace",
				Kind: metav1.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				},
				Operation: admissionregistrationv1.Create,
				UserInfo:  &authenticationv1.UserInfo{Username: "system:serviceaccount"},
				Object: func() []byte {
					marshalledObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledObject
				}(),
				OldObject: func() []byte {
					marshalledOldObject, _ := json.Marshal(&unstructured.Unstructured{Object: map[string]interface{}{"kind": "Pod"}})
					return marshalledOldObject
				}(),
				DynamicClient: nil,
				APIClient:     nil,
			},
			expectedEvent: event.Event{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock demultiplexer and sender - create fresh instances for each test case
			demultiplexerMock := createDemultiplexer(t)
			mockSender := mocksender.NewMockSenderWithSenderManager(eventType, demultiplexerMock)
			err := demultiplexerMock.SetSender(mockSender, eventType)
			assert.NoError(t, err)

			// Create the webhook
			kubernetesAuditWebhook := NewWebhook(datadogConfigMock, demultiplexerMock, tt.supportsMatchConditions)
			assert.True(t, kubernetesAuditWebhook.IsEnabled())
			assert.Equal(t, eventType, kubernetesAuditWebhook.name)

			// Emit the event
			start := time.Now()
			mockSender.On("Event", mock.AnythingOfType("event.Event")).Return().Once()
			validated, err := kubernetesAuditWebhook.emitEvent(&tt.request, "", nil)
			// Force flush to serializer to ensure the event is emitted and received.
			demultiplexerMock.ForceFlushToSerializer(start, true)
			assert.NoError(t, err)
			assert.True(t, validated)
			if tt.expectedEmitted {
				mockSender.AssertEventWithCompareFunc(t, tt.expectedEvent, 1*time.Second, compareText)
			} else {
				mockSender.AssertNotCalled(t, "Event")
			}
		})
	}
}

// createDemultiplexer creates a demultiplexer for testing
func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t, fx.Provide(func() log.Component { return logmock.New(t) }), logscompression.MockModule(), metricscompression.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
}
