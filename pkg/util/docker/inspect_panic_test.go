// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build docker

package docker

// Regression test: DockerUtil.InspectNoCache must not let panics from the
// moby/client JSON decoder propagate up to its caller. This was observed in
// a live agent running inside an SMP regression-test container:
//
//	panic: reflect: Field index out of range
//	reflect.Value.Field(...)                                    reflect/value.go:1265
//	encoding/json.(*decodeState).object(...)                    encoding/json/decode.go:735
//	encoding/json.(*decodeState).value(...)                     encoding/json/decode.go:380
//	encoding/json.(*decodeState).object(...)                    encoding/json/decode.go:767
//	encoding/json.(*decodeState).value(...)                     encoding/json/decode.go:380
//	encoding/json.(*decodeState).unmarshal(...)                 encoding/json/decode.go:183
//	encoding/json.(*Decoder).Decode(...)                        encoding/json/stream.go:75
//	github.com/moby/moby/client.decodeWithRaw[...](...)         moby/client@v0.4.0/utils.go:125
//	github.com/moby/moby/client.(*Client).ContainerInspect(...) moby/client@v0.4.0/container_inspect.go:45
//	DataDog/datadog-agent/pkg/util/docker.(*DockerUtil).InspectNoCache(...)
//	        pkg/util/docker/docker_util.go:341
//	DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker.
//	        (*collector).buildCollectorEvent/.handleContainerEvent/.stream(...)
//	AGENT EXITED WITH CODE 2, SIGNAL 0, KILLING CONTAINER
//
// The root-cause class is inside Go's encoding/json + reflect interaction
// with moby/client@v0.4.0's InspectResponse struct. We could not reproduce
// the exact upstream panic with a natural Docker response body under any
// combination of synthetic mutations or concurrent decoding we tried —
// whatever Docker daemon / API version SMP's runner exposes to the agent
// emits a response shape we can't cleanly reconstruct here.
//
// What we CAN reliably reproduce: *any* panic inside the moby client's HTTP
// response path (decoder, body reader, transport) propagates through
// InspectNoCache unchanged. The test below triggers that code path with a
// body whose Read method panics. It documents the current fragility so
// callers (notably the workloadmeta docker collector's stream goroutine,
// which has no recovery) can be hardened independently.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// mockHTTPTransport turns any Docker API request into a fixed response. It
// short-circuits moby's /_ping API-version negotiation so tests can focus on
// the ContainerInspect path.
type mockHTTPTransport struct {
	ping    func(*http.Request) (*http.Response, error)
	inspect func(*http.Request) (*http.Response, error)
}

func (m *mockHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/_ping":
		if m.ping != nil {
			return m.ping(req)
		}
		return okPingResponse(req), nil
	default:
		if m.inspect != nil {
			return m.inspect(req)
		}
		return nil, errors.New("no handler for " + req.URL.Path)
	}
}

func okPingResponse(req *http.Request) *http.Response {
	h := make(http.Header)
	h.Set("Api-Version", "1.51")
	h.Set("Ostype", "linux")
	h.Set("Server", "Docker/27.0.0 (linux)")
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(bytes.NewReader([]byte("OK"))),
		Request:    req,
		Header:     h,
	}
}

func newMockDockerUtil(t *testing.T, inspect func(*http.Request) (*http.Response, error)) *DockerUtil {
	t.Helper()
	httpc := &http.Client{
		Transport: &mockHTTPTransport{inspect: inspect},
	}
	cli, err := client.New(client.WithHTTPClient(httpc))
	require.NoError(t, err, "constructing mock docker client")
	return &DockerUtil{
		cfg:            &Config{CollectNetwork: false},
		cli:            cli,
		queryTimeout:   5 * time.Second,
		imageNameBySha: make(map[string]string),
	}
}

// panickingBody satisfies io.ReadCloser but panics when Read is called.
// This simulates a panic deep inside the moby client's response-body path
// — matching the shape of the upstream json/reflect panic we observed in
// production, without relying on any specific malformed JSON payload.
type panickingBody struct{ msg string }

func (p *panickingBody) Read(_ []byte) (int, error) {
	panic(p.msg)
}

func (p *panickingBody) Close() error { return nil }

func inspectResponseWithBody(req *http.Request, body io.ReadCloser) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          body,
		Request:       req,
		Header:        h,
		ContentLength: -1,
	}
}

// TestInspectNoCache_NormalResponse is a sanity check: with a well-formed
// ContainerInspect response, InspectNoCache returns the decoded data.
func TestInspectNoCache_NormalResponse(t *testing.T) {
	du := newMockDockerUtil(t, func(req *http.Request) (*http.Response, error) {
		// Return a minimal valid InspectResponse by JSON-marshalling it. This
		// avoids hand-rolling the response shape and keeps the test robust
		// against moby API changes.
		wantID := "container_id_42"
		resp := dcontainer.InspectResponse{
			ID:    wantID,
			Image: "image",
			Name:  "name",
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		return inspectResponseWithBody(req, io.NopCloser(bytes.NewReader(b))), nil
	})

	got, err := du.InspectNoCache(t.Context(), "container_id_42", false)
	require.NoError(t, err)
	require.Equal(t, "container_id_42", got.ID)
	require.Equal(t, "image", got.Image)
	require.Equal(t, "name", got.Name)
}

// TestInspectNoCache_PanicInResponseBody_PropagatesUnrecovered documents the
// current failure mode: if anything in the JSON-decoding path (or the
// response body path) panics, InspectNoCache does NOT recover. The panic
// propagates to whichever goroutine called InspectNoCache, and in production
// that goroutine is the workloadmeta docker collector's stream() loop, which
// also does not recover — so the whole agent process dies.
//
// This test asserts the current behaviour (the panic escapes InspectNoCache)
// so a fix that adds recovery at either layer can be verified with a mirror
// test that flips the assertion.
func TestInspectNoCache_PanicInResponseBody_PropagatesUnrecovered(t *testing.T) {
	const panicMsg = "simulated moby/json decode panic"

	du := newMockDockerUtil(t, func(req *http.Request) (*http.Response, error) {
		return inspectResponseWithBody(req, &panickingBody{msg: panicMsg}), nil
	})

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = du.InspectNoCache(t.Context(), "anything", false)
	}()

	require.NotNilf(t, recovered,
		"expected InspectNoCache to NOT recover the inner panic; current "+
			"behaviour is that panics from the moby/client JSON decoder escape "+
			"all the way up to the caller's goroutine. If this assertion starts "+
			"failing, InspectNoCache (or a layer above it) has learned to "+
			"recover such panics \u2014 flip this assertion to require.Nil and "+
			"update the companion test in the docker workloadmeta collector.")
	require.Equalf(t, panicMsg, fmt.Sprintf("%v", recovered),
		"expected the original panic value to propagate unchanged")
}
