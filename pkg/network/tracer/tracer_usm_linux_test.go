// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
	gorilla "github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	prototls "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/openssl"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/pcap"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	grpc2 "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func httpSupported() bool {
	if isFentry() {
		return false
	}
	// kv is declared in `tracer_linux_test.go`.
	return kv >= http.MinimumKernelVersion
}

func httpsSupported() bool {
	if isFentry() {
		return false
	}
	return http.TLSSupported(testConfig())
}

func classificationSupported(config *config.Config) bool {
	return kprobe.ClassificationSupported(config)
}

type USMSuite struct {
	suite.Suite
}

func TestUSMSuite(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(USMSuite))
	})
}

func (s *USMSuite) TestEnableHTTPMonitoring() {
	t := s.T()
	if !httpSupported() {
		t.Skip("HTTP monitoring not supported")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	_ = setupTracer(t, cfg, false)
}

func (s *USMSuite) TestProtocolClassification() {
	t := s.T()
	cfg := testConfig()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}

	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	t.Run("with dnat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
		netlink.SetupDNAT(t)
		testProtocolClassification(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testHTTPSClassification(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "2.2.2.2", "1.1.1.1:0")
	})

	t.Run("with snat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 6.6.6.6 to 7.7.7.7
		netlink.SetupSNAT(t)
		testProtocolClassification(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testHTTPSClassification(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
	})

	t.Run("without nat", func(t *testing.T) {
		testProtocolClassification(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testHTTPSClassification(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}

func testProtocolConnectionProtocolMapCleanup(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	t.Run("protocol cleanup", func(t *testing.T) {
		t.Cleanup(func() { tr.ebpfTracer.Pause() })

		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP:   net.ParseIP(clientHost),
				Port: 0,
			},
			Control: func(network, address string, c syscall.RawConn) error {
				var opErr error
				err := c.Control(func(fd uintptr) {
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				})
				if err != nil {
					return err
				}
				return opErr
			},
		}

		initTracerState(t, tr)
		require.NoError(t, tr.ebpfTracer.Resume())

		mux := gorilla.NewRouter()
		mux.Handle("/test", nethttp.DefaultServeMux)
		grpcHandler := grpc.NewServerWithoutBind()

		lis, err := net.Listen("tcp", serverHost)
		require.NoError(t, err)
		srv := grpc2.NewMuxedGRPCServer(serverHost, nil, grpcHandler.GetGRPCServer(), mux)
		srv.Addr = lis.Addr().String()

		go srv.Serve(lis)
		t.Cleanup(func() {
			_ = srv.Shutdown(context.Background())
			_ = lis.Close()
		})
		_, port, err := net.SplitHostPort(srv.Addr)
		require.NoError(t, err)
		targetAddr := net.JoinHostPort(targetHost, port)

		// Running a HTTP client
		client := nethttp.Client{
			Transport: &nethttp.Transport{
				DialContext: dialer.DialContext,
			},
		}
		resp, err := client.Post("http://"+targetAddr+"/test", "text/plain", bytes.NewReader(bytes.Repeat([]byte("test"), 100)))
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		client.CloseIdleConnections()
		waitForConnectionsWithProtocol(t, tr, targetAddr, srv.Addr, &protocols.Stack{Application: protocols.HTTP})

		grpcClient, err := grpc.NewClient(targetAddr, grpc.Options{
			CustomDialer: dialer,
		}, false)
		require.NoError(t, err)
		defer grpcClient.Close()
		_ = grpcClient.HandleUnary(context.Background(), "test")
		waitForConnectionsWithProtocol(t, tr, targetAddr, srv.Addr, &protocols.Stack{API: protocols.GRPC, Application: protocols.HTTP2})
	})
}

type tlsTestCommand struct {
	version        string
	openSSLCommand string
}

func getFreePort() (port uint16, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return uint16(l.Addr().(*net.TCPAddr).Port), nil
		}
	}
	return
}

// TLS classification tests
func (s *USMSuite) TestTLSClassification() {
	t := s.T()
	cfg := testConfig()
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

	scenarios := []tlsTestCommand{
		{
			version:        "1.0",
			openSSLCommand: "-tls1",
		},
		{
			version:        "1.1",
			openSSLCommand: "-tls1_1",
		},
		{
			version:        "1.2",
			openSSLCommand: "-tls1_2",
		},
		{
			version:        "1.3",
			openSSLCommand: "-tls1_3",
		},
	}

	port, err := getFreePort()
	require.NoError(t, err)
	portAsString := strconv.Itoa(int(port))
	require.NoError(t, prototls.RunServerOpenssl(t, portAsString, len(scenarios), "-www"))

	tr := setupTracer(t, cfg, false)

	type tlsTest struct {
		name            string
		postTracerSetup func(t *testing.T)
		validation      func(t *testing.T, tr *Tracer)
	}
	tests := make([]tlsTest, 0, len(scenarios))
	for _, scenario := range scenarios {
		tests = append(tests, tlsTest{
			name: "TLS-" + scenario.version + "_docker",
			postTracerSetup: func(t *testing.T) {
				require.True(t, prototls.RunClientOpenssl(t, "localhost", portAsString, scenario.openSSLCommand))
			},
			validation: func(t *testing.T, tr *Tracer) {
				// Iterate through active connections until we find connection created above
				require.Eventuallyf(t, func() bool {
					payload := getConnections(t, tr)
					for _, c := range payload.Conns {
						if c.DPort == port && c.ProtocolStack.Contains(protocols.TLS) {
							return true
						}
					}
					return false
				}, 4*time.Second, 100*time.Millisecond, "couldn't find TLS connection matching: dst port %v", portAsString)
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tr.ebpfTracer.Type() == connection.TracerTypeFentry {
				t.Skip("protocol classification not supported for fentry tracer")
			}
			t.Cleanup(func() { tr.removeClient(clientID) })
			t.Cleanup(func() { _ = tr.ebpfTracer.Pause() })

			tr.removeClient(clientID)
			initTracerState(t, tr)
			require.NoError(t, tr.ebpfTracer.Resume(), "enable probes - before post tracer")
			tt.postTracerSetup(t)
			require.NoError(t, tr.ebpfTracer.Pause(), "disable probes - after post tracer")
			tt.validation(t, tr)
		})
	}
}

func (s *USMSuite) TestTLSClassificationAlreadyRunning() {
	t := s.T()

	cfg := testConfig()
	cfg.ProtocolClassificationEnabled = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

	t.Run("Already running python server", func(t *testing.T) {
		serverAddr := net.JoinHostPort("localhost", httpsPort)
		portAsValue, err := strconv.ParseUint(httpsPort, 10, 16)
		require.NoError(t, err)

		_ = testutil.HTTPPythonServer(t, serverAddr, testutil.Options{
			EnableKeepAlive: false,
			EnableTLS:       true,
		})

		client := &nethttp.Client{
			Transport: &nethttp.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		// Helper to make a request to the server, and discard the response.
		makeRequest := func() {
			resp, err := client.Get(fmt.Sprintf("https://%s/200/test", serverAddr))
			require.NoError(t, err)

			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		makeRequest()
		tr := setupTracer(t, cfg, false)
		time.Sleep(100 * time.Millisecond)
		makeRequest()

		// Iterate through active connections until we find connection created above
		var foundIncoming, foundOutgoing bool
		require.Eventuallyf(t, func() bool {
			payload := getConnections(t, tr)

			for _, c := range payload.Conns {
				if !foundIncoming && c.DPort == uint16(portAsValue) && c.ProtocolStack.Contains(protocols.TLS) {
					foundIncoming = true
				}

				if !foundOutgoing && c.SPort == uint16(portAsValue) && c.ProtocolStack.Contains(protocols.TLS) {
					foundOutgoing = true
				}
			}
			return foundIncoming && foundOutgoing
		}, 4*time.Second, 100*time.Millisecond, "couldn't find matching TLS connection")
	})

	t.Run("From PCAP", func(t *testing.T) {
		tr := setupTracer(t, cfg, tracerNoStart)

		prog := tr.ebpfTracer.GetProg("socket__classifier_entry")
		require.NotNil(t, prog)

		curDir, _ := testutil.CurDir()
		pktSource := pcap.GetPacketSourceFromPCAP(t, curDir+"/testdata/tls_already_running.pcap")

		for packet := range pktSource.Packets() {
			// HACK: For some reason, the first 14bytes are skipped, so we pad
			// the front of the packet data to trick it.
			data := make([]byte, 14)
			data = append(data, packet.Data()...)

			_, _, err := prog.Test(data)
			require.NoError(t, err)
		}

		// Print connections tuples and protocol stacks
		iter := tr.ebpfTracer.GetMap(probes.ConnectionProtocolMap).Iterate()
		var key http.ConnTuple
		var value ebpf.ProtocolStackWrapper
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Dump(key, value)
		}
	})
}

func skipIfHTTPSNotSupported(t *testing.T, _ testContext) {
	if !httpsSupported() {
		t.Skip("https is not supported")
	}
}

func testHTTPSClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfHTTPSNotSupported)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    httpsPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	serverAddress := net.JoinHostPort(serverHost, httpsPort)
	targetAddress := net.JoinHostPort(targetHost, httpsPort)
	tests := []protocolClassificationAttributes{
		{
			name: "HTTPs request",
			context: testContext{
				serverPort:    httpsPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				cmd := testutil.HTTPPythonServer(t, ctx.serverAddress, testutil.Options{
					EnableKeepAlive: false,
					EnableTLS:       true,
				})
				ctx.extras["cmd"] = cmd
			},
			validation: func(t *testing.T, ctx testContext, tr *Tracer) {
				cmd := ctx.extras["cmd"].(*exec.Cmd)
				utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid)
				client := nethttp.Client{
					Transport: &nethttp.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						DialContext:     defaultDialer.DialContext,
					},
				}

				// Ensure that we see HTTPS requests being traced *before* the actual test assertions
				// This is done to reduce test test flakiness due to uprobe attachment delays
				require.Eventually(t, func() bool {
					resp, err := client.Get(fmt.Sprintf("https://%s/200/warm-up", ctx.targetAddress))
					if err != nil {
						return false
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()

					httpData := getConnections(t, tr).HTTP
					for httpKey := range httpData {
						if httpKey.Path.Content.Get() == resp.Request.URL.Path {
							return true
						}
					}

					return false
				}, 5*time.Second, 100*time.Millisecond, "couldn't detect HTTPS traffic being traced (test setup validation)")

				t.Log("run 3 clients request as we can have a race between the closing tcp socket and the http response")
				for i := 0; i < 3; i++ {
					resp, err := client.Get(fmt.Sprintf("https://%s/200/request-1", ctx.targetAddress))
					require.NoError(t, err)
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					client.CloseIdleConnections()
				}

				waitForConnectionsWithProtocol(t, tr, ctx.targetAddress, ctx.serverAddress, &protocols.Stack{Encryption: protocols.TLS, Application: protocols.HTTP})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func TestFullMonitorWithTracer(t *testing.T) {
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableKafkaMonitoring = true
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableIstioMonitoring = true
	cfg.EnableGoTLSSupport = true

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	initTracerState(t, tr)
}
