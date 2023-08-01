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

	mockDetector := mockDetector{}
	mockDetector.On("DetectLanguage", mockPid).Return(mockGoLanguage, nil).Once()
	languagedetection.MockPrivilegedDetectors(t, []languagedetection.Detector{&mockDetector})

	rec := httptest.NewRecorder()

	reqProto := languageDetectionProto.DetectLanguageRequest{Processes: []*languageDetectionProto.Process{{Pid: mockPid}}}
	reqBytes, err := proto.Marshal(&reqProto)
	require.NoError(t, err)

	detectLanguage(rec, httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(reqBytes)))

	resBytes, err := io.ReadAll(rec.Result().Body)
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

func (m *mockDetector) DetectLanguage(pid int) (languagemodels.Language, error) {
	args := m.Mock.Called(pid)
	return args.Get(0).(languagemodels.Language), args.Error(1)
}
