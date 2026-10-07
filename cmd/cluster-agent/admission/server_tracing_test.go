// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
)

func TestAdmissionRequestSpans(t *testing.T) {
	for _, version := range []string{"v1", "v1beta1"} {
		t.Run(version, func(t *testing.T) {
			mt := mocktracer.Start()
			defer mt.Stop()
			s := newTestServer(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.Register("/mutate", "test_mutation", admicommon.MutatingWebhook, func(r *Request) *admiv1.AdmissionResponse {
				parent, ok := tracer.SpanFromContext(r.Context)
				require.True(t, ok)
				require.NotNil(t, parent)
				cancel()
				assert.ErrorIs(t, r.Context.Err(), context.Canceled)
				child, _ := tracer.StartSpanFromContext(r.Context, "test.mutation")
				child.Finish()
				return &admiv1.AdmissionResponse{Allowed: true, Patch: []byte(`[]`)}
			}, nil, nil)
			body := `{"apiVersion":"admission.k8s.io/` + version + `","kind":"AdmissionReview","request":{"uid":"test","namespace":"application","kind":{"kind":"Pod"},"operation":"CREATE","object":{"metadata":{}}}}`
			req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(body)).WithContext(ctx)
			req.Header.Set("Content-Type", jsonContentType)
			rw := httptest.NewRecorder()
			s.mux.ServeHTTP(rw, req)
			require.Equal(t, http.StatusOK, rw.Code)
			spans := map[string]*mocktracer.Span{}
			for _, span := range mt.FinishedSpans() {
				spans[span.OperationName()] = span
			}
			require.Len(t, spans, 7)
			root := spans["cluster_agent.admission.request"]
			require.NotNil(t, root)
			assert.Equal(t, "test_mutation", root.Tag("resource.name"))
			assert.Equal(t, "application", root.Tag("namespace"))
			assert.Equal(t, "CREATE", root.Tag("operation"))
			assert.Equal(t, float64(http.StatusOK), root.Tag("http.status_code"))
			assert.Nil(t, root.Tag("error.message"))
			webhook := spans["cluster_agent.admission.webhook"]
			require.NotNil(t, webhook)
			for _, name := range []string{"read_body", "decode_review", "webhook", "encode_response"} {
				span := spans["cluster_agent.admission."+name]
				require.NotNil(t, span)
				assert.Equal(t, root.SpanID(), span.ParentID())
				assert.Equal(t, root.TraceID(), span.TraceID())
			}
			assert.Equal(t, webhook.SpanID(), spans["cluster_agent.admission.check_probe"].ParentID())
			assert.Equal(t, webhook.SpanID(), spans["test.mutation"].ParentID())
		})
	}
}

func TestAdmissionSpanErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		result     *metav1.Status
	}{
		{name: "decode", body: `{`, status: http.StatusBadRequest},
		{name: "allowed mutation failure", body: `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test"}}`, status: http.StatusOK, result: &metav1.Status{Message: "mutation failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mt := mocktracer.Start()
			defer mt.Stop()
			s := newTestServer(t)
			s.Register("/mutate", "test", admicommon.MutatingWebhook, func(*Request) *admiv1.AdmissionResponse {
				return &admiv1.AdmissionResponse{Allowed: true, Result: tc.result}
			}, nil, nil)
			req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", jsonContentType)
			rw := httptest.NewRecorder()
			s.mux.ServeHTTP(rw, req)
			assert.Equal(t, tc.status, rw.Code)
			for _, span := range mt.FinishedSpans() {
				if span.OperationName() == "cluster_agent.admission.request" {
					assert.NotNil(t, span.Tag("error.message"))
					assert.Equal(t, float64(tc.status), span.Tag("http.status_code"))
				}
			}
		})
	}
}

func TestAdmissionProbeAndPanicSpans(t *testing.T) {
	for _, probe := range []bool{true, false} {
		mt := mocktracer.Start()
		s := newTestServer(t)
		s.Register("/mutate", "test", admicommon.MutatingWebhook, func(*Request) *admiv1.AdmissionResponse { panic("mutation panic") }, nil, nil)
		label := "false"
		if probe {
			label = "true"
		}
		body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test","object":{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"` + label + `"}}}}}`
		req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(body))
		req.Header.Set("Content-Type", jsonContentType)
		rw := httptest.NewRecorder()
		if probe {
			require.NotPanics(t, func() { s.mux.ServeHTTP(rw, req) })
		} else {
			require.PanicsWithValue(t, "mutation panic", func() { s.mux.ServeHTTP(rw, req) })
		}
		found := false
		for _, span := range mt.FinishedSpans() {
			if span.OperationName() == "cluster_agent.admission.webhook" {
				found = true
				if probe {
					assert.Equal(t, "true", span.Tag("probe"))
				} else {
					assert.NotNil(t, span.Tag("error.message"))
				}
			}
		}
		assert.True(t, found)
		mt.Stop()
	}
}
