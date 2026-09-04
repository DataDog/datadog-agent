// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	utiltest "github.com/DataDog/datadog-agent/pkg/util/testutil"

	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// ErrNotInstalled is returned when the trace-agent can not be found in $PATH.
var ErrNotInstalled = errors.New("agent: trace-agent not found in $PATH")

// SecretBackendBinary secret binary name
var SecretBackendBinary = "secret-script.test"

var (
	tmpDir    string
	buildOnce sync.Once
)

// grpcServer stands in for the core agent: a spawned trace-agent will not finish starting
// up until it has registered and seeded its config from one.
type grpcServer struct {
	pb.UnimplementedAgentSecureServer

	mu        sync.Mutex
	sessionID string
	settings  []*pb.ConfigSetting
}

func (g *grpcServer) setSettings(settings []*pb.ConfigSetting) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.settings = settings
}

func (g *grpcServer) RegisterRemoteAgent(_ context.Context, _ *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.sessionID = "trace-agent-test-session"
	return &pb.RegisterRemoteAgentResponse{
		SessionId:                      g.sessionID,
		RecommendedRefreshIntervalSecs: 60,
	}, nil
}

func (g *grpcServer) RefreshRemoteAgent(_ context.Context, req *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if req.SessionId != g.sessionID {
		return nil, status.Errorf(codes.NotFound, "session_id %q not found", req.SessionId)
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
}

// StreamConfigEvents sends the runner's config as a snapshot. A remote agent's configuration
// is the stream, so anything omitted here is unset for it.
func (g *grpcServer) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	g.mu.Lock()
	settings := g.settings
	g.mu.Unlock()

	if err := stream.Send(&pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{SequenceId: 1, Settings: settings},
		},
	}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

type agentRunner struct {
	mu  sync.RWMutex // guards pid
	pid int          // agent pid, if running

	port                 int         // agent receiver port
	log                  *safeBuffer // agent log output
	ddAddr               string      // Datadog intake address (host:port)
	bindir               string      // the temporary directory where the trace-agent binary is located
	verbose              bool
	agentServer          *grpc.Server
	coreAgent            *grpcServer
	agentServerListerner net.Listener
	authToken            string
	ipcCertPEM           []byte // cert+key PEM, as the agent's ipc_cert_file_path expects
}

// CleanupCachedBinaries removes the temporary directory created for cached binaries.
func CleanupCachedBinaries() {
	if tmpDir != "" {
		_, tmpDir = os.RemoveAll(tmpDir), ""
	}
}

func buildBinaries(verbose bool) error {
	var err error
	tmpDir, err = os.MkdirTemp("", "trace-agent-integration-tests")
	if err != nil {
		return err
	}
	binpath := filepath.Join(tmpDir, "trace-agent")
	if verbose {
		log.Printf("agent: installing in %s...", binpath)
	}
	cmd := utiltest.IsolatedGoBuildCmd(tmpDir, binpath, "-tags", "otlp", "github.com/DataDog/datadog-agent/cmd/trace-agent")
	o, err := cmd.CombinedOutput()
	if err != nil {
		if verbose {
			log.Printf("error installing trace-agent: %v", err)
			log.Print(string(o))
		}
		return ErrNotInstalled
	}

	binSecrets := filepath.Join(tmpDir, SecretBackendBinary)
	cmd = utiltest.IsolatedGoBuildCmd(tmpDir, binSecrets, "./testdata/secretscript.go")
	o, err = cmd.CombinedOutput()
	if err != nil {
		if verbose {
			log.Printf("error installing secret-script: %v", err)
			log.Print(string(o))
		}
		return ErrNotInstalled
	}

	if err := os.Chmod(binSecrets, 0700); err != nil {
		if verbose {
			log.Printf("error changing permissions secret-script: %v", err)
		}
		return ErrNotInstalled
	}
	return nil
}

func newAgentRunner(ddAddr string, verbose bool, buildSecretBackend bool) (*agentRunner, error) {
	var err error
	buildOnce.Do(func() {
		err = buildBinaries(verbose)
	})
	if err != nil {
		return nil, err
	}
	bindir, err := os.MkdirTemp(tmpDir, "runner-")
	if err != nil {
		return nil, err
	}
	binpath := filepath.Join(bindir, "trace-agent")
	if verbose {
		log.Printf("agent: installing in %s...", binpath)
	}
	if err := os.Symlink(filepath.Join(tmpDir, "trace-agent"), binpath); err != nil {
		if verbose {
			log.Printf("error installing trace-agent: %v", err)
		}
		return nil, ErrNotInstalled
	}

	if buildSecretBackend {
		binSecrets := filepath.Join(bindir, SecretBackendBinary)
		if err := os.Symlink(filepath.Join(tmpDir, SecretBackendBinary), binSecrets); err != nil {
			if verbose {
				log.Printf("error installing secret-script: %v", err)
			}
			return nil, ErrNotInstalled
		}
	}

	tlsKeyPair, ipcCertPEM, err := buildSelfSignedTLSCertificate("127.0.0.1")
	if err != nil {
		return nil, fmt.Errorf("unable to generate TLS certificate: %v", err)
	}

	// Generate an authentication token and set up our gRPC server to both serve over TLS and authenticate each RPC
	// using the authentication token.
	authToken, err := generateAuthenticationToken()
	if err != nil {
		return nil, fmt.Errorf("unable to generate authentication token: %v", err)
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(tlsKeyPair)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(authToken))),
	}

	// Start dummy gRPc server mocking the core agent. Ephemeral, since the port reaches the
	// agent through cmd_port and a fixed one collides with the previous test's runner.
	serverListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("unable to listen for the mock core agent: %v", err)
	}
	s := grpc.NewServer(serverOpts...)
	coreAgent := &grpcServer{}
	pb.RegisterAgentSecureServer(s, coreAgent)

	go func() {
		err := s.Serve(serverListener)
		if err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return &agentRunner{
		bindir:               bindir,
		ddAddr:               ddAddr,
		log:                  newSafeBuffer(),
		verbose:              verbose,
		agentServer:          s,
		coreAgent:            coreAgent,
		agentServerListerner: serverListener,
		authToken:            authToken,
		ipcCertPEM:           ipcCertPEM,
	}, nil
}

// cleanup removes the agent binary.
func (s *agentRunner) cleanup() error {
	s.Kill()
	s.agentServer.Stop()
	s.agentServerListerner.Close()
	return os.RemoveAll(s.bindir)
}

// Run runs the agent using a given yaml config. If an agent is already running,
// it will be killed.
func (s *agentRunner) Run(conf []byte) error {
	cfgPath, err := s.createConfigFile(conf)
	if err != nil {
		return fmt.Errorf("agent: error creating config: %v", err)
	}
	timeout := time.After(10 * time.Second)
	exit := s.runAgentConfig(cfgPath)
	for {
		select {
		case err := <-exit:
			return fmt.Errorf("agent: %v, log output:\n%s", err, s.Log())
		case <-timeout:
			return fmt.Errorf("agent: timed out waiting for start, log:\n%s", s.Log())
		default:
			if strings.Contains(s.log.String(), "trace-agent running...") {
				if s.verbose {
					log.Print("agent: trace-agent running...")
				}
				return nil
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// Log returns the tail of the agent log (up to 1M).
func (s *agentRunner) Log() string { return s.log.String() }

// PID returns the process ID of the trace-agent. If the trace-agent is not running
// as a child process of this program, it will be 0.
func (s *agentRunner) PID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pid
}

// Addr returns the address of the trace agent receiver.
func (s *agentRunner) Addr() string { return fmt.Sprintf("localhost:%d", s.port) }

// Kill stops a running trace-agent, if it was started by this process.
func (s *agentRunner) Kill() {
	pid := s.PID()
	if pid == 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		if s.verbose {
			log.Print("couldn't find process: ", err)
		}
		return
	}
	if err := proc.Kill(); err != nil {
		if s.verbose {
			log.Print("couldn't kill running agent: ", err)
		}
		return
	}
	if _, err := proc.Wait(); err != nil {
		if s.verbose {
			log.Print("error waiting for process to exit", err)
		}
		return
	}

	s.mu.Lock()
	s.pid = 0
	s.mu.Unlock()
}

func (s *agentRunner) runAgentConfig(path string) <-chan error {
	s.Kill()
	cmd := exec.Command(filepath.Join(s.bindir, "trace-agent"), "--config", path)
	s.log.Reset()
	cmd.Stdout = s.log
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		log.Print("error starting process: ", err)
	}

	s.mu.Lock()
	s.pid = cmd.Process.Pid
	s.mu.Unlock()

	ch := make(chan error, 1) // don't block
	go func() {
		ch <- cmd.Wait()
		if s.verbose {
			log.Printf("agent: killed")
		}
	}()
	return ch
}

// createConfigFile creates a config file from the given config, altering the
// apm_config.apm_dd_url and log_level values and returns the full path.
func (s *agentRunner) createConfigFile(conf []byte) (string, error) {
	v := create.NewConfig("datadog")
	v.SetTestOnlyDynamicSchema(true)
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(conf)); err != nil {
		return "", err
	}
	if v.IsConfigured("apm_config.receiver_port") {
		s.port = v.GetInt("apm_config.receiver_port")
	} else {
		if p, err := testutil.FindTCPPort(); err != nil {
			fmt.Printf("There was an error finding a free port: %v. Trying 8126.\n", err)
			s.port = 8126
		} else {
			s.port = p
		}
		v.Set("apm_config.receiver_port", s.port, pkgconfigmodel.SourceFile)
	}
	v.Set("apm_config.apm_dd_url", "http://"+s.ddAddr, pkgconfigmodel.SourceFile)
	if !v.IsConfigured("api_key") {
		v.Set("api_key", "testing123", pkgconfigmodel.SourceFile)
	}
	if !v.IsConfigured("hostname") {
		v.Set("hostname", "trace-agent-test", pkgconfigmodel.SourceFile)
	}
	if !v.IsConfigured("apm_config.trace_writer.flush_period_seconds") {
		v.Set("apm_config.trace_writer.flush_period_seconds", 0.1, pkgconfigmodel.SourceFile)
	}
	if !v.IsConfigured("log_level") {
		v.Set("log_level", "debug", pkgconfigmodel.SourceFile)
	}
	if !v.IsConfigured("apm_config.enable_v1_trace_endpoint") {
		v.Set("apm_config.enable_v1_trace_endpoint", true, pkgconfigmodel.SourceFile)
	}

	// The consumer verifies the IPC cert against cmd_host, the certificate's only SAN.
	v.Set("cmd_host", "127.0.0.1", pkgconfigmodel.SourceFile)
	v.Set("cmd_port", s.agentServerListerner.Addr().(*net.TCPAddr).Port, pkgconfigmodel.SourceFile)
	v.Set("auth_token_file_path", filepath.Join(s.bindir, "auth_token"), pkgconfigmodel.SourceFile)
	v.Set("ipc_cert_file_path", filepath.Join(s.bindir, "ipc_cert.pem"), pkgconfigmodel.SourceFile)

	settings := v.AllSettings()
	s.coreAgent.setSettings(configSettings(settings))

	out, err := yaml.Marshal(settings)
	if err != nil {
		return "", err
	}
	confFile, err := os.Create(filepath.Join(s.bindir, "datadog.yaml"))
	if err != nil {
		return "", err
	}
	if _, err := confFile.Write(out); err != nil {
		return "", err
	}
	if err := confFile.Close(); err != nil {
		return "", err
	}
	// create auth_token file
	authTokenFile, err := os.Create(filepath.Join(s.bindir, "auth_token"))
	if err != nil {
		return "", err
	}
	if _, err := authTokenFile.Write([]byte(s.authToken)); err != nil {
		return "", err
	}
	if err := authTokenFile.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.bindir, "ipc_cert.pem"), s.ipcCertPEM, 0600); err != nil {
		return "", err
	}
	return confFile.Name(), nil
}

// configSettings is the generated config overlaid with the DD_* environment. A remote agent
// ignores its own env layer, so a test's t.Setenv only reaches it through the stream.
func configSettings(settings map[string]interface{}) []*pb.ConfigSetting {
	flat := map[string]interface{}{}
	flattenConfigSettings("", settings, flat)
	for key, value := range envConfigSettings() {
		flat[key] = value
	}

	out := make([]*pb.ConfigSetting, 0, len(flat))
	for key, raw := range flat {
		value, err := structpb.NewValue(raw)
		if err != nil {
			continue
		}
		out = append(out, &pb.ConfigSetting{Key: key, Value: value, Source: "file"})
	}
	return out
}

func flattenConfigSettings(prefix string, settings map[string]interface{}, out map[string]interface{}) {
	for key, raw := range settings {
		if prefix != "" {
			key = prefix + "." + key
		}
		if nested, ok := raw.(map[string]interface{}); ok {
			flattenConfigSettings(key, nested, out)
			continue
		}
		out[key] = raw
	}
}

// envConfigSettings resolves DD_* through the agent's schema, the only thing that knows each
// variable's key (DD_APM_ANALYZED_SPANS is apm_config.analyzed_spans).
func envConfigSettings() map[string]interface{} {
	cfg := create.NewConfig("datadog")
	pkgconfigsetup.InitConfig(cfg)
	cfg.BuildSchema()

	out := map[string]interface{}{}
	for _, key := range cfg.AllKeysLowercased() {
		if cfg.GetSource(key) == pkgconfigmodel.SourceEnvVar {
			out[key] = cfg.Get(key)
		}
	}
	return out
}

// buildSelfSignedTLSCertificate returns the fake core agent's key pair and the cert+key PEM
// the spawned agent reads as its IPC cert. The key must be EC: the only type that loader takes.
func buildSelfSignedTLSCertificate(host string) (*tls.Certificate, []byte, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate private key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "trace-agent-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IPAddresses:           []net.IP{net.ParseIP(host)},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to marshal private key: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}
	return &pair, append(certPEM, keyPEM...), nil
}

func generateAuthenticationToken() (string, error) {
	rawToken := make([]byte, 32)
	_, err := rand.Read(rawToken)
	if err != nil {
		return "", fmt.Errorf("can't create authentication token value: %s", err)
	}

	return hex.EncodeToString(rawToken), nil
}
