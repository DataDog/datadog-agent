// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestAdmissionRequestTracing(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		body    string
		method  string
		probe   bool
		fail    bool
		status  int
		stage   string
	}{
		{name: "v1", version: "v1", status: http.StatusOK},
		{name: "v1beta1", version: "v1beta1", status: http.StatusOK},
		{name: "mutation error is HTTP 200", version: "v1", fail: true, status: http.StatusOK, stage: "webhook"},
		{name: "probe bypass", version: "v1", probe: true, status: http.StatusOK},
		{name: "invalid method", method: http.MethodGet, status: http.StatusMethodNotAllowed, stage: "method"},
		{name: "invalid review", body: "sensitive-invalid-body", status: http.StatusBadRequest, stage: "decode_review"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := mocktracer.Start()
			defer mt.Stop()
			s := newTestServer(t)
			called := false
			s.Register("/test", "test", admicommon.MutatingWebhook, func(req *Request) *admiv1.AdmissionResponse {
				called = true
				return admicommon.MutationResponse(mutatecommon.MutateWithContext(req.Context, req.Object, req.Namespace, "test",
					func(*corev1.Pod, string, dynamic.Interface) (bool, error) {
						if tt.fail {
							return false, errors.New("sensitive-error")
						}
						return false, nil
					}, nil))
			}, nil, nil)
			body := tt.body
			if tt.version != "" {
				object := `{"metadata":{"name":"sensitive-pod","annotations":{"secret":"sensitive-annotation"}},"spec":{"containers":[]}}`
				if tt.probe {
					object = `{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`
				}
				body = fmt.Sprintf(`{"apiVersion":"admission.k8s.io/%s","kind":"AdmissionReview","request":{"uid":"sensitive-uid","namespace":"sensitive-namespace","userInfo":{"username":"sensitive-user"},"object":%s}}`, tt.version, object)
			}
			method := tt.method
			if method == "" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, "/test", strings.NewReader(body))
			req.Header.Set("Content-Type", jsonContentType)
			upstream, ctx := tracer.StartSpanFromContext(context.Background(), "upstream")
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req.WithContext(ctx))
			upstream.Finish()
			require.Equal(t, tt.status, w.Code)
			require.Equal(t, tt.version != "" && !tt.probe, called)
			spans := map[string]*mocktracer.Span{}
			for _, span := range mt.FinishedSpans() {
				spans[span.OperationName()] = span
				for key, value := range span.Tags() {
					require.NotContains(t, key+fmt.Sprint(value), "sensitive-")
				}
			}
			request := spans["cluster_agent.admission.request"]
			require.NotNil(t, request)
			require.Equal(t, spans["upstream"].SpanID(), request.ParentID())
			require.Equal(t, "server", request.Tag("span.kind"))
			require.Equal(t, float64(tt.status), request.Tag("http.status_code"))
			if tt.stage != "" {
				require.Equal(t, tt.stage, request.Tag("failure_stage"))
				require.NotNil(t, request.Tag("error.message"))
			} else {
				require.Nil(t, request.Tag("error.message"))
			}
			if called {
				mutation := spans["cluster_agent.admission.mutate"]
				require.NotNil(t, mutation)
				require.Equal(t, request.SpanID(), mutation.ParentID())
				callback := spans["cluster_agent.admission.mutate_pod"]
				require.NotNil(t, callback)
				require.Equal(t, mutation.SpanID(), callback.ParentID())
				if !tt.fail {
					patch := spans["cluster_agent.admission.prepare_patch"]
					require.NotNil(t, patch)
					require.Equal(t, mutation.SpanID(), patch.ParentID())
				}
			}
		})
	}
}

func TestAdmissionTracingPreservesPanic(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()
	s := newTestServer(t)
	s.Register("/test", "test", admicommon.MutatingWebhook, func(req *Request) *admiv1.AdmissionResponse {
		return admicommon.MutationResponse(mutatecommon.MutateWithContext(req.Context, []byte(`{"spec":{"containers":[]}}`), "", "test",
			func(*corev1.Pod, string, dynamic.Interface) (bool, error) {
				panic("sensitive-panic")
			}, nil))
	}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test"}}`))
	req.Header.Set("Content-Type", jsonContentType)
	require.PanicsWithValue(t, "sensitive-panic", func() { s.mux.ServeHTTP(httptest.NewRecorder(), req) })
	spans := map[string]*mocktracer.Span{}
	for _, span := range mt.FinishedSpans() {
		spans[span.OperationName()] = span
		require.NotNil(t, span.Tag("error.message"))
		require.NotContains(t, fmt.Sprint(span.Tags()), "sensitive-")
		require.Nil(t, span.Tag("injected"))
	}
	require.Len(t, spans, 3)
	request := spans["cluster_agent.admission.request"]
	require.NotNil(t, request)
	require.Equal(t, "panic", request.Tag("failure_stage"))
	mutation := spans["cluster_agent.admission.mutate"]
	require.NotNil(t, mutation)
	require.Equal(t, request.SpanID(), mutation.ParentID())
	callback := spans["cluster_agent.admission.mutate_pod"]
	require.NotNil(t, callback)
	require.Equal(t, mutation.SpanID(), callback.ParentID())
	require.Nil(t, spans["cluster_agent.admission.prepare_patch"])
}

// A policy denial is an admission outcome, not necessarily an internal failure.
func TestAdmissionTracingPolicyDenial(t *testing.T) {
	for _, tt := range []struct {
		name        string
		webhookType admicommon.WebhookType
		result      *metav1.Status
	}{
		{name: "reason only", webhookType: admicommon.ValidatingWebhook, result: &metav1.Status{Reason: metav1.StatusReasonForbidden}},
		// DatadogInstrumentation policy denials supply a message without a reason.
		{name: "validation message", webhookType: admicommon.ValidatingWebhook, result: &metav1.Status{Message: "sensitive-policy-explanation"}},
		{name: "mutation denial", webhookType: admicommon.MutatingWebhook, result: &metav1.Status{Message: "sensitive-policy-explanation"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := mocktracer.Start()
			defer mt.Stop()
			s := newTestServer(t)
			s.Register("/test", "test", tt.webhookType, func(*Request) *admiv1.AdmissionResponse {
				return &admiv1.AdmissionResponse{Allowed: false, Result: tt.result}
			}, nil, nil)
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test"}}`))
			req.Header.Set("Content-Type", jsonContentType)
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			var response admiv1.AdmissionReview
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.NotNil(t, response.Response)
			require.False(t, response.Response.Allowed)
			require.Equal(t, tt.result, response.Response.Result)
			spans := mt.FinishedSpans()
			require.Len(t, spans, 1)
			require.Equal(t, "false", spans[0].Tag("admission.allowed"))
			require.Nil(t, spans[0].Tag("error.message"))
			require.Nil(t, spans[0].Tag("failure_stage"))
			require.NotContains(t, fmt.Sprint(spans[0].Tags()), "sensitive-")
		})
	}
}
