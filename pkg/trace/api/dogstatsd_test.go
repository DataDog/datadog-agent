// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestDogStatsDReverseProxy(t *testing.T) {
	testCases := []struct {
		name       string
		configFunc func(cfg *config.AgentConfig)
		errCode    int
	}{
		{
			"dogstatsd disabled",
			func(cfg *config.AgentConfig) {
				cfg.StatsdEnabled = false
			},
			http.StatusServiceUnavailable,
		},
		{
			"bad statsd host",
			func(cfg *config.AgentConfig) {
				// Use a hostname that will fail to resolve even with AppGate enabled,
				// otherwise this test will timeout
				cfg.StatsdHost = "[invalid][host]"
			},
			http.StatusInternalServerError,
		},
		{
			"bad statsd port",
			func(cfg *config.AgentConfig) {
				cfg.StatsdPort = -1
			},
			http.StatusInternalServerError,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.New()
			tc.configFunc(cfg)
			receiver := newTestReceiverFromConfig(cfg)
			proxy := receiver.dogstatsdProxyHandler()
			require.NotNil(t, proxy)

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
			require.Equal(t, tc.errCode, rec.Code)
		})
	}

	t.Run("dogstatsd enabled (default)", func(t *testing.T) {
		cfg := config.New()
		receiver := newTestReceiverFromConfig(cfg)
		proxy := receiver.dogstatsdProxyHandler()
		require.NotNil(t, proxy)

		rec := httptest.NewRecorder()
		body := io.NopCloser(bytes.NewBufferString("users.online:1|c|@0.5|#country:china"))
		proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestDogStatsDReverseProxyPayloadRelay covers how the proxy turns a request
// body into DogStatsD payloads: newlines on their own carry no payload, and a
// body that is unreadable, over the size limit, or over the payload cap is
// reported as an error only once the payloads read before it have been relayed.
func TestDogStatsDReverseProxyPayloadRelay(t *testing.T) {
	const line = "users.online:1|c"
	testCases := []struct {
		name string
		// maxRequestBytes overrides the default body size limit when non-zero.
		maxRequestBytes int64
		body            io.Reader
		errCode         int
		// wantFirstLine is the first line of the first datagram expected to
		// reach DogStatsD, or empty when nothing at all should be relayed.
		wantFirstLine string
	}{
		{
			name:    "newlines only",
			body:    bytes.NewReader(bytes.Repeat([]byte("\n"), maxDogstatsdProxyLines)),
			errCode: http.StatusOK,
		},
		{
			// Empty lines relay nothing, but still count towards the limit.
			name:    "more newlines than the proxy scans",
			body:    bytes.NewReader(bytes.Repeat([]byte("\n"), maxDogstatsdProxyLines+1)),
			errCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:          "unreadable body",
			body:          io.MultiReader(strings.NewReader(line+"\n"), iotest.ErrReader(errors.New("read failed"))),
			errCode:       http.StatusInternalServerError,
			wantFirstLine: line,
		},
		{
			// The limit cuts the body mid way through its second line.
			name:            "body over size limit",
			maxRequestBytes: int64(len(line)) + 5,
			body:            strings.NewReader(line + "\n" + line),
			errCode:         http.StatusInternalServerError,
			wantFirstLine:   line,
		},
		{
			name:          "more payloads than the proxy relays",
			body:          strings.NewReader(strings.Repeat("x\n", maxDogstatsdProxyLines+1)),
			errCode:       http.StatusRequestEntityTooLarge,
			wantFirstLine: "x",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Bind a UDP socket so we can observe what the proxy relays.
			conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			require.NoError(t, err)
			defer conn.Close()

			cfg := config.New()
			cfg.StatsdHost = "127.0.0.1"
			cfg.StatsdPort = conn.LocalAddr().(*net.UDPAddr).Port
			if tc.maxRequestBytes != 0 {
				cfg.MaxRequestBytes = tc.maxRequestBytes
			}
			receiver := newTestReceiverFromConfig(cfg)
			proxy := receiver.dogstatsdProxyHandler()
			require.NotNil(t, proxy)

			// Drain the socket while the proxy runs, so that a case relaying
			// many payloads cannot fill the receive buffer and have the rest
			// dropped; the first payload is kept for the assertion below.
			var mu sync.Mutex
			var first []byte
			var count int
			go func() {
				buf := make([]byte, 65536)
				for {
					n, _, err := conn.ReadFrom(buf)
					if err != nil { // the socket was closed, the subtest is over
						return
					}
					mu.Lock()
					if count == 0 {
						first = append([]byte(nil), buf[:n]...)
					}
					count++
					mu.Unlock()
				}
			}()
			// Counting rather than inspecting first, so that an empty payload
			// is not mistaken for no payload at all.
			relayed := func() bool {
				mu.Lock()
				defer mu.Unlock()
				return count > 0
			}

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", tc.body))
			require.Equal(t, tc.errCode, rec.Code)

			if tc.wantFirstLine == "" {
				require.Never(t, relayed, 100*time.Millisecond, 10*time.Millisecond, "expected no payload to be relayed")
				return
			}
			require.Eventually(t, relayed, 5*time.Second, 10*time.Millisecond, "expected a payload to be relayed")
			mu.Lock()
			defer mu.Unlock()
			firstLine, _, _ := bytes.Cut(first, []byte("\n"))
			require.Equal(t, tc.wantFirstLine, string(firstLine))
		})
	}
}

// TestDogStatsDReverseProxyPacking covers how the proxy packs lines into
// datagrams.
func TestDogStatsDReverseProxyPacking(t *testing.T) {
	// Two of these, plus a newline, are one byte over the packet size cap.
	big := strings.Repeat("b", maxDogstatsdProxyPacketSize/2)
	testCases := []struct {
		name       string
		bufferSize int
		body       string
		want       []string
	}{
		{
			name:       "lines are packed until the next one does not fit",
			bufferSize: 10,
			body:       "aaa\nbbb\nccc\n",
			want:       []string{"aaa\nbbb", "ccc"},
		},
		{
			name:       "lines that fill the buffer exactly share a datagram",
			bufferSize: 7,
			body:       "aaa\nbbb",
			want:       []string{"aaa\nbbb"},
		},
		{
			name:       "a line longer than the buffer is sent alone",
			bufferSize: 5,
			body:       "aa\nbbbbbbb\ncc",
			want:       []string{"aa", "bbbbbbb", "cc"},
		},
		{
			name:       "a line exactly the buffer size is sent alone",
			bufferSize: 3,
			body:       "aaa\nbbb",
			want:       []string{"aaa", "bbb"},
		},
		{
			name:       "empty lines and carriage returns are dropped",
			bufferSize: 8192,
			body:       "a\n\n\r\nb\r\n\n",
			want:       []string{"a\nb"},
		},
		{
			name:       "packing is disabled without a buffer size",
			bufferSize: 0,
			body:       "a\nb\nc",
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "buffer sizes over the packet size cap are capped",
			bufferSize: 1 << 20,
			body:       big + "\n" + big,
			want:       []string{big, big},
		},
		{
			name:       "lines that fill the packet size cap exactly share a datagram",
			bufferSize: 1 << 20,
			body:       big[1:] + "\n" + big,
			want:       []string{big[1:] + "\n" + big},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			require.NoError(t, err)
			defer conn.Close()
			// Large enough to hold every datagram of the request at once.
			require.NoError(t, conn.SetReadBuffer(1<<20))

			cfg := config.New()
			cfg.StatsdHost = "127.0.0.1"
			cfg.StatsdPort = conn.LocalAddr().(*net.UDPAddr).Port
			cfg.StatsdBufferSize = tc.bufferSize
			proxy := newTestReceiverFromConfig(cfg).dogstatsdProxyHandler()

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)))
			require.Equal(t, http.StatusOK, rec.Code)

			// All datagrams were written before ServeHTTP returned, so a short
			// wait after the last expected one shows there are no more.
			var got []string
			buf := make([]byte, 65536)
			for {
				wait := 5 * time.Second
				if len(got) == len(tc.want) {
					wait = 100 * time.Millisecond
				}
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(wait)))
				n, _, err := conn.ReadFrom(buf)
				if err != nil {
					var netErr net.Error
					require.True(t, errors.As(err, &netErr) && netErr.Timeout(), "unexpected read error: %v", err)
					break
				}
				got = append(got, string(buf[:n]))
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDogStatsDProxyPacketSize(t *testing.T) {
	testCases := []struct {
		name       string
		ip         net.IP
		bufferSize int
		want       int
	}{
		{"IPv4 loopback", net.IPv4(127, 0, 0, 1), 8192, 8192},
		{"IPv6 loopback", net.IPv6loopback, 8192, 8192},
		{"unspecified address", net.IPv4zero, 8192, 8192},
		{"remote host", net.IPv4(10, 0, 0, 1), 8192, 0},
		{"over the cap", net.IPv4(127, 0, 0, 1), 1 << 20, maxDogstatsdProxyPacketSize},
		{"under the cap", net.IPv4(127, 0, 0, 1), 512, 512},
		{"zero", net.IPv4(127, 0, 0, 1), 0, 0},
		{"negative", net.IPv4(127, 0, 0, 1), -1, 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr := &net.UDPAddr{IP: tc.ip, Port: 8125}
			require.Equal(t, tc.want, dogstatsdProxyPacketSize(addr, tc.bufferSize))
		})
	}
}

func testDogStatsDReverseProxyEndToEndUDP(t *testing.T, cfg *config.AgentConfig) {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	for _, host := range hosts {
		t.Run(fmt.Sprintf("host=%q", host), func(t *testing.T) {
			// Bind to available port first to eliminate race condition
			addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, "0"))
			if err != nil {
				t.Fatalf("could not resolve udp addr: %s", err)
			}
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				t.Fatalf("can't listen: %s", err)
			}
			defer conn.Close()

			// Extract the actual bound port
			_, port, err := net.SplitHostPort(conn.LocalAddr().String())
			if err != nil {
				t.Fatalf("can't extract port: %s", err)
			}
			p, err := strconv.Atoi(port)
			if err != nil {
				t.Fatalf("can't convert udp port to int: %v", err)
			}

			// Configure with the bound port
			cfg.StatsdHost = host
			cfg.StatsdPort = p

			receiver := newTestReceiverFromConfig(cfg)
			proxy := receiver.dogstatsdProxyHandler()
			require.NotNil(t, proxy)
			rec := httptest.NewRecorder()

			// Send two payloads separated by a newline.
			payloads := [][]byte{[]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"), []byte("_e{21,36}:An exception occurred|Cannot parse CSV file from\\n10.0.0.17|t:warning|#err_type:bad_file")}
			sep := []byte("\n")
			msg := bytes.Join(payloads, sep)
			body := io.NopCloser(bytes.NewBuffer(msg))
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
			require.Equal(t, http.StatusOK, rec.Code)

			// Check that both payloads were packed into a single datagram.
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			buf := make([]byte, len(msg)+1)
			n, _, err := conn.ReadFrom(buf)
			require.NoError(t, err)
			if got, want := buf[:n], msg; !bytes.Equal(got, want) {
				t.Errorf("got datagram: %q\nwant datagram: %q", got, want)
			}
		})
	}
}

func TestDogStatsDReverseProxyEndToEndUDP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	t.Run("ipV4", func(t *testing.T) {
		cfg := config.New()
		cfg.StatsdHost = "127.0.0.1"
		testDogStatsDReverseProxyEndToEndUDP(t, cfg)
	})
	t.Run("ipV6", func(t *testing.T) {
		cfg := config.New()
		cfg.StatsdHost = "[::1]"
		testDogStatsDReverseProxyEndToEndUDP(t, cfg)
	})
}
