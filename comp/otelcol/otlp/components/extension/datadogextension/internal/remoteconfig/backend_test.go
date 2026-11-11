// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"google.golang.org/protobuf/proto"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestCollectorBackendTrustAndRedirectPolicy(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	t.Cleanup(target.Close)
	var redirect atomic.Bool
	want := &pbgo.OrgDataResponse{Uuid: "test-org"}
	body, err := proto.Marshal(want)
	require.NoError(t, err)
	var connections atomic.Int32
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, []string{"collector-secret"}, r.Header.Values("DD-API-KEY"))
		if redirect.Load() {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write(body)
	}))
	backend.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			connections.Add(1)
		case http.StateClosed:
			connections.Add(-1)
		}
	}
	backend.StartTLS()
	t.Cleanup(backend.Close)
	e := testExtension(t, &fakeService{})
	u, err := url.Parse(backend.URL)
	require.NoError(t, err)
	client, err := e.newAPIClient(t.Context(), componenttest.NewNopHost(), configmock.New(t), u)
	require.NoError(t, err)
	_, err = client.FetchOrgData(t.Context())
	require.Error(t, err, "the backend's private CA must not be trusted implicitly")
	client.CloseIdleConnections()

	ca := filepath.Join(t.TempDir(), "backend-ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backend.Certificate().Raw}), 0600))
	e.config.Backend.TLS.CAFile = ca
	client, err = e.newAPIClient(t.Context(), componenttest.NewNopHost(), configmock.New(t), u)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	got, err := client.FetchOrgData(t.Context())
	require.NoError(t, err)
	require.True(t, proto.Equal(want, got))
	redirect.Store(true)
	_, err = client.FetchOrgData(t.Context())
	require.Error(t, err)
	require.Zero(t, redirected.Load(), "Collector authentication must stay at the configured origin")
	client.CloseIdleConnections()
	require.Eventually(t, func() bool { return connections.Load() == 0 }, time.Second, 10*time.Millisecond,
		"OTel instrumentation must preserve cleanup of the owned connection pool")
}
