// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	t.Run("should properly format the request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow requests only to "/foo/bar"
			routes := r.URL.Query()["endpoint"]

			payloads := [][]byte{}

			payloads = append(payloads, []byte(r.URL.Path))
			payloads = append(payloads, []byte(fmt.Sprintf("%d", len(routes))))
			payloads = append(payloads, []byte(routes[0]))
			// create fake response
			resp, err := json.Marshal(api.APIFakeIntakePayloadsGETResponse{
				Payloads: payloads,
			})
			require.NoError(t, err)
			w.Write(resp)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 3, len(payloads))
		assert.Equal(t, "/fakeintake/payloads", string(payloads[0]))
		assert.Equal(t, "1", string(payloads[1]))
		assert.Equal(t, "/foo/bar", string(payloads[2]))
	})

	t.Run("should handle response with errors", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.Error(t, err, "Expecting error")
		assert.Nil(t, payloads)
	})
}
