// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newMockStore(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func TestPreHandlerSpan_FeatureDisabled(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := newMockStore(t)
	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled: false,
		},
		wlm:             mockStore,
		ownersLanguages: newOwnersLanguages(),
	}

	req := httptest.NewRequest("POST", "/languagedetection", nil)
	rec := httptest.NewRecorder()

	result := handler.preHandler(rec, req)

	assert.False(t, result)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.pre_handler", span.OperationName())
	assert.Equal(t, "preHandler", span.Tag("resource.name"))
	assert.Equal(t, false, span.Tag("feature_enabled"))
	// mocktracer stores the error object in the "error" tag via WithError
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Equal(t, "language detection feature is disabled", err.Error())
}

func TestPreHandlerSpan_NilBody(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := newMockStore(t)
	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled: true,
		},
		wlm:             mockStore,
		ownersLanguages: newOwnersLanguages(),
	}

	req := httptest.NewRequest("POST", "/languagedetection", nil)
	req.Body = nil // explicitly nil body
	rec := httptest.NewRecorder()

	result := handler.preHandler(rec, req)

	assert.False(t, result)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.pre_handler", span.OperationName())
	assert.Equal(t, "preHandler", span.Tag("resource.name"))
	assert.Equal(t, true, span.Tag("feature_enabled"))
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Equal(t, "request body is empty", err.Error())
}

func TestLeaderHandlerSpan_Success(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := newMockStore(t)
	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled:     true,
			languageTTL: 10 * time.Minute,
		},
		wlm:             mockStore,
		ownersLanguages: newOwnersLanguages(),
	}

	requestData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			{
				Namespace: "default",
				Name:      "pod-a",
				ContainerDetails: []*pbgo.ContainerLanguageDetails{
					{
						ContainerName: "container-1",
						Languages:     []*pbgo.Language{{Name: "java"}},
					},
				},
				Ownerref: &pbgo.KubeOwnerInfo{
					Kind: "ReplicaSet",
					Name: "deploy-a-12345",
				},
			},
		},
	}

	body, err := proto.Marshal(requestData)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/languagedetection", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.leaderHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.leader_handler", span.OperationName())
	assert.Equal(t, "leaderHandler", span.Tag("resource.name"))
	assert.Equal(t, 1, span.Tag("owner_count"))
	assert.Nil(t, span.Tag("error"))
}

func TestLeaderHandlerSpan_UnmarshalError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := newMockStore(t)
	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled:     true,
			languageTTL: 10 * time.Minute,
		},
		wlm:             mockStore,
		ownersLanguages: newOwnersLanguages(),
	}

	// Send invalid protobuf data
	req := httptest.NewRequest("POST", "/languagedetection", bytes.NewReader([]byte("not-a-protobuf")))
	rec := httptest.NewRecorder()

	handler.leaderHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.leader_handler", span.OperationName())
	assert.Equal(t, "leaderHandler", span.Tag("resource.name"))
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Contains(t, err.Error(), "failed to unmarshal request body")
}

func TestHandleLeadershipState_BecameLeader(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled: true,
		},
		ownersLanguages:       newOwnersLanguages(),
		leaderElectionEnabled: false, // isLeader() returns true when disabled
		wasLeader:             false,
		initialized:           true, // skip initial state handling
	}

	ctx := context.Background()
	handler.handleLeadershipState(ctx)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.leadership_change", span.OperationName())
	assert.Equal(t, "leadershipChange", span.Tag("resource.name"))
	assert.Equal(t, "leader", span.Tag("became"))
}

func TestHandleLeadershipState_BecameFollower(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := newMockStore(t)

	// We need isLeader() to return false. When leaderElectionEnabled is true,
	// isLeader calls leaderelection.GetLeaderEngine() which will fail in tests,
	// causing it to return false.
	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled:     true,
			languageTTL: 10 * time.Minute,
		},
		wlm:                   mockStore,
		ownersLanguages:       newOwnersLanguages(),
		leaderElectionEnabled: true, // isLeader() returns false when engine not available
		wasLeader:             true,
		initialized:           true, // skip initial state handling
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler.handleLeadershipState(ctx)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.language_detection.leadership_change", span.OperationName())
	assert.Equal(t, "leadershipChange", span.Tag("resource.name"))
	assert.Equal(t, "follower", span.Tag("became"))
}

func TestHandleLeadershipState_NoChange(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	handler := &languageDetectionHandler{
		cfg: handlerConfig{
			enabled: true,
		},
		ownersLanguages:       newOwnersLanguages(),
		leaderElectionEnabled: false, // isLeader() returns true
		wasLeader:             true,  // already leader, no change
		initialized:           true,
	}

	ctx := context.Background()
	handler.handleLeadershipState(ctx)

	spans := mt.FinishedSpans()
	assert.Empty(t, spans, "no span should be created when leadership state doesn't change")
}
