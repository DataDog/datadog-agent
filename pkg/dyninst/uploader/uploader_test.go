// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

func validateDiagnosticsRequest(
	t *testing.T, expectedMessages []*DiagnosticMessage, req *http.Request,
) {
	contentType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", contentType)

	reader := multipart.NewReader(req.Body, params["boundary"])
	part, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "event", part.FormName())
	require.Equal(t, "event.json", part.FileName())

	data, err := io.ReadAll(part)
	require.NoError(t, err)

	var batch []*DiagnosticMessage
	require.NoError(t, json.Unmarshal(data, &batch))
	require.Len(t, batch, len(expectedMessages))
	require.EqualValues(t, expectedMessages, batch)
}

func validateLogsRequest(t *testing.T, expectedMessages []json.RawMessage, req *http.Request) {
	contentType := req.Header.Get("Content-Type")
	require.Equal(t, "application/json", contentType)

	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var batch []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &batch))
	require.Len(t, batch, len(expectedMessages))

	for i, msg := range expectedMessages {
		assert.Equal(t, string(msg), string(batch[i]))
	}
}

func validateSymDBRequest(
	t *testing.T, expectedService, expectedRuntimeID string, req *http.Request,
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

	// Validate basic structure exists
	require.NotEmpty(t, symdbRoot.Scopes)
	require.Equal(t, "main", symdbRoot.Scopes[0].Name)
}

func TestDiagnosticsUploader(t *testing.T) {
	computeExpectedBytes := func(messages []*DiagnosticMessage) int {
		var expectedBytes int
		for _, msg := range messages {
			msg := *msg
			msg.Timestamp = time.Now().UnixMilli()
			msgBytes, err := json.Marshal(&msg)
			require.NoError(t, err)
			expectedBytes += len(msgBytes)
		}
		return expectedBytes
	}

	t.Run("success", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(0), // disable timers
		)
		defer uploader.Stop()

		expectedMessages := []*DiagnosticMessage{
			NewDiagnosticMessage("service1", Diagnostic{
				ProbeID: "probe1",
				Status:  StatusReceived,
			}),
			NewDiagnosticMessage("service2", Diagnostic{
				ProbeID: "probe2",
				Status:  StatusInstalled,
			}),
		}

		expectedBytes := computeExpectedBytes(expectedMessages)
		for _, msg := range expectedMessages {
			require.NoError(t, uploader.Enqueue(msg))
		}

		req := <-ts.requests
		validateDiagnosticsRequest(t, expectedMessages, req.r)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(expectedMessages)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("success with timer", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(10*time.Millisecond),
		)
		defer uploader.Stop()

		expectedMessages := []*DiagnosticMessage{
			NewDiagnosticMessage("service1", Diagnostic{
				ProbeID: "probe1",
				Status:  StatusReceived,
				DiagnosticException: &DiagnosticException{
					Message: "test",
				},
			}),
		}

		expectedBytes := computeExpectedBytes(expectedMessages)
		for _, msg := range expectedMessages {
			require.NoError(t, uploader.Enqueue(msg))
		}

		req := <-ts.requests
		validateDiagnosticsRequest(t, expectedMessages, req.r)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(expectedMessages)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("failure", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(1),
		)
		defer uploader.Stop()

		msg1 := NewDiagnosticMessage(
			"service1", Diagnostic{ProbeID: "probe1", Status: StatusInstalled},
		)
		require.NoError(t, uploader.Enqueue(msg1))

		req := <-ts.requests
		validateDiagnosticsRequest(t, []*DiagnosticMessage{msg1}, req.r)
		req.w.WriteHeader(http.StatusInternalServerError)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 0,
				"bytes_sent":   0,
				"items_sent":   0,
				"errors":       1,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestLogsUploader(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploaderFactory := NewLogsUploaderFactory(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
		)
		defer uploaderFactory.Stop()
		uploader := uploaderFactory.GetUploader(LogsUploaderMetadata{
			Tags: "service:test",
		})

		msg1 := json.RawMessage(`{"key":"value1"}`)
		msg2 := json.RawMessage(`{"key":"value2"}`)

		uploader.Enqueue(msg1)
		uploader.Enqueue(msg2)

		// receive and validate request
		req := <-ts.requests
		validateLogsRequest(t, []json.RawMessage{msg1, msg2}, req.r)

		// send response and unblock handler
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(len(msg1) + len(msg2)),
				"items_sent":   2,
				"errors":       0,
			}, uploaderFactory.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("failure", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploaderFactory := NewLogsUploaderFactory(
			WithURL(ts.serverURL),
			WithMaxBatchItems(1),
		)
		defer uploaderFactory.Stop()
		uploader := uploaderFactory.GetUploader(LogsUploaderMetadata{
			Tags: "service:test",
		})

		msg1 := json.RawMessage(`{"key":"value1"}`)
		uploader.Enqueue(msg1)

		// receive request
		req := <-ts.requests
		validateLogsRequest(t, []json.RawMessage{msg1}, req.r)

		// send failure response
		req.w.WriteHeader(http.StatusInternalServerError)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 0,
				"bytes_sent":   0,
				"items_sent":   0,
				"errors":       1,
			}, uploaderFactory.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestSymDBUploader(t *testing.T) {
	createTestSymDBRoot := func(service string) SymDBRoot {
		return SymDBRoot{
			Service:  service,
			Env:      "test",
			Version:  "1.0",
			Language: "go",
			Scopes: []Scope{
				{
					ScopeType: ScopeTypePackage,
					Name:      "main",
					StartLine: 0,
					EndLine:   0,
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
									LanguageSpecifics: &LanguageSpecifics{
										AvailableLineRanges: []LineRange{
											{Start: 12, End: 18},
										},
									},
								},
								{
									Name:       "arg1",
									Type:       "int",
									SymbolType: SymbolTypeArg,
									Line:       &[]int{10}[0],
									LanguageSpecifics: &LanguageSpecifics{
										AvailableLineRanges: []LineRange{
											{Start: 10, End: 20},
										},
									},
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
											LanguageSpecifics: &LanguageSpecifics{
												AvailableLineRanges: []LineRange{
													{Start: 25, End: 30},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	computeExpectedBytes := func(services []string) int {
		var expectedBytes int
		for _, service := range services {
			root := createTestSymDBRoot(service)
			symdbBytes, err := json.Marshal(root)
			require.NoError(t, err)

			metadata := EventMetadata{
				DDSource:  "dd_debugger",
				Service:   service,
				RuntimeID: "test-runtime-" + service,
			}

			msg := map[string]interface{}{
				"metadata": metadata,
				"symdb":    json.RawMessage(symdbBytes),
			}
			msgBytes, err := json.Marshal(msg)
			require.NoError(t, err)
			expectedBytes += len(msgBytes)
		}
		return expectedBytes
	}

	t.Run("success", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewSymDBUploader(
			"service1",
			"env",
			"1.0.0",
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(0), // disable timers
		)
		defer uploader.Stop()

		testServices := []string{"service1", "service2"}

		expectedBytes := computeExpectedBytes(testServices)
		for _, service := range testServices {
			root := createTestSymDBRoot(service)
			runtimeID := "test-runtime-" + service
			require.NoError(t, uploader.Enqueue(service, runtimeID, root))
		}

		// We expect 2 separate HTTP requests (one per service)
		for _, service := range testServices {
			req := <-ts.requests
			validateSymDBRequest(t, service, "test-runtime-"+service, req.r)
			req.w.WriteHeader(http.StatusOK)
			close(req.done)
		}

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(testServices)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("success with timer", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewSymDBUploader(
			"service1",
			"env",
			"1.0.0",
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(10*time.Millisecond),
		)
		defer uploader.Stop()

		testServices := []string{"service1"}

		expectedBytes := computeExpectedBytes(testServices)
		for _, service := range testServices {
			root := createTestSymDBRoot(service)
			runtimeID := "test-runtime-" + service
			require.NoError(t, uploader.Enqueue(service, runtimeID, root))
		}

		req := <-ts.requests
		validateSymDBRequest(t, "service1", "test-runtime-service1", req.r)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(testServices)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("failure", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewSymDBUploader(
			"service1",
			"env",
			"1.0.0",
			WithURL(ts.serverURL),
			WithMaxBatchItems(1),
		)
		defer uploader.Stop()

		msg1 := createTestMessage("service1")
		require.NoError(t, uploader.Enqueue(msg1))

		req := <-ts.requests
		validateSymDBRequest(t, []*SymDBMessage{msg1}, req.r)
		req.w.WriteHeader(http.StatusInternalServerError)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 0,
				"bytes_sent":   0,
				"items_sent":   0,
				"errors":       1,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("schema validation", func(t *testing.T) {
		// Test that our schema conversion produces valid JSON structure
		msg := createTestMessage("test-service")

		// Validate the message structure
		require.Equal(t, "test-service", msg.Service)
		require.NotNil(t, msg.SymDB)

		// Validate the schema structure
		var root SymDBRoot
		require.NoError(t, json.Unmarshal(msg.SymDB, &root))

		require.Equal(t, "test-service", root.Service)
		require.Equal(t, "go", root.Language)
		require.Len(t, root.Scopes, 1)

		// Validate package scope
		packageScope := root.Scopes[0]
		require.Equal(t, ScopeTypePackage, packageScope.ScopeType)
		require.Equal(t, "main", packageScope.Name)
		require.Len(t, packageScope.Scopes, 2) // 1 function + 1 type

		// Find function scope
		var functionScope *Scope
		var typeScope *Scope
		for i := range packageScope.Scopes {
			if packageScope.Scopes[i].ScopeType == ScopeTypeMethod {
				functionScope = &packageScope.Scopes[i]
			} else if packageScope.Scopes[i].ScopeType == ScopeTypeStruct {
				typeScope = &packageScope.Scopes[i]
			}
		}

		// Validate function scope
		require.NotNil(t, functionScope)
		require.Equal(t, "testFunction", functionScope.Name)
		require.Equal(t, "/test/main.go", functionScope.SourceFile)
		require.Equal(t, 10, functionScope.StartLine)
		require.Equal(t, 20, functionScope.EndLine)
		require.Len(t, functionScope.Symbols, 2)

		// Validate function symbols
		var argSymbol, localSymbol *Symbol
		for i := range functionScope.Symbols {
			if functionScope.Symbols[i].SymbolType == SymbolTypeArg {
				argSymbol = &functionScope.Symbols[i]
			} else if functionScope.Symbols[i].SymbolType == SymbolTypeLocal {
				localSymbol = &functionScope.Symbols[i]
			}
		}

		require.NotNil(t, argSymbol)
		require.Equal(t, "arg1", argSymbol.Name)
		require.Equal(t, "int", argSymbol.Type)
		require.NotNil(t, argSymbol.LanguageSpecifics)
		require.Len(t, argSymbol.LanguageSpecifics.AvailableLineRanges, 1)
		require.Equal(t, 10, argSymbol.LanguageSpecifics.AvailableLineRanges[0].Start)
		require.Equal(t, 20, argSymbol.LanguageSpecifics.AvailableLineRanges[0].End)

		require.NotNil(t, localSymbol)
		require.Equal(t, "testVar", localSymbol.Name)
		require.Equal(t, "string", localSymbol.Type)

		// Validate type scope
		require.NotNil(t, typeScope)
		require.Equal(t, ScopeTypeStruct, typeScope.ScopeType)
		require.Equal(t, "main.TestStruct", typeScope.Name)
		require.Len(t, typeScope.Symbols, 2) // 2 fields
		require.Len(t, typeScope.Scopes, 1)  // 1 method

		// Validate fields
		require.Equal(t, SymbolTypeField, typeScope.Symbols[0].SymbolType)
		require.Equal(t, "field1", typeScope.Symbols[0].Name)
		require.Equal(t, "string", typeScope.Symbols[0].Type)

		// Validate method
		methodScope := typeScope.Scopes[0]
		require.Equal(t, ScopeTypeMethod, methodScope.ScopeType)
		require.Equal(t, "method1", methodScope.Name)
		require.Len(t, methodScope.Symbols, 1) // receiver
		require.Equal(t, SymbolTypeArg, methodScope.Symbols[0].SymbolType)
		require.Equal(t, "receiver", methodScope.Symbols[0].Name)
		require.Equal(t, "*main.TestStruct", methodScope.Symbols[0].Type)
	})
}
