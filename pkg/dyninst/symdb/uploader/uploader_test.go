// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SymDBRoot models the root structure for SymDB uploads, following the JSON
// schema produced by the uploader and expected by the backend.
type SymDBRoot struct {
	Service  string  `json:"service,omitempty"`
	Env      string  `json:"env,omitempty"`
	Version  string  `json:"version,omitempty"`
	Language string  `json:"language"`
	Scopes   []Scope `json:"scopes"`
	UploadID string  `json:"upload_id"`
	BatchNum int     `json:"batch_num"`
	Final    bool    `json:"final"`
}

type EventMetadata struct {
	DDSource  string `json:"ddsource"`
	Service   string `json:"service"`
	RuntimeID string `json:"runtimeId"`
}

type testServer struct {
	requests  <-chan receivedRequest
	server    *httptest.Server
	serverURL *url.URL
	close     chan struct{}
}

func (s *testServer) Close() {
	close(s.close)
	s.server.Close()
}

type receivedRequest struct {
	w    http.ResponseWriter
	r    *http.Request
	done chan struct{}
}

func newTestServer() *testServer {
	requestsC := make(chan receivedRequest)
	closeC := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doneC := make(chan struct{})
		select {
		case requestsC <- receivedRequest{w: w, r: r, done: doneC}:
		case <-closeC:
		case <-r.Context().Done():
			return
		}
		select {
		case <-doneC:
		case <-closeC:
		case <-r.Context().Done():
			return
		}
	}))
	serverURL, _ := url.Parse(server.URL)
	ts := &testServer{
		server:    server,
		serverURL: serverURL,
		requests:  requestsC,
		close:     closeC,
	}
	return ts
}
func validateSymDBRequest(
	t *testing.T,
	expectedService, expectedRuntimeID string,
	expectedUploadID uuid.UUID,
	req *http.Request,
) {
	contentType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", contentType)

	// No additional custom headers expected (consistent with other uploaders)

	reader := multipart.NewReader(req.Body, params["boundary"])

	// We expect 2 parts: "file" and "event"
	var filePart, eventPart *multipart.Part
	var fileData, eventData []byte

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		data, err := io.ReadAll(part)
		require.NoError(t, err)

		switch part.FormName() {
		case "file":
			filePart = part
			fileData = data
		case "event":
			eventPart = part
			eventData = data
		default:
			t.Errorf("unexpected form part: %s", part.FormName())
		}
	}

	// Validate both parts are present
	require.NotNil(t, filePart, "missing 'file' part in multipart request")
	require.NotNil(t, eventPart, "missing 'event' part in multipart request")

	// Validate event part metadata
	require.Equal(t, "event.json", eventPart.FileName())
	require.Equal(t, "application/json", eventPart.Header.Get("Content-Type"))

	var eventMetadata EventMetadata
	require.NoError(t, json.Unmarshal(eventData, &eventMetadata))
	require.Equal(t, "dd_debugger", eventMetadata.DDSource)
	require.Equal(t, expectedService, eventMetadata.Service)
	require.Equal(t, expectedRuntimeID, eventMetadata.RuntimeID)

	// Validate file part - it should always be compressed as file.gz
	require.Equal(t, "file.gz", filePart.FileName())
	require.Equal(t, "application/gzip", filePart.Header.Get("Content-Type"))

	// Decompress the data to validate the SymDB content
	gzReader, err := gzip.NewReader(bytes.NewReader(fileData))
	require.NoError(t, err)
	defer gzReader.Close()

	actualSymDBData, err := io.ReadAll(gzReader)
	require.NoError(t, err)

	// Validate the SymDB JSON structure
	var symdbRoot SymDBRoot
	require.NoError(t, json.Unmarshal(actualSymDBData, &symdbRoot))

	// Validate service matches
	require.Equal(t, expectedService, symdbRoot.Service)
	require.Equal(t, "go", symdbRoot.Language)
	require.Equal(t, expectedUploadID.String(), symdbRoot.UploadID)
	require.Equal(t, 1, symdbRoot.BatchNum)
	require.Equal(t, true, symdbRoot.Final)

	// Validate basic structure exists
	require.NotEmpty(t, symdbRoot.Scopes)
	require.Equal(t, "main", symdbRoot.Scopes[0].Name)
}

func createPackageScopes() []Scope {
	return []Scope{
		{
			ScopeType: ScopeTypePackage,
			Name:      "main",
			StartLine: 0,
			EndLine:   0,
			LanguageSpecifics: &LanguageSpecifics{
				AgentVersion: "7.72.0-test",
			},
			Scopes: []Scope{
				{
					ScopeType:  ScopeTypeMethod,
					Name:       "testFunction",
					SourceFile: "/test/main.go",
					StartLine:  10,
					EndLine:    20,
					Symbols: []Symbol{
						{
							Name:       "testVar",
							Type:       "string",
							SymbolType: SymbolTypeLocal,
							Line:       &[]int{12}[0],
						},
						{
							Name:       "arg1",
							Type:       "int",
							SymbolType: SymbolTypeArg,
							Line:       &[]int{10}[0],
						},
					},
				},
				{
					ScopeType: ScopeTypeStruct,
					Name:      "main.TestStruct",
					StartLine: 0,
					EndLine:   0,
					Symbols: []Symbol{
						{
							Name:       "field1",
							Type:       "string",
							SymbolType: SymbolTypeField,
						},
						{
							Name:       "field2",
							Type:       "int",
							SymbolType: SymbolTypeField,
						},
					},
					Scopes: []Scope{
						{
							ScopeType:  ScopeTypeMethod,
							Name:       "method1",
							SourceFile: "/test/main.go",
							StartLine:  25,
							EndLine:    30,
							Symbols: []Symbol{
								{
									Name:       "receiver",
									Type:       "*main.TestStruct",
									SymbolType: SymbolTypeArg,
									Line:       &[]int{25}[0],
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestBatchEncoder(t *testing.T) {
	for _, injectError := range []bool{false, true} {
		testName := "success"
		if injectError {
			testName = "failure"
		}

		t.Run(testName, func(t *testing.T) {
			ts := newTestServer()
			defer ts.Close()

			// Do a (blocking) flush in a goroutine so that the test goroutine
			// can intercept the request.
			var wg sync.WaitGroup
			wg.Add(1)
			uploadID := uuid.New()
			go func() {
				defer wg.Done()
				enc := NewBatchEncoder(
					ts.serverURL.String(),
					"service1",
					"1.0.0",
					"dummy-runtime-id",
					uploadID,
				)
				for _, s := range createPackageScopes() {
					assert.NoError(t, enc.AddScope(s))
				}
				err := enc.Flush(context.Background(), true /* final */)
				if injectError {
					assert.Error(t, err)
					assert.ErrorIs(t, err, ErrUpload)
				} else {
					assert.NoError(t, err)
				}
			}()

			req := <-ts.requests
			validateSymDBRequest(t, "service1", "dummy-runtime-id", uploadID, req.r)
			if injectError {
				req.w.WriteHeader(http.StatusInternalServerError)
			} else {
				req.w.WriteHeader(http.StatusOK)
			}
			close(req.done)
			wg.Wait()
		})
	}
}

// TestBatchEncoder_MultiBatch drives the encoder by flushing after every
// scope and verifies that batch_num increments across requests and final=true
// is set only on the last one.
func TestBatchEncoder_MultiBatch(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	scopes := createPackageScopes()
	// Duplicate the package a few times to get multiple batches.
	scopes = append(scopes, scopes[0], scopes[0])
	uploadID := uuid.New()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		enc := NewBatchEncoder(
			ts.serverURL.String(),
			"service1",
			"1.0.0",
			"dummy-runtime-id",
			uploadID,
		)
		for i, s := range scopes {
			assert.NoError(t, enc.AddScope(s))
			final := i == len(scopes)-1
			assert.NoError(t, enc.Flush(context.Background(), final))
		}
		assert.Equal(t, len(scopes), enc.BatchCount())
	}()

	for i := 0; i < len(scopes); i++ {
		req := <-ts.requests
		root := readSymDBRoot(t, req.r)
		assert.Equal(t, i+1, root.BatchNum)
		assert.Equal(t, i == len(scopes)-1, root.Final)
		assert.Equal(t, uploadID.String(), root.UploadID)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)
	}
	wg.Wait()
}

// TestBatchEncoder_Size exercises Size, which reports the underlying buffer
// length and grows monotonically as scopes accumulate (modulo gzip's internal
// buffering) and resets to zero after Flush.
func TestBatchEncoder_Size(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	enc := NewBatchEncoder(
		ts.serverURL.String(),
		"service1",
		"1.0.0",
		"dummy-runtime-id",
		uuid.New(),
	)
	require.Zero(t, enc.Size(), "Size should be zero before any scopes")

	// Add scopes until Size grows. gzip buffers internally (~32 KiB deflate
	// window), so it may take many small scopes before bytes reach the
	// underlying buffer.
	scope := createPackageScopes()[0]
	grew := false
	for i := 0; i < 100000; i++ {
		require.NoError(t, enc.AddScope(scope))
		if enc.Size() > 0 {
			grew = true
			break
		}
	}
	require.True(t, grew, "Size never grew above zero")

	// Drain the upload that Flush will send.
	doneC := make(chan error, 1)
	go func() {
		doneC <- enc.Flush(context.Background(), false /* final */)
	}()
	req := <-ts.requests
	req.w.WriteHeader(http.StatusOK)
	close(req.done)
	require.NoError(t, <-doneC)

	require.Zero(t, enc.Size(), "Size should reset after Flush")
}

// TestBatchEncoder_EmptyFinal verifies that Flush(ctx, true) with no scopes
// added is a no-op (no HTTP request).
func TestBatchEncoder_EmptyFinal(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	enc := NewBatchEncoder(
		ts.serverURL.String(),
		"service1",
		"1.0.0",
		"dummy-runtime-id",
		uuid.New(),
	)
	// No AddScope calls.
	require.NoError(t, enc.Flush(context.Background(), true))
	require.Equal(t, 0, enc.BatchCount())

	select {
	case req := <-ts.requests:
		t.Fatalf("unexpected HTTP request from empty Flush: %v", req.r.URL)
	case <-time.After(50 * time.Millisecond):
		// expected — no HTTP request issued
	}
}

// readSymDBRoot decompresses and parses the file part of a multipart SymDB
// upload request, returning the unmarshalled SymDBRoot.
func readSymDBRoot(t *testing.T, req *http.Request) SymDBRoot {
	t.Helper()
	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	require.NoError(t, err)
	reader := multipart.NewReader(req.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FormName() != "file" {
			continue
		}
		gzReader, err := gzip.NewReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer gzReader.Close()
		raw, err := io.ReadAll(gzReader)
		require.NoError(t, err)
		var root SymDBRoot
		require.NoError(t, json.Unmarshal(raw, &root))
		return root
	}
	t.Fatal("no file part in multipart request")
	return SymDBRoot{}
}
