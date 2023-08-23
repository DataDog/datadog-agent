// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	languageDetectionProto "github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
)

const mockPid = 1

func TestLanguageDetectionEndpoint(t *testing.T) {
	mockGoLanguage := languagemodels.Language{Name: languagemodels.Go, Version: "go version go1.19.10 linux/arm64"}
	proc := &languageDetectionProto.Process{Pid: mockPid}

	mockDetector := mockDetector{}
	mockDetector.On("DetectLanguage", mock.MatchedBy(
		func(proc languagemodels.Process) bool {
			return proc.GetPid() == mockPid
		})).
		Return(mockGoLanguage, nil).Once()
	languagedetection.MockPrivilegedDetectors(t, []languagemodels.Detector{&mockDetector})

	rec := httptest.NewRecorder()

	reqProto := languageDetectionProto.DetectLanguageRequest{Processes: []*languageDetectionProto.Process{proc}}
	reqBytes, err := proto.Marshal(&reqProto)
	require.NoError(t, err)

	detectLanguage(rec, httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(reqBytes)))

	resBody := rec.Result().Body
	defer resBody.Close()

	resBytes, err := io.ReadAll(resBody)
	require.NoError(t, err)

	var detectLanguageResponse languageDetectionProto.DetectLanguageResponse
	err = proto.Unmarshal(resBytes, &detectLanguageResponse)
	require.NoError(t, err)

	assert.True(t, proto.Equal(
		&languageDetectionProto.DetectLanguageResponse{
			Languages: []*languageDetectionProto.Language{{
				Name:    string(mockGoLanguage.Name),
				Version: mockGoLanguage.Version,
			}}},
		&detectLanguageResponse,
	))
}

type mockDetector struct {
	mock.Mock
}

func (m *mockDetector) DetectLanguage(proc languagemodels.Process) (languagemodels.Language, error) {
	args := m.Mock.Called(proc)
	return args.Get(0).(languagemodels.Language), args.Error(1)
}
