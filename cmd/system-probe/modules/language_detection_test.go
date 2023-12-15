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

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
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
	privileged.MockPrivilegedDetectors(t, []languagemodels.Detector{&mockDetector})

	rec := httptest.NewRecorder()

	reqProto := languageDetectionProto.DetectLanguageRequest{Processes: []*languageDetectionProto.Process{proc}}
	reqBytes, err := proto.Marshal(&reqProto)
	require.NoError(t, err)

	m := languageDetectionModule{
		languageDetector: privileged.NewLanguageDetector(),
	}
	m.detectLanguage(rec, httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(reqBytes)))

	resBody := rec.Result().Body
	defer resBody.Close()

	resBytes, err := io.ReadAll(resBody)
	require.NoError(t, err)

	var detectLanguageResponse languageDetectionProto.DetectLanguageResponse
	err = proto.Unmarshal(resBytes, &detectLanguageResponse)
	require.NoError(t, err)

	expected := &languageDetectionProto.DetectLanguageResponse{
		Languages: []*languageDetectionProto.Language{{
			Name:    string(mockGoLanguage.Name),
			Version: mockGoLanguage.Version,
		}}}
	assert.True(t,
		proto.Equal(expected, &detectLanguageResponse),
		"expected:\n%v\nactual:\n%v", spew.Sdump(expected), spew.Sdump(&detectLanguageResponse))
}

type mockDetector struct {
	mock.Mock
}

func (m *mockDetector) DetectLanguage(proc languagemodels.Process) (languagemodels.Language, error) {
	args := m.Mock.Called(proc)
	return args.Get(0).(languagemodels.Language), args.Error(1)
}
