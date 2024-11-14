// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockAttacher struct {
	overrideAttachPID func(uint32) error
	overrideDetachPID func(uint32) error
}

func (m *mockAttacher) AttachPID(pid uint32) error {
	if m.overrideAttachPID != nil {
		return m.overrideAttachPID(pid)
	}
	return nil
}

func (m *mockAttacher) DetachPID(pid uint32) error {
	if m.overrideDetachPID != nil {
		return m.overrideDetachPID(pid)
	}
	return nil
}

func TestRunAttacherCallback(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		moduleName     string
		body           interface{}
		mode           callbackType
		expectedStatus int
		expectedBody   string
		attacherSetup  func(Attacher)
	}{
		{
			name:           "Non-POST method",
			method:         http.MethodGet,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           attach,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Only POST requests are allowed",
		},
		{
			name:           "Malformed JSON body",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           "{malformed_json}",
			mode:           attach,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Error decoding request body",
		},
		{
			name:           "Unrecognized module",
			method:         http.MethodPost,
			moduleName:     "unknownModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           attach,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `module "unknownModule" is unrecognized`,
		},
		{
			name:           "Unrecognized attacher type",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "unknownType", PID: 123},
			mode:           attach,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `Module "unknownType" is not enabled`,
		},
		{
			name:           "Attach callback error",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           attach,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Error attaching PID",
			attacherSetup: func(att Attacher) {
				att.(*mockAttacher).overrideAttachPID = func(uint32) error { return errors.New("attach error") }
			},
		},
		{
			name:           "Detach callback error",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           detach,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Error detaching PID",
			attacherSetup: func(att Attacher) {
				att.(*mockAttacher).overrideDetachPID = func(uint32) error { return errors.New("detach error") }
			},
		},
		{
			name:           "Successful attach",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           attach,
			expectedStatus: http.StatusOK,
			expectedBody:   "testType successfully attached PID 123",
		},
		{
			name:           "Successful detach",
			method:         http.MethodPost,
			moduleName:     "testModule",
			body:           attachRequestBody{Type: "testType", PID: 123},
			mode:           detach,
			expectedStatus: http.StatusOK,
			expectedBody:   "testType successfully detached PID 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &tlsDebugger{
				attachers: map[string]attacherMap{
					"testModule": {
						"testType": &mockAttacher{},
					},
				},
			}

			if tt.attacherSetup != nil {
				tt.attacherSetup(d.attachers["testModule"]["testType"])
			}

			var reqBody bytes.Buffer
			if bodyStr, ok := tt.body.(string); ok {
				reqBody = *bytes.NewBufferString(bodyStr)
			} else if body, ok := tt.body.(attachRequestBody); ok {
				json.NewEncoder(&reqBody).Encode(body)
			}

			req := httptest.NewRequest(tt.method, "/attach", &reqBody)
			w := httptest.NewRecorder()

			d.runAttacherCallback(tt.moduleName, w, req, tt.mode)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			body, _ := io.ReadAll(res.Body)
			if !strings.Contains(string(body), tt.expectedBody) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedBody, string(body))
			}
		})
	}
}
