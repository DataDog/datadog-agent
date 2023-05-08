// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestMkURL(t *testing.T) {
	assert.Equal(t, "https://example.com/support/flare/999?api_key=123456", mkURL("https://example.com", "999", "123456"))
	assert.Equal(t, "https://example.com/support/flare?api_key=123456", mkURL("https://example.com", "", "123456"))
}

func TestFlareHasRightForm(t *testing.T) {
	var lastRequest *http.Request

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This server serves two requests:
		//  * the original flare request URL, which redirects on HEAD to /post-target
		//  * HEAD /post-target - responds with 200 OK
		//  * POST /post-target - the final POST

		if r.Method == "HEAD" && r.RequestURI == "/support/flare/12345?api_key=abcdef" {
			// redirect to /post-target.
			w.Header().Set("Location", "/post-target")
			w.WriteHeader(307)
		} else if r.Method == "HEAD" && r.RequestURI == "/post-target" {
			// accept a HEAD to /post-target
			w.WriteHeader(200)
		} else if r.Method == "POST" && r.RequestURI == "/post-target" {
			// handle the actual POST
			w.Header().Set("Content-Type", "application/json")
			lastRequest = r
			err := lastRequest.ParseMultipartForm(1000000)
			assert.Nil(t, err)
			io.WriteString(w, "{}")
		} else {
			w.WriteHeader(500)
			io.WriteString(w, "path not recognized by httptest server")
		}
	}))
	defer ts.Close()

	ddURL := ts.URL

	archivePath := "./test/blank.zip"
	caseID := "12345"
	email := "dev@datadoghq.com"

	_, err := SendTo(archivePath, caseID, email, "abcdef", ddURL)

	assert.Nil(t, err)

	av, _ := version.Agent()

	assert.Equal(t, caseID, lastRequest.FormValue("case_id"))
	assert.Equal(t, email, lastRequest.FormValue("email"))
	assert.Equal(t, av.String(), lastRequest.FormValue("agent_version"))
}

func TestAnalyzeResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"case_id\": 1234}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.NoError(t, reserr)
		require.Equal(t,
			"Your logs were successfully uploaded. For future reference, your internal case id is 1234",
			resstr)
	})

	t.Run("error-from-server", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"case_id\": 1234, \"error\": \"uhoh\"}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t, errors.New("uhoh"), reserr)
		require.Equal(t,
			"An error occurred while uploading the flare: uhoh. Please contact support by email.",
			resstr)
	})

	t.Run("unparseable-from-server", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("thats-not-json"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("invalid character 'h' in literal true (expecting 'r')\n"+
				"Server returned:\n"+
				"thats-not-json"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("unparseable-from-server-huge", func(t *testing.T) {
		resp := "uhoh"
		for i := 0; i < 100; i++ {
			resp += "\npad this out to be pretty long"
		}
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte(resp))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("invalid character 'u' looking for beginning of value\n"+
				"Server returned:\n"+
				resp[:150]),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("no-content-type", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"json\": true}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("Server returned a 200 but with no content-type header\n"+
				"Server returned:\n"+
				"{\"json\": true}"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("bad-content-type", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"json\": true}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("Server returned a 200 but with an unknown content-type text/plain\n"+
				"Server returned:\n"+
				"{\"json\": true}"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("unknown-status", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 502,
			Status:     "Bad Gateway",
			Body:       io.NopCloser(bytes.NewBuffer([]byte("<html>.."))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("HTTP 502 Bad Gateway\n"+
				"Server returned:\n"+
				"<html>.."),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("forbidden-no-api-key", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 403,
		}
		resstr, reserr := analyzeResponse(r, "")
		require.Equal(t,
			errors.New("HTTP 403 Forbidden: API key is missing"),
			reserr)
		require.Equal(t, "", resstr)
	})

	t.Run("forbidden-with-api-key", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 403,
		}
		resstr, reserr := analyzeResponse(r, "abcd123abcd12344abcd1234")
		require.Equal(t,
			errors.New("HTTP 403 Forbidden: Make sure your API key is valid. API Key ending with: d1234"),
			reserr)
		require.Equal(t, "", resstr)
	})
}
