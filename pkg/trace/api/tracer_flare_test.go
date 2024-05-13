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
	"net/http/httputil"
	"net/url"
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

func mockGetServerlessFlareEndpoint(url *url.URL, _ string) *url.URL {
	url.Path = "/api/ui/support/serverless/flare"

	return url
}

func TestGetServerlessFlareEndpoint(t *testing.T) {
	agentVersion := "7.50.0-devel+git.488.be5861b"
	t.Run("url without subdomain", func(t *testing.T) {
		site := "datadoghq.com"
		testURL, err := url.Parse(site)

		getServerlessFlareEndpoint(testURL, agentVersion)
		assert.Nil(t, err)
		assert.Equal(t, "https://7-50-0-flare.datadoghq.com/api/ui/support/serverless/flare", testURL.String())
	})

	t.Run("url with subdomain", func(t *testing.T) {
		site := "https://app.datadoghq.com/test"
		testURL, err := url.Parse(site)
		assert.Nil(t, err)

		getServerlessFlareEndpoint(testURL, agentVersion)
		assert.Nil(t, err)
		assert.Equal(t, "https://7-50-0-flare.datadoghq.com/api/ui/support/serverless/flare", testURL.String())
	})

	t.Run("url with subdomain, custom dd domain", func(t *testing.T) {
		site := "https://us3.datadoghq.com/test"
		testURL, err := url.Parse(site)
		assert.Nil(t, err)

		getServerlessFlareEndpoint(testURL, agentVersion)
		assert.Nil(t, err)
		// Don't change custom domain
		assert.Equal(t, "https://us3.datadoghq.com/api/ui/support/serverless/flare", testURL.String())
	})

	t.Run("url with subdomain, custom domain", func(t *testing.T) {
		site := "https://datadog.random.com/test"
		testURL, err := url.Parse(site)
		assert.Nil(t, err)

		getServerlessFlareEndpoint(testURL, agentVersion)
		assert.Nil(t, err)
		// Don't change custom domain
		assert.Equal(t, "https://datadog.random.com/api/ui/support/serverless/flare", testURL.String())
	})
}

func TestTracerFlareProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != "POST" {
				w.WriteHeader(http.StatusInternalServerError)
			}

			switch req.URL.Path {
			case "/api/ui/support/serverless/flare":
				assert.Equal(t, "test", req.Header.Get("DD-API-KEY"), "got invalid API key: %q", req.Header.Get("DD-API-KEY"))
				err := req.ParseMultipartForm(1000000)
				assert.Nil(t, err)

				assert.Equal(t, "case_id", req.FormValue("case_id"))
				assert.Equal(t, "tracer_go", req.FormValue("source"))
				assert.Equal(t, "test@test.com", req.FormValue("email"))
				assert.Equal(t, "hostname", req.FormValue("hostname"))
				assert.Equal(t, "flare.zip", req.MultipartForm.File["flare_file"][0].Filename)

				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodPost, srv.URL, nil)
		assert.NoError(t, err)
		boundaryWriter := multipart.NewWriter(nil)

		req.Header.Set("Content-Type", boundaryWriter.FormDataContentType())
		req.Body = getTraceFlareBody(boundaryWriter.Boundary())
		req.ContentLength = -1

		rec := httptest.NewRecorder()
		cfg := newTestReceiverConfig()
		cfg.Site = srv.URL
		receiver := newTestReceiverFromConfig(cfg)
		handler := receiver.tracerFlareHandler()
		handler.(*httputil.ReverseProxy).Transport.(*tracerFlareTransport).getEndpoint = mockGetServerlessFlareEndpoint
		handler.(*httputil.ReverseProxy).Transport.(*tracerFlareTransport).agentVersion = "1.1.1"

		req.URL.Scheme = "http"
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid host", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {}))
		defer srv.Close()

		req, err := http.NewRequest(http.MethodPost, srv.URL, nil)
		assert.NoError(t, err)
		boundaryWriter := multipart.NewWriter(nil)

		req.Header.Set("Content-Type", boundaryWriter.FormDataContentType())
		req.Body = getTraceFlareBody(boundaryWriter.Boundary())
		req.ContentLength = -1

		rec := httptest.NewRecorder()
		cfg := newTestReceiverConfig()
		cfg.Site = srv.URL
		receiver := newTestReceiverFromConfig(cfg)
		receiver.tracerFlareHandler().ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

}
