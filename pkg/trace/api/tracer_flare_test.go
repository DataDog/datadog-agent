// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTraceFlareBody(multipartBoundary string) io.ReadCloser {
	bodyReader, bodyWriter := io.Pipe()

	writer := multipart.NewWriter(bodyWriter)
	writer.SetBoundary(multipartBoundary)

	go func() {
		defer bodyWriter.Close()
		defer writer.Close()

		archive, err := os.Create("flare.zip")
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		archive.Close()
		defer os.Remove("flare.zip")

		p, err := writer.CreateFormFile("flare_file", filepath.Base("flare.zip"))
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		file, err := os.Open("flare.zip")
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		defer file.Close()

		_, err = io.Copy(p, file)

		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}

		writer.WriteField("case_id", "case_id")
		writer.WriteField("source", "tracer_go")
		writer.WriteField("email", "test@test.com")
		writer.WriteField("hostname", "hostname")
	}()

	return bodyReader

}

func TestTracerFlareProxyHandler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(404)
		}

		switch req.URL.Path {
		case "/api/ui/support/serverless/flare":
			assert.Equal(t, "test", req.Header.Get("DD-API-KEY"), "got invalid API key: %q", req.Header.Get("DD-API-KEY"))
			err := req.ParseMultipartForm(1000000)
			assert.Nil(t, err)

			assert.Equal(t, req.FormValue("case_id"), "case_id")
			assert.Equal(t, req.FormValue("source"), "tracer_go")
			assert.Equal(t, req.FormValue("email"), "test@test.com")
			assert.Equal(t, req.FormValue("hostname"), "hostname")
			assert.Equal(t, req.MultipartForm.File["flare_file"][0].Filename, "flare.zip")

			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	req, err := http.NewRequest("POST", srv.URL, nil)
	assert.NoError(t, err)
	boundaryWriter := multipart.NewWriter(nil)

	req.Header.Set("Content-Type", boundaryWriter.FormDataContentType())
	req.Body = getTraceFlareBody(boundaryWriter.Boundary())
	req.ContentLength = -1

	rec := httptest.NewRecorder()
	receiver := newTestReceiverFromConfig(newTestReceiverConfig())
	receiver.tracerFlareHandler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
