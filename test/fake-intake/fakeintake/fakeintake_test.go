// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fakeintake

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPOSTPayloads(t *testing.T) {
	t.Run("should accept payloads on any route", func(t *testing.T) {
		fi := NewFakeIntake()
		defer fi.server.Close()

		request, err := http.NewRequest(http.MethodPost, "/api/v2/series", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err, "Error creating POST request")
		response := httptest.NewRecorder()

		fi.postPayload(response, request)

		assert.Equal(t, http.StatusAccepted, response.Code, "unexpected code")
	})

	t.Run("should not accept GET requests on any route", func(t *testing.T) {
		fi := NewFakeIntake()
		defer fi.server.Close()

		request, err := http.NewRequest(http.MethodGet, "/api/v2/series", nil)
		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.postPayload(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code, "unexpected code")
	})

	t.Run("should accept GET requests on /fakeintake/payloads route", func(t *testing.T) {
		fi := NewFakeIntake()
		defer fi.server.Close()

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/foo", nil)

		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.getPayloads(response, request)
		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")

		expectedResponse := getPayloadResponse{
			Payloads: [][]byte{},
		}
		actualResponse := getPayloadResponse{}
		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err, "Error reading response")
		json.Unmarshal(body, &actualResponse)

		assert.Equal(t, expectedResponse, actualResponse, "unexpected response")
	})

	t.Run("should not accept GET requests on /fakeintake/payloads route without endpoint query parameter", func(t *testing.T) {
		fi := NewFakeIntake()
		defer fi.server.Close()

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads", nil)

		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.getPayloads(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code, "unexpected code")
	})

	t.Run("should store multiple payloads on any route and return them", func(t *testing.T) {
		fi := NewFakeIntake()
		defer fi.server.Close()

		request, err := http.NewRequest(http.MethodPost, "/api/v2/series", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err, "Error creating POST request")
		postResponse := httptest.NewRecorder()
		fi.postPayload(postResponse, request)

		request, err = http.NewRequest(http.MethodPost, "/api/v2/series", strings.NewReader("totoro|7|tag:valid,owner:pducolin"))
		assert.NoError(t, err, "Error creating POST request")
		postResponse = httptest.NewRecorder()
		fi.postPayload(postResponse, request)

		request, err = http.NewRequest(http.MethodPost, "/api/v2/logs", strings.NewReader("I am just a poor log"))
		assert.NoError(t, err, "Error creating POST request")
		postResponse = httptest.NewRecorder()
		fi.postPayload(postResponse, request)

		request, err = http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/api/v2/series", nil)
		assert.NoError(t, err, "Error creating GET request")
		getResponse := httptest.NewRecorder()

		fi.getPayloads(getResponse, request)

		assert.Equal(t, http.StatusOK, getResponse.Code)

		expectedGETResponse := getPayloadResponse{
			Payloads: [][]byte{
				[]byte("totoro|5|tag:valid,owner:pducolin"),
				[]byte("totoro|7|tag:valid,owner:pducolin"),
			},
		}
		actualGETResponse := getPayloadResponse{}
		body, err := io.ReadAll(getResponse.Body)
		assert.NoError(t, err, "Error reading GET response")
		json.Unmarshal(body, &actualGETResponse)

		assert.Equal(t, expectedGETResponse, actualGETResponse, "unexpected GET response")
	})
}
