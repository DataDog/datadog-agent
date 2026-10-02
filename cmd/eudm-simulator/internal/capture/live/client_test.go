// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestInstalledClientUsesIPCTrustAuthenticationAndAcknowledgement(t *testing.T) {
	auth := ipcmock.New(t)
	manager := tc.NewManager("core-agent", "7.85.0", strings.Repeat("b", 40))
	defer manager.Close()
	if err := manager.Register(tc.Capability{Stream: tc.Software, Cadence: time.Minute}); err != nil {
		t.Fatal(err)
	}
	handler, err := manager.Handler(auth.HTTPMiddleware)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/agent/eudm-capture/", http.StripPrefix("/agent/eudm-capture", handler))
	server := auth.NewMockServer(mux)
	address, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(address.Host)
	cfg := config.NewMock(t)
	cfg.Set("cmd_host", host, configmodel.SourceAgentRuntime)
	cfg.Set("cmd_port", port, configmodel.SourceAgentRuntime)
	clients, closeClients, err := installedClients(cfg, auth, false)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClients()
	client := clients[0]
	ctx := context.Background()
	status, err := client.Capabilities(ctx)
	if err != nil || status.Producer != manager.Status().Producer {
		t.Fatalf("IPC capabilities: %v", err)
	}
	control := tc.Control{ProtocolVersion: tc.ProtocolVersion, SessionID: "http-client-session-0001"}
	if _, err := client.Prepare(ctx, tc.PrepareRequest{Control: control, Streams: []tc.Stream{tc.Software}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Activate(ctx, control); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Heartbeat(ctx, control); err != nil {
		t.Fatal(err)
	}
	payload := tc.Payload{Software: &tc.Message{Body: []byte("owned snapshot fixture"), Timestamp: 123}}
	if !manager.Observe(tc.Software, time.Now(), time.Minute, tc.PayloadSize(payload), func() tc.Payload { return payload }) {
		t.Fatal("observation failed")
	}
	read := tc.ReadRequest{Control: control}
	first, err := client.Records(ctx, read)
	if err != nil || len(first.Records) != 1 || !reflect.DeepEqual(first.Records[0].Payload, payload) {
		t.Fatalf("bounded record read: %v", err)
	}
	repeated, err := client.Records(ctx, read)
	if err != nil || !reflect.DeepEqual(first.Records, repeated.Records) {
		t.Fatal("repeated cursor changed its logical cycle")
	}
	if status, err = client.Stop(ctx, control); err != nil || status.State != tc.Stopping {
		t.Fatalf("stop prematurely acknowledged unconsumed record: %v", err)
	}
	read.Cursor = first.Records[0].Sequence
	if _, err := client.Records(ctx, read); err != nil {
		t.Fatal(err)
	}
	if status, err = client.Stop(ctx, control); err != nil || status.State != tc.Stopped || status.Acknowledged != status.FinalSequence {
		t.Fatalf("final acknowledgement: %v", err)
	}
}

func TestHTTPClientRejectsAuthenticationIncompatibilityAndRedirects(t *testing.T) {
	for _, code := range []int{401, 403, 404, 405, 409, 503, 302} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing IPC token")
				}
				if r.URL.Path != "/capabilities" {
					t.Error("followed a capture API redirect")
				}
				w.Header().Set("Location", "/private-redirect")
				w.WriteHeader(code)
				_, _ = io.WriteString(w, "native-secret-from-error-body")
			}))
			defer server.Close()
			client := newHTTPClient("core-agent", server.URL, "test-token", server.Client())
			_, err := client.Capabilities(context.Background())
			if err == nil || strings.Contains(err.Error(), "native-secret") {
				t.Fatal("unsafe API failure")
			}
			if (code == 401 || code == 403) && !errors.Is(err, ErrAuthentication) {
				t.Fatal(err)
			}
			if (code == 404 || code == 405) && !errors.Is(err, ErrIncompatible) {
				t.Fatal(err)
			}
			if code == 503 && !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
}

func TestBoundedClientRejectsMalformedBatches(t *testing.T) {
	tooMany, err := json.Marshal(Batch{Records: make([]tc.Record, tc.MaxBatchRecords+1)})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`null`, `{"status":{},"records":null}`, `{"status":{},"records":[]}{}`,
		`{"status":{},"records":[],"records":[]}`, `{"status":{},"records":[],"unknown":"native-secret"}`,
		`{"status":{},"records":[{"native-secret":true}]}`, `{"status":{},"records":[`, string(tooMany),
	} {
		if _, err := decodeBatch(strings.NewReader(input)); err == nil || strings.Contains(err.Error(), "native-secret") {
			t.Fatal("invalid batch was accepted or leaked content")
		}
	}
	if _, err := decodeBatch(strings.NewReader(`{"records":[],"status":{}}`)); err != nil {
		t.Fatal("JSON field order must not affect the protocol")
	}
}

func TestHTTPClientBoundsStatusAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"producer":{"role":"%s"}}`, strings.Repeat("x", int(maxStatusBytes)))
	}))
	defer server.Close()
	client := newHTTPClient("core-agent", server.URL, "test-token", server.Client())
	if _, err := client.Capabilities(context.Background()); !errors.Is(err, errResponse) {
		t.Fatalf("unbounded status accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Capabilities(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost request cancellation: %v", err)
	}
}

func TestRecordResponseBodyPreservesCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":`)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newHTTPClient("core-agent", server.URL, "test-token", server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Records(ctx, tc.ReadRequest{})
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation lost its cause: %v", err)
	}
}
