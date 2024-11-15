// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tests

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gorilla "github.com/gorilla/mux"
	redis2 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/amqp"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	protocolsmongo "github.com/DataDog/datadog-agent/pkg/network/protocols/mongo"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/mysql"
	pgutils "github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	tracertestutil "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	grpc2 "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

const (
	// Most of the classifications are only supported on Linux, hence, they are defined in a Linux specific file.
	amqpPort       = "5672"
	amqpsPort      = "5671"
	grpcPort       = "9091"
	http2Port      = "9090"
	httpsPort      = "8443"
	kafkaPort      = "9092"
	mongoPort      = "27017"
	mysqlPort      = "3306"
	postgresPort   = "5432"
	rawTrafficPort = "9093"
	redisPort      = "6379"

	fetchAPIKey   = 1
	produceAPIKey = 0

	produceMaxSupportedVersion = 8
	produceMinSupportedVersion = 1

	fetchMaxSupportedVersion = 12
	fetchMinSupportedVersion = 0
)

func httpSupported() bool {
	if ebpftest.GetBuildMode() == ebpftest.Fentry {
		return false
	}
	return kv >= usmconfig.MinimumKernelVersion
}

func httpsSupported() bool {
	if ebpftest.GetBuildMode() == ebpftest.Fentry {
		return false
	}
	return usmconfig.TLSSupported(tracertestutil.Config())
}

func classificationSupported(config *config.Config) bool {
	return kprobe.ClassificationSupported(config)
}

// skipIfUsingNAT skips the test if we have a NAT rules applied.
func skipIfUsingNAT(t *testing.T, ctx testContext) {
	if ctx.targetAddress != ctx.serverAddress {
		t.Skip("test is not supported when NAT is applied")
	}
}

// skipIfGoTLSNotSupported skips the test if GoTLS is not supported.
func skipIfGoTLSNotSupported(t *testing.T, _ testContext) {
	if !gotlstestutil.GoTLSSupported(t, config.New()) {
		t.Skip("GoTLS is not supported")
	}
}

// composeSkips skips if one of the given filters is matched.
func composeSkips(skippers ...func(t *testing.T, ctx testContext)) func(t *testing.T, ctx testContext) {
	return func(t *testing.T, ctx testContext) {
		for _, skipFunction := range skippers {
			skipFunction(t, ctx)
		}
	}
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

	cfg := tracertestutil.Config()
	cfg.EnableHTTPMonitoring = true
	_ = setupTracer(t, cfg)
}

func (s *USMSuite) TestDisableUSM() {
	t := s.T()

	cfg := tracertestutil.Config()
	cfg.ServiceMonitoringEnabled = false
	// Enabling all features, to ensure nothing is forcing USM enablement.
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableKafkaMonitoring = true
	cfg.EnablePostgresMonitoring = true
	cfg.EnableGoTLSSupport = true
	cfg.EnableNodeJSMonitoring = true
	cfg.EnableIstioMonitoring = true
	cfg.EnableNativeTLSMonitoring = true

	tr := setupTracer(t, cfg)
	require.Nil(t, tr.USMMonitor())
}

func (s *USMSuite) TestProtocolClassification() {
	t := s.T()
	cfg := tracertestutil.Config()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}

	cfg.ServiceMonitoringEnabled = true
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	cfg.EnablePostgresMonitoring = true
	cfg.EnableGoTLSSupport = gotlstestutil.GoTLSSupported(t, cfg)
	cfg.BypassEnabled = true
	tr, err := tracer.NewTracer(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	t.Run("with dnat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
		netlink.SetupDNAT(t)
		testProtocolClassificationCrossOS(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testProtocolClassificationLinux(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testTLSClassification(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "2.2.2.2", "1.1.1.1:0")
	})

	t.Run("with snat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 6.6.6.6 to 7.7.7.7
		netlink.SetupSNAT(t)
		testProtocolClassificationCrossOS(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testProtocolClassificationLinux(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testTLSClassification(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
	})

	t.Run("without nat", func(t *testing.T) {
		testProtocolClassificationCrossOS(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testProtocolClassificationLinux(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testTLSClassification(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}

func testProtocolConnectionProtocolMapCleanup(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Run("protocol cleanup", func(t *testing.T) {
		t.Cleanup(func() { tr.Pause() })

		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP:   net.ParseIP(clientHost),
				Port: 0,
			},
			Control: func(_, _ string, c syscall.RawConn) error {
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

		require.NoError(t, tr.RegisterClient(clientID))
		require.NoError(t, tr.Resume())

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

func (s *USMSuite) TestIgnoreTLSClassificationIfApplicationProtocolWasDetected() {
	t := s.T()
	cfg := tracertestutil.Config()
	cfg.ServiceMonitoringEnabled = true
	// USM cannot be enabled without a protocol.
	cfg.EnableHTTPMonitoring = true
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

	srv := testutil.NewTLSServerWithSpecificVersion("localhost:0", func(conn net.Conn) {
		defer conn.Close()
		// Echo back whatever is received
		_, err := io.Copy(conn, conn)
		if err != nil {
			fmt.Printf("Failed to echo data: %v\n", err)
			return
		}
	}, tls.VersionTLS12)
	done := make(chan struct{})
	require.NoError(t, srv.Run(done))
	t.Cleanup(func() { close(done) })
	_, srvPortStr, err := net.SplitHostPort(srv.Address())
	require.NoError(t, err)
	srvPort, err := strconv.Atoi(srvPortStr)
	require.NoError(t, err)
	srvPortU16 := uint16(srvPort)

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	tr := setupTracer(t, cfg)

	localhostAddress, err := netip.ParseAddr("127.0.0.1")
	require.NoError(t, err)
	addrLow, addrHigh := util.ToLowHighIP(localhostAddress)

	tests := []struct {
		name          string
		protocolValue uint8
		shouldBeTLS   bool
	}{
		{
			name:          "UNKNOWN",
			protocolValue: 0,
			shouldBeTLS:   true,
		},
		{
			name:          "HTTP",
			protocolValue: 1,
			shouldBeTLS:   false,
		},
		{
			name:          "HTTP2",
			protocolValue: 2,
			shouldBeTLS:   false,
		},
		{
			name:          "KAFKA",
			protocolValue: 3,
			shouldBeTLS:   false,
		},
		{
			name:          "MONGO",
			protocolValue: 4,
			shouldBeTLS:   false,
		},
		{
			name:          "POSTGRES",
			protocolValue: 5,
			shouldBeTLS:   true,
		},
		{
			name:          "AMQP",
			protocolValue: 6,
			shouldBeTLS:   false,
		},
		{
			name:          "REDIS",
			protocolValue: 7,
			shouldBeTLS:   false,
		},
		{
			name:          "MYSQL",
			protocolValue: 8,
			shouldBeTLS:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientPort, err := getFreePort()
			require.NoError(t, err)
			dialer := &net.Dialer{
				LocalAddr: &net.TCPAddr{
					IP:   net.ParseIP("localhost"),
					Port: int(clientPort),
				},
			}
			conn, err := dialer.Dial("tcp", srv.Address())
			require.NoError(t, err)
			defer conn.Close()
			tlsConn := tls.Client(conn, tlsConfig)

			connTupleKey := netebpf.ConnTuple{
				Saddr_h:  addrHigh,
				Saddr_l:  addrLow,
				Daddr_h:  addrHigh,
				Daddr_l:  addrLow,
				Sport:    clientPort,
				Dport:    srvPortU16,
				Metadata: uint32(netebpf.TCP),
			}
			protocolValue := netebpf.ProtocolStackWrapper{
				Stack: netebpf.ProtocolStack{
					Application: tt.protocolValue,
				},
			}
			tr.USMMonitor().SetConnectionProtocol(t, protocolValue, connTupleKey)
			connTupleKey.Sport, connTupleKey.Dport = connTupleKey.Dport, connTupleKey.Sport
			tr.USMMonitor().SetConnectionProtocol(t, protocolValue, connTupleKey)

			// Perform the TLS handshake
			require.NoError(t, tlsConn.Handshake())

			require.Eventually(t, func() bool {
				payload := getConnections(t, tr)
				for _, c := range payload.Conns {
					if c.DPort == srvPortU16 || c.SPort == srvPortU16 {
						return c.ProtocolStack.Contains(protocols.TLS) == tt.shouldBeTLS
					}
				}
				return false
			}, 10*time.Second, 100*time.Millisecond)
		})
	}
}

// TLS classification tests
func (s *USMSuite) TestTLSClassification() {
	t := s.T()
	cfg := tracertestutil.Config()
	cfg.ServiceMonitoringEnabled = true
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

	port, err := getFreePort()
	require.NoError(t, err)
	portAsString := strconv.Itoa(int(port))

	tr := setupTracer(t, cfg)

	type tlsTest struct {
		name            string
		postTracerSetup func(t *testing.T)
		validation      func(t *testing.T, tr *tracer.Tracer)
	}
	tests := make([]tlsTest, 0)
	for _, scenario := range []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13} {
		scenario := scenario
		tests = append(tests, tlsTest{
			name: strings.Replace(tls.VersionName(scenario), " ", "-", 1) + "_docker",
			postTracerSetup: func(t *testing.T) {
				srv := testutil.NewTLSServerWithSpecificVersion("localhost:"+portAsString, func(conn net.Conn) {
					defer conn.Close()
					// Echo back whatever is received
					_, err := io.Copy(conn, conn)
					if err != nil {
						fmt.Printf("Failed to echo data: %v\n", err)
						return
					}
				}, scenario)
				done := make(chan struct{})
				require.NoError(t, srv.Run(done))
				t.Cleanup(func() { close(done) })
				tlsConfig := &tls.Config{
					MinVersion:         scenario,
					MaxVersion:         scenario,
					InsecureSkipVerify: true,
				}
				conn, err := net.Dial("tcp", "localhost:"+portAsString)
				require.NoError(t, err)
				defer conn.Close()

				// Wrap the TCP connection with TLS
				tlsConn := tls.Client(conn, tlsConfig)

				// Perform the TLS handshake
				require.NoError(t, tlsConn.Handshake())
			},
			validation: func(t *testing.T, tr *tracer.Tracer) {
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
			if ebpftest.GetBuildMode() == ebpftest.Fentry {
				t.Skip("protocol classification not supported for fentry tracer")
			}
			t.Cleanup(func() { tr.RemoveClient(clientID) })
			t.Cleanup(func() { _ = tr.Pause() })

			tr.RemoveClient(clientID)
			require.NoError(t, tr.RegisterClient(clientID))
			require.NoError(t, tr.Resume(), "enable probes - before post tracer")
			tt.postTracerSetup(t)
			require.NoError(t, tr.Pause(), "disable probes - after post tracer")
			tt.validation(t, tr)
		})
	}
}

func (s *USMSuite) TestTLSClassificationAlreadyRunning() {
	t := s.T()

	cfg := tracertestutil.Config()
	cfg.ProtocolClassificationEnabled = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

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
	tr := setupTracer(t, cfg)
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
}

func skipIfHTTPSNotSupported(t *testing.T, _ testContext) {
	if !httpsSupported() {
		t.Skip("https is not supported")
	}
}

func testTLSClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Run("TLS", func(t *testing.T) {
		tests := []struct {
			name string
			fn   func(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string)
		}{
			{"HTTP", testHTTPSClassification},
			{"amqp", testTLSAMQPProtocolClassification},
			{"mysql", testMySQLProtocolClassificationTLS},
			{"postgres", testPostgresProtocolClassificationWrapper(protocolsUtils.TLSEnabled)},
			{"redis", testTLSRedisProtocolClassification},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.fn(t, tr, clientHost, targetHost, serverHost)
			})
		}
	})
}

func testHTTPSClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Skip("Flaky test")
	skipFunc := composeSkips(skipIfHTTPSNotSupported, skipIfGoTLSNotSupported)
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

	// makeRequest is a helper that makes a GET request and handle the response.
	makeRequest := func(t require.TestingT, client *nethttp.Client, url string) {
		r, err := client.Get(url)
		assert.NoError(t, err)
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		client.CloseIdleConnections()
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
				cmd := gotlstestutil.NewGoTLSServer(t, ctx.serverAddress)
				ctx.extras["cmd"] = cmd
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				cmd := ctx.extras["cmd"].(*exec.Cmd)
				goTLSAttachPID(t, cmd.Process.Pid)

				client := &nethttp.Client{
					Transport: &nethttp.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						DialContext:     defaultDialer.DialContext,
					},
				}

				requestURL := fmt.Sprintf("https://%s/200/request", ctx.targetAddress)

				// The server might not be ready to accept connection just yet, so we
				// try until it starts accepting them.
				require.EventuallyWithT(t, func(c *assert.CollectT) {
					makeRequest(c, client, requestURL)
				}, 5*time.Second, 100*time.Millisecond)
			},
			validation: func(t *testing.T, ctx testContext, tr *tracer.Tracer) {
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

	tr, err := tracer.NewTracer(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	require.NoError(t, tr.RegisterClient(clientID))
}

func testKafkaProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	const topicName = "franz-kafka"
	testIndex := 0
	// Kafka does not allow us to delete topic, but to mark them for deletion, so we have to generate a unique topic
	// per a test.
	getTopicName := func() string {
		testIndex++
		return fmt.Sprintf("%s-%d", topicName, testIndex)
	}

	skipFunc := composeSkips(skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    kafkaPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	kafkaTeardown := func(_ *testing.T, ctx testContext) {
		for key, val := range ctx.extras {
			if strings.HasSuffix(key, "client") {
				client := val.(*kafka.Client)
				client.Client.Close()
			}
		}
	}

	serverAddress := net.JoinHostPort(serverHost, kafkaPort)
	targetAddress := net.JoinHostPort(targetHost, kafkaPort)
	require.NoError(t, kafka.RunServer(t, serverHost, kafkaPort))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
			teardown:   kafkaTeardown,
		},
		{
			name: "create topic",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
			teardown:   kafkaTeardown,
		},
		{
			name: "produce - empty string client id",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Kafka}),
			teardown:   kafkaTeardown,
		},
		{
			name: "produce - multiple topics",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name1": getTopicName(),
					"topic_name2": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name1"].(string)))
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name2"].(string)))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				record1 := &kgo.Record{Topic: ctx.extras["topic_name1"].(string), Value: []byte("Hello Kafka!")}
				record2 := &kgo.Record{Topic: ctx.extras["topic_name2"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1, record2).FirstErr(), "record had a produce error while synchronously producing")
			},
			validation: validateProtocolConnection(&protocols.Stack{
				Application: protocols.Kafka,
			}),
			teardown: kafkaTeardown,
		},
	}

	versions := []struct {
		produceVersion int16
		fetchVersion   int16
	}{
		{
			produceVersion: 0,
			fetchVersion:   0,
		},
		{
			produceVersion: 1,
			fetchVersion:   1,
		},
		{
			produceVersion: 2,
			fetchVersion:   2,
		},
		{
			produceVersion: 3,
			fetchVersion:   3,
		},
		{
			produceVersion: 4,
			fetchVersion:   4,
		},
		{
			produceVersion: 5,
			fetchVersion:   5,
		},
		{
			produceVersion: 6,
			fetchVersion:   6,
		},
		{
			produceVersion: 7,
			fetchVersion:   7,
		},
		{
			produceVersion: 8,
			fetchVersion:   8,
		},
		{
			produceVersion: 9,
			fetchVersion:   9,
		},
		{
			produceVersion: 9,
			fetchVersion:   10,
		},
		{
			produceVersion: 9,
			fetchVersion:   11,
		},
		{
			produceVersion: 9,
			fetchVersion:   12,
		},
		{
			produceVersion: 9,
			fetchVersion:   13,
		},
	}
	for _, pair := range versions {
		produceExpectedStack := &protocols.Stack{Application: protocols.Kafka}
		fetchExpectedStack := &protocols.Stack{Application: protocols.Kafka}

		if pair.produceVersion < produceMinSupportedVersion || pair.produceVersion > produceMaxSupportedVersion {
			produceExpectedStack.Application = protocols.Unknown
		}
		if pair.fetchVersion < fetchMinSupportedVersion || pair.fetchVersion > fetchMaxSupportedVersion {
			fetchExpectedStack.Application = protocols.Unknown
		}

		version := kversion.V3_4_0()
		version.SetMaxKeyVersion(produceAPIKey, pair.produceVersion)
		version.SetMaxKeyVersion(fetchAPIKey, pair.fetchVersion)

		tests = append(tests, protocolClassificationAttributes{
			name: fmt.Sprintf("fetch (v%d); produce (v%d)", pair.fetchVersion, pair.produceVersion),
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				produceClient, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(version), kgo.ConsumeTopics(ctx.extras["topic_name"].(string))},
				})
				require.NoError(t, err)
				fetchClient, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(version), kgo.ConsumeTopics(ctx.extras["topic_name"].(string))},
				})
				require.NoError(t, err)
				ctx.extras["produce_client"] = produceClient
				ctx.extras["fetch_client"] = fetchClient
				require.NoError(t, produceClient.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				produceClient := ctx.extras["produce_client"].(*kafka.Client)
				record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				require.NoError(t, produceClient.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
				cancel()

				validateProtocolConnection(produceExpectedStack)
				tr.RemoveClient(clientID)
				require.NoError(t, tr.RegisterClient(clientID))
				fetchClient := ctx.extras["fetch_client"].(*kafka.Client)
				fetches := fetchClient.Client.PollFetches(context.Background())
				require.Empty(t, fetches.Errors())
				records := fetches.Records()
				require.Len(t, records, 1)
				require.Equal(t, ctx.extras["topic_name"].(string), records[0].Topic)
			},
			validation: validateProtocolConnection(fetchExpectedStack),
			teardown:   kafkaTeardown,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testMySQLProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	testMySQLProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSDisabled)
}

func testMySQLProtocolClassificationTLS(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Skip("MySQL+TLS classification is flaky")
	testMySQLProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSEnabled)
}

func testMySQLProtocolClassificationInner(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string, withTLS bool) {
	skipFuncs := []func(*testing.T, testContext){
		skipIfUsingNAT,
	}
	if withTLS {
		skipFuncs = append(skipFuncs, skipIfGoTLSNotSupported)
	}
	composeSkips(skipFuncs...)(t, testContext{
		serverAddress: serverHost,
		serverPort:    mysqlPort,
		targetAddress: targetHost,
	})

	expectedStack := &protocols.Stack{Application: protocols.MySQL}
	if withTLS {
		expectedStack.Encryption = protocols.TLS

		// Our client runs in this binary. By default, USM will exclude the current process from tracing. But,
		// we need to include it in this case. So we allowing it by setting GoTLSExcludeSelf to false and resetting it
		// after the test.
		pid := os.Getpid()
		require.NoError(t, usm.SetGoTLSExcludeSelf(false))
		goTLSAttachPID(t, pid)
		t.Cleanup(func() {
			goTLSDetachPID(t, pid)
			require.NoError(t, usm.SetGoTLSExcludeSelf(true))
		})
	}

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	mysqlTeardown := func(_ *testing.T, ctx testContext) {
		if client, ok := ctx.extras["conn"].(*mysql.Client); ok {
			defer client.DB.Close()
			client.DropDB()
		}
	}

	serverAddress := net.JoinHostPort(serverHost, mysqlPort)
	targetAddress := net.JoinHostPort(targetHost, mysqlPort)
	require.NoError(t, mysql.RunServer(t, serverHost, mysqlPort, withTLS))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
			},
			// We classify on MySQL's Server Greeting messages,
			// which are sent in plaintext, before a TLS handshake
			// could occur.
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.MySQL}),
			teardown:   mysqlTeardown,
		},
		{
			name: "create db",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.CreateDB())
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "create table",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.CreateTable())
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "insert",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "delete",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.DeleteFromTable("Bratislava"))
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "select",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				population, err := c.SelectFromTable("Bratislava")
				require.NoError(t, err)
				require.Equal(t, 432000, population)
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "update",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.UpdateTable("Bratislava", "Bratislava2", 10))
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "drop table",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.DropTable())
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			name: "alter",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.AlterTable())
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long query",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.InsertIntoTable(strings.Repeat("#", 16384), 10))
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long response",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					WithTLS:       withTLS,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				name := strings.Repeat("#", 1024)
				for i := int64(1); i <= 40; i++ {
					require.NoError(t, c.InsertIntoTable(name+"i", 10))
				}
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.SelectAllFromTable())
			},
			validation: validateProtocolConnection(expectedStack),
			teardown:   mysqlTeardown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

// waitForPostgresServer verifies that the postgres server is up and running.
// It tries to connect to the server until it succeeds or the timeout is reached.
// We need that function (and cannot relay on the RunServer method) as the target regex is being logged a couple os
// milliseconds before the server is actually ready to accept connections.
func waitForPostgresServer(t *testing.T, serverAddress string, enableTLS bool) {
	pgClient := pgutils.NewPGClient(pgutils.ConnectionOptions{
		ServerAddress: serverAddress,
		EnableTLS:     enableTLS,
	})
	defer pgClient.Close()
	require.Eventually(t, func() bool {
		return pgClient.Ping() == nil
	}, 5*time.Second, 100*time.Millisecond, "couldn't connect to postgres server")
}

func testPostgresProtocolClassificationWrapper(enableTLS bool) func(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	return func(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
		testPostgresProtocolClassification(t, tr, clientHost, targetHost, serverHost, enableTLS)
	}
}

func testPostgresProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string, enableTLS bool) {
	skippers := []func(t *testing.T, ctx testContext){skipIfUsingNAT}
	if enableTLS {
		t.Skip("TLS+Postgres classification tests are flaky")
		skippers = append(skippers, skipIfGoTLSNotSupported)
	}
	skipFunc := composeSkips(skippers...)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    postgresPort,
		targetAddress: targetHost,
	})

	if clientHost != "127.0.0.1" && clientHost != "localhost" {
		t.Skip("postgres tests are not supported DNat")
	}

	// Setting one instance of postgres server for all tests.
	serverAddress := net.JoinHostPort(serverHost, postgresPort)
	targetAddress := net.JoinHostPort(targetHost, postgresPort)
	require.NoError(t, pgutils.RunServer(t, serverHost, postgresPort, enableTLS))
	// Verifies that the postgres server is up and running.
	// It tries to connect to the server until it succeeds or the timeout is reached.
	// We need that function (and cannot relay on the RunServer method) as the target regex is being logged a couple os
	// milliseconds before the server is actually ready to accept connections.
	waitForPostgresServer(t, serverAddress, enableTLS)

	expectedProtocolStack := &protocols.Stack{Application: protocols.Postgres}
	if enableTLS {
		expectedProtocolStack.Encryption = protocols.TLS
		// Our client runs in this binary. By default, USM will exclude the current process from tracing. But,
		// we need to include it in this case. So we allowing it by setting GoTLSExcludeSelf to false and resetting it
		// after the test.
		pid := os.Getpid()
		require.NoError(t, usm.SetGoTLSExcludeSelf(false))
		goTLSAttachPID(t, pid)
		t.Cleanup(func() {
			goTLSDetachPID(t, pid)
			require.NoError(t, usm.SetGoTLSExcludeSelf(true))
		})
	}

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				defer pg.Close()
				// Ping is not supported by the classification, but we need to trigger a connection handshake between
				// the client and the server to classify the connection. So ping is a reasonable choice.
				require.NoError(t, pg.Ping())
			},
		},
		{
			name: "insert",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunInsertQuery(1))
			},
		},
		{
			name: "delete",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
				require.NoError(t, pg.RunInsertQuery(1))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunDeleteQuery())
			},
		},
		{
			name: "select",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunSelectQuery())
			},
		},
		{
			name: "update",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
				require.NoError(t, pg.RunInsertQuery(1))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunUpdateQuery())
			},
		},
		{
			name: "drop",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
				require.NoError(t, pg.RunInsertQuery(1))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunDropQuery())
			},
		},
		{
			name: "alter",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunAlterQuery())
			},
		},
		{
			name: "truncate",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
				require.NoError(t, pg.RunInsertQuery(1))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunTruncateQuery())
			},
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long query",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)

				// This will fail but it should make a query and be classified
				require.NoError(t, pg.RunMultiInsertQuery(strings.Repeat("#", 16384)))
			},
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long response",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.NewPGClient(pgutils.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     enableTLS,
				})
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunCreateQuery())
				for i := int64(1); i < 200; i++ {
					require.NoError(t, pg.RunInsertQuery(i))
				}
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := ctx.extras["pg"].(*pgutils.PGClient)
				require.NoError(t, pg.RunSelectQuery())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validation = validateProtocolConnection(expectedProtocolStack)
			tt.teardown = func(_ *testing.T, ctx testContext) {
				if pg, ok := ctx.extras["pg"].(*pgutils.PGClient); ok {
					defer pg.Close()
					_ = pg.RunDropQuery()
				}
			}
			tt.context = testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			}
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testMongoProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    mongoPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	mongoTeardown := func(_ *testing.T, ctx testContext) {
		if client, ok := ctx.extras["client"].(*protocolsmongo.Client); ok {
			require.NoError(t, client.DeleteDatabases())
			defer client.Stop()
		}
	}

	// Setting one instance of mongo server for all tests.
	serverAddress := net.JoinHostPort(serverHost, mongoPort)
	targetAddress := net.JoinHostPort(targetHost, mongoPort)
	require.NoError(t, protocolsmongo.RunServer(t, serverHost, mongoPort))

	tests := []protocolClassificationAttributes{
		{
			name: "classify by connect",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				client.Stop()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Mongo}),
		},
		{
			name: "classify by collection creation",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*protocolsmongo.Client)
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Mongo}),
			teardown:   mongoTeardown,
		},
		{
			name: "classify by insertion",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
				ctx.extras["collection"] = db.Collection("collection")
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				collection := ctx.extras["collection"].(*mongo.Collection)
				input := map[string]string{"test": "test"}
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_, err := collection.InsertOne(timedContext, input)
				require.NoError(t, err)
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Mongo}),
			teardown:   mongoTeardown,
		},
		{
			name: "classify by find",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
				cancel()
				collection := db.Collection("collection")
				ctx.extras["input"] = map[string]string{"test": "test"}
				timedContext, cancel = context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_, err = collection.InsertOne(timedContext, ctx.extras["input"])
				require.NoError(t, err)

				ctx.extras["collection"] = db.Collection("collection")
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				collection := ctx.extras["collection"].(*mongo.Collection)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := collection.FindOne(timedContext, bson.M{"test": "test"})
				require.NoError(t, res.Err())
				var output map[string]string
				require.NoError(t, res.Decode(&output))
				delete(output, "_id")
				require.EqualValues(t, output, ctx.extras["input"])
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Mongo}),
			teardown:   mongoTeardown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testRedisProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	testRedisProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSDisabled)
}

func testTLSRedisProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Skip("TLS+Redis classification tests are flaky")
	testRedisProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSEnabled)
}

func testRedisProtocolClassificationInner(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string, withTLS bool) {
	skipFuncs := []func(*testing.T, testContext){
		skipIfUsingNAT,
	}
	if withTLS {
		skipFuncs = append(skipFuncs, skipIfGoTLSNotSupported)
	}

	composeSkips(skipFuncs...)(t, testContext{
		serverAddress: serverHost,
		serverPort:    redisPort,
		targetAddress: targetHost,
	})

	expectedStack := &protocols.Stack{Application: protocols.Redis}
	if withTLS {
		expectedStack.Encryption = protocols.TLS

		// Our client runs in this binary. By default, USM will exclude the current process from tracing. But,
		// we need to include it in this case. So we allowing it by setting GoTLSExcludeSelf to false and resetting it
		// after the test.
		pid := os.Getpid()
		require.NoError(t, usm.SetGoTLSExcludeSelf(false))
		goTLSAttachPID(t, pid)
		t.Cleanup(func() {
			goTLSDetachPID(t, pid)
			require.NoError(t, usm.SetGoTLSExcludeSelf(true))
		})
	}

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	redisTeardown := func(_ *testing.T, ctx testContext) {
		redis.NewClient(ctx.serverAddress, defaultDialer, withTLS)
		if client, ok := ctx.extras["client"].(*redis2.Client); ok {
			timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()
			require.NoError(t, client.FlushDB(timedContext).Err())
		}
	}

	// Setting one instance of redis server for all tests.
	serverAddress := net.JoinHostPort(serverHost, redisPort)
	targetAddress := net.JoinHostPort(targetHost, redisPort)
	require.NoError(t, redis.RunServer(t, serverHost, redisPort, withTLS))

	tests := []protocolClassificationAttributes{
		{
			name: "set",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(_ *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer, withTLS)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(_ *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Set(timedContext, "key", "value", time.Minute)
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(expectedStack),
		},
		{
			name: "get",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(_ *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer, withTLS)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Set(timedContext, "key", "value", time.Minute)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.Get(timedContext, "key")
				val, err := res.Result()
				require.NoError(t, err)
				require.Equal(t, "value", val)
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(expectedStack),
		},
		{
			name: "get unknown key",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(_ *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer, withTLS)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.Get(timedContext, "unknown")
				require.Error(t, res.Err())
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(expectedStack),
		},
		{
			name: "err response",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				conn, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				_, err = conn.Write([]byte("+dummy\r\n"))
				require.NoError(t, err)
			},
			validation: validateProtocolConnection(expectedStack),
		},
		{
			name: "client id",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(_ *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer, withTLS)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.ClientID(timedContext)
				require.NoError(t, res.Err())
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(expectedStack),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testTLSAMQPProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	t.Skip("TLS+AMQP classification tests are flaky")
	testAMQPProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSEnabled)
}

func testAMQPProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	testAMQPProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSDisabled)
}

type amqpTestSpec struct {
	port               string
	classifiedStack    *protocols.Stack
	nonClassifiedStack *protocols.Stack
	skipFuncs          []func(*testing.T, testContext)
}

var amqpTestSpecsMap = map[bool]amqpTestSpec{
	protocolsUtils.TLSDisabled: {
		port:               amqpPort,
		classifiedStack:    &protocols.Stack{Application: protocols.AMQP},
		nonClassifiedStack: &protocols.Stack{},
		skipFuncs: []func(*testing.T, testContext){
			skipIfUsingNAT,
		},
	},
	protocolsUtils.TLSEnabled: {
		port:               amqpsPort,
		classifiedStack:    &protocols.Stack{Encryption: protocols.TLS, Application: protocols.AMQP},
		nonClassifiedStack: &protocols.Stack{Encryption: protocols.TLS},
		skipFuncs: []func(*testing.T, testContext){
			skipIfUsingNAT,
			skipIfGoTLSNotSupported,
		},
	},
}

func testAMQPProtocolClassificationInner(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string, withTLS bool) {
	spec := amqpTestSpecsMap[withTLS]
	composeSkips(spec.skipFuncs...)(t, testContext{
		serverAddress: serverHost,
		serverPort:    spec.port,
		targetAddress: targetHost,
	})

	getAMQPClientOpts := func(ctx testContext) amqp.Options {
		// We return options for both TLS and Plaintext. Our
		// AMQP client wrapper will only uses the ones it needs.
		return amqp.Options{
			ServerAddress: ctx.serverAddress,
			WithTLS:       withTLS,
			Dialer: &net.Dialer{
				LocalAddr: &net.TCPAddr{
					IP: net.ParseIP(clientHost),
				},
			},
		}
	}

	amqpTeardown := func(_ *testing.T, ctx testContext) {
		if client, ok := ctx.extras["client"].(*amqp.Client); ok {
			defer client.Terminate()
			require.NoError(t, client.DeleteQueues())
		}
	}

	if withTLS {
		// Our client runs in this binary. By default, USM will exclude the current process from tracing. But,
		// we need to include it in this case. So we allowing it by setting GoTLSExcludeSelf to false and resetting it
		// after the test.
		pid := os.Getpid()
		require.NoError(t, usm.SetGoTLSExcludeSelf(false))
		goTLSAttachPID(t, pid)
		t.Cleanup(func() {
			goTLSDetachPID(t, pid)
			require.NoError(t, usm.SetGoTLSExcludeSelf(true))
		})
	}

	// Setting one instance of amqp server for all tests.
	serverAddress := net.JoinHostPort(serverHost, spec.port)
	targetAddress := net.JoinHostPort(targetHost, spec.port)
	require.NoError(t, amqp.RunServer(t, serverHost, spec.port, withTLS))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    spec.port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(getAMQPClientOpts(ctx))
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(spec.classifiedStack),
		},
		{
			name: "declare channel",
			context: testContext{
				serverPort:    spec.port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(getAMQPClientOpts(ctx))
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(spec.nonClassifiedStack),
		},
		{
			name: "publish",
			context: testContext{
				serverPort:    spec.port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(getAMQPClientOpts(ctx))
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.Publish("test", "my msg"))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(spec.classifiedStack),
		},
		{
			name: "consume",
			context: testContext{
				serverPort:    spec.port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(getAMQPClientOpts(ctx))
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				require.NoError(t, client.DeclareQueue("test", client.ConsumeChannel))
				require.NoError(t, client.Publish("test", "my msg"))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				res, err := client.Consume("test", 1)
				require.NoError(t, err)
				require.Equal(t, []string{"my msg"}, res)
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(spec.classifiedStack),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testHTTP2ProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP(clientHost),
			Port: 0,
		},
	}

	// http2 server init
	http2ServerAddress := net.JoinHostPort(serverHost, http2Port)
	http2TargetAddress := net.JoinHostPort(targetHost, http2Port)
	http2Server := &nethttp.Server{
		Addr: ":" + http2Port,
		Handler: h2c.NewHandler(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
			w.WriteHeader(200)
			w.Write([]byte("test"))
		}), &http2.Server{}),
	}

	go func() {
		if err := http2Server.ListenAndServe(); err != nethttp.ErrServerClosed {
			require.NoError(t, err, "could not serve")
		}
	}()
	t.Cleanup(func() {
		http2Server.Close()
	})

	// gRPC server init
	grpcServerAddress := net.JoinHostPort(serverHost, grpcPort)
	grpcTargetAddress := net.JoinHostPort(targetHost, grpcPort)

	grpcServer, err := grpc.NewServer(grpcServerAddress, false)
	require.NoError(t, err)
	grpcServer.Run()
	t.Cleanup(grpcServer.Stop)

	grpcContext := testContext{
		serverPort:    grpcPort,
		serverAddress: grpcServerAddress,
		targetAddress: grpcTargetAddress,
	}

	tests := []protocolClassificationAttributes{
		{
			name: "http2 traffic without grpc",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: http2ServerAddress,
				targetAddress: http2TargetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := &nethttp.Client{
					Transport: &http2.Transport{
						AllowHTTP: true,
						DialTLSContext: func(_ context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
							return net.Dial(network, addr)
						},
					},
				}

				resp, err := client.Post("http://"+ctx.targetAddress, "application/json", bytes.NewReader([]byte("test")))
				require.NoError(t, err)

				resp.Body.Close()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2}),
		},
		{
			name:    "http2 traffic using gRPC - unary call",
			context: grpcContext,
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				}, false)
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleUnary(timedContext, "test"))
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			name:    "http2 traffic using gRPC - stream call",
			context: grpcContext,
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				}, false)
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleStream(timedContext, 5))
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			// This test checks if the classifier can properly skip literal
			// headers that are not useful to determine if gRPC is used.
			name: "http2 traffic using gRPC - irrelevant literal headers",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: http2ServerAddress,
				targetAddress: http2TargetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := &nethttp.Client{
					Transport: &http2.Transport{
						AllowHTTP: true,
						DialTLSContext: func(_ context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
							return net.Dial(network, addr)
						},
					},
				}

				req, err := nethttp.NewRequest("POST", "http://"+ctx.targetAddress, bytes.NewReader([]byte("test")))
				require.NoError(t, err)

				// Add some literal headers that needs to be skipped by the
				// classifier. Also adding a grpc content-type to emulate grpc
				// traffic
				req.Header.Add("someheader", "somevalue")
				req.Header.Add("Content-type", "application/grpc")
				req.Header.Add("someotherheader", "someothervalue")

				resp, err := client.Do(req)
				require.NoError(t, err)

				resp.Body.Close()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			// This test checks that we are not classifying a connection as
			// gRPC traffic without a prior classification as HTTP2.
			name: "GRPC without prior HTTP2 classification",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				targetAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := tracertestutil.NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					io.Copy(c, c)
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// The gRPC classification is based on having only POST requests,
				// and having "application/grpc" as a content-type.
				var testHeaderFields = []hpack.HeaderField{
					{Name: ":authority", Value: "127.0.0.0.1:" + rawTrafficPort},
					{Name: ":method", Value: "POST"},
					{Name: ":path", Value: "/aaa"},
					{Name: ":scheme", Value: "http"},
					{Name: "content-type", Value: "application/grpc"},
					{Name: "content-length", Value: "0"},
					{Name: "accept-encoding", Value: "gzip"},
					{Name: "user-agent", Value: "Go-http-client/2.0"},
				}

				buf := new(bytes.Buffer)
				framer := http2.NewFramer(buf, nil)
				rawHdrs, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: testHeaderFields})
				require.NoError(t, err)

				// Writing the header frames to the buffer using the Framer.
				require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      uint32(1),
					BlockFragment: rawHdrs,
					EndStream:     true,
					EndHeaders:    true,
				}))

				c, err := net.Dial("tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				_, err = c.Write(buf.Bytes())
				require.NoError(t, err)
			},
			teardown: func(_ *testing.T, ctx testContext) {
				if srv, ok := ctx.extras["server"].(*tracertestutil.TCPServer); ok {
					srv.Shutdown()
				}
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
		},
		{
			name: "GRPC with prior HTTP2 classification",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				targetAddress: net.JoinHostPort(targetHost, rawTrafficPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := tracertestutil.NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					io.Copy(c, c)
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// The gRPC classification is based on having only POST requests,
				// and having "application/grpc" as a content-type.
				var testHeaderFields = []hpack.HeaderField{
					{Name: ":authority", Value: "127.0.0.0.1:" + rawTrafficPort},
					{Name: ":method", Value: "POST"},
					{Name: ":path", Value: "/aaa"},
					{Name: ":scheme", Value: "http"},
					{Name: "content-type", Value: "application/grpc"},
					{Name: "content-length", Value: "0"},
					{Name: "accept-encoding", Value: "gzip"},
					{Name: "user-agent", Value: "Go-http-client/2.0"},
				}

				// Initiate a connection to the TCP server.
				c, err := net.Dial("tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()

				// Writing a magic and the settings in the same packet to socket.
				_, err = c.Write([]byte(http2.ClientPreface))
				require.NoError(t, err)
				n, err := c.Read(make([]byte, len(http2.ClientPreface)))
				require.NoError(t, err)
				require.Equal(t, len(http2.ClientPreface), n)

				rawHdrs, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: testHeaderFields})
				require.NoError(t, err)

				buf := new(bytes.Buffer)
				framer := http2.NewFramer(buf, nil)
				// Writing the header frames to the buffer using the Framer.
				require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      uint32(1),
					BlockFragment: rawHdrs,
					EndStream:     true,
					EndHeaders:    true,
				}))

				_, err = c.Write(buf.Bytes())
				require.NoError(t, err)
				n, err = c.Read(make([]byte, buf.Len()))
				require.NoError(t, err)
				require.Equal(t, buf.Len(), n)
			},
			teardown: func(_ *testing.T, ctx testContext) {
				if srv, ok := ctx.extras["server"].(*tracertestutil.TCPServer); ok {
					srv.Shutdown()
				}
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testProtocolClassificationLinux(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string)
	}{
		{
			name:     "kafka",
			testFunc: testKafkaProtocolClassification,
		},
		{
			name:     "mysql",
			testFunc: testMySQLProtocolClassification,
		},
		{
			name:     "postgres",
			testFunc: testPostgresProtocolClassificationWrapper(protocolsUtils.TLSDisabled),
		},
		{
			name:     "mongo",
			testFunc: testMongoProtocolClassification,
		},
		{
			name:     "redis",
			testFunc: testRedisProtocolClassification,
		},
		{
			name:     "amqp",
			testFunc: testAMQPProtocolClassification,
		},
		{
			name:     "http2",
			testFunc: testHTTP2ProtocolClassification,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, tr, clientHost, targetHost, serverHost)
		})
	}
}

// goTLSAttachPID attaches the Go-TLS monitoring to the given PID.
// Wraps the call to the Go-TLS attach function and waits for the program to be traced.
func goTLSAttachPID(t *testing.T, pid int) {
	t.Helper()
	if utils.IsProgramTraced(usm.GoTLSAttacherName, pid) {
		return
	}
	require.NoError(t, usm.GoTLSAttachPID(uint32(pid)))
	utils.WaitForProgramsToBeTraced(t, usm.GoTLSAttacherName, pid, utils.ManualTracingFallbackEnabled)
}

// goTLSDetachPID detaches the Go-TLS monitoring from the given PID.
// Wraps the call to the Go-TLS detach function and waits for the program to be untraced.
func goTLSDetachPID(t *testing.T, pid int) {
	t.Helper()

	// The program is not traced; nothing to do.
	if !utils.IsProgramTraced(usm.GoTLSAttacherName, pid) {
		return
	}

	require.NoError(t, usm.GoTLSDetachPID(uint32(pid)))

	require.Eventually(t, func() bool {
		return !utils.IsProgramTraced(usm.GoTLSAttacherName, pid)
	}, 5*time.Second, 100*time.Millisecond, "process %v is still traced by Go-TLS after detaching", pid)
}
