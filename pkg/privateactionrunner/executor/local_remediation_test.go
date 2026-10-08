// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package executor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/protobuf/proto"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	rshell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/rshell"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

type sandboxExecutor struct {
	verifier taskverifier.TaskVerifier
	handler  *rshell.RunCommandHandler
	tasks    []*types.Task
}

func (e *sandboxExecutor) PrepareTask(_ context.Context, task *types.Task) (*runners.PreparedWorkflowTask, *types.Task, error) {
	verified, err := e.verifier.UnwrapTask(task)
	if err != nil {
		return nil, task, err
	}
	e.tasks = append(e.tasks, verified)
	return &runners.PreparedWorkflowTask{Task: verified}, nil, nil
}

func (e *sandboxExecutor) RunPrepared(ctx context.Context, prepared *runners.PreparedWorkflowTask) (interface{}, error) {
	return e.handler.Run(ctx, prepared.Task, nil)
}

func localTestClient(t *testing.T, srv *Server, shared bool) pb.ExecutorClient {
	t.Helper()
	ca, key := newTestCA(t)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	cert := newLeafCert(t, ca, key, "localhost", x509.ExtKeyUsageAny)
	srv.SetControlPlaneConfig(&pb.GetControlPlaneConfigResponse{}, cert.Certificate[0])
	address := testListenAddr(t)
	listener, err := Listen(address)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, srv, ServeOptions{}, grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool,
		})))
	}()
	clientCert := cert
	if !shared {
		clientCert = newLeafCert(t, ca, key, "other-local-client", x509.ExtKeyUsageClientAuth)
	}
	conn, err := grpc.NewClient("passthrough:///"+address,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{clientCert}, RootCAs: pool, ServerName: "localhost"})),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return Dial(ctx, address, time.Second) }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(); cancel(); require.NoError(t, <-done) })
	return pb.NewExecutorClient(conn)
}

func TestLocalRemediationSandboxAndStop(t *testing.T) {
	for _, scenario := range []string{"success", "denied path", "denied command", "failed step"} {
		t.Run(scenario, func(t *testing.T) {
			dir := filepath.ToSlash(t.TempDir())
			first := dir + "/first"
			later := dir + "/later"
			core := &sandboxExecutor{
				verifier: taskverifier.NewLocalTrustVerifier(&config.Config{OrgId: 42, RunnerId: "local-runner"}),
				handler:  rshell.NewRunRemediationCommandHandler(rshell.RunCommandHandlerConfig{OperatorAllowedPaths: []string{dir + ":rw"}, OperatorAllowedCommands: []string{"rshell:echo", "rshell:false"}, DisableDetailedTelemetry: true}),
			}
			srv := NewServer(&fakeExecutor{}, "test")
			srv.SetLocalRemediationExecutor(core)
			srv.SetReady(true)
			client := localTestClient(t, srv, true)
			request := &pb.RunLocalRemediationRequest{
				Commands:  []string{fmt.Sprintf("echo remediated > %q", first), fmt.Sprintf("echo remediated > %q", later)},
				Allowlist: &pb.RemediationAllowlist{AllowedPaths: []string{dir + ":rw"}, AllowedCommands: []string{"rshell:echo", "rshell:false"}},
			}
			switch scenario {
			case "denied path":
				request.Allowlist.AllowedPaths = nil
			case "denied command":
				request.Allowlist.AllowedCommands = nil
			case "failed step":
				request.Commands[0] = "false"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			response, err := client.RunLocalRemediation(ctx, request)
			require.NoError(t, err)
			if scenario == "success" {
				require.Len(t, response.Steps, 2)
				require.Empty(t, response.Steps[0].Error)
				require.Zero(t, response.Steps[0].ExitCode)
				require.FileExists(t, first)
				require.FileExists(t, later)
			} else {
				require.Len(t, response.Steps, 1)
				require.True(t, response.Steps[0].Error != "" || response.Steps[0].ExitCode != 0)
				_, err := os.Stat(later)
				require.True(t, os.IsNotExist(err))
			}
			require.NotEmpty(t, core.tasks)
			attrs := core.tasks[0].Data.Attributes
			require.Equal(t, "com.datadoghq.remoteaction.rshell", attrs.BundleID)
			require.Equal(t, "runRemediationCommand", attrs.Name)
			require.Equal(t, request.Commands[0], attrs.Inputs["command"])
			require.Equal(t, int64(42), attrs.OrgId)
			require.Equal(t, "local-runner", attrs.ConnectionInfo.RunnerId)
			require.Nil(t, attrs.SignedEnvelope)
			require.Nil(t, attrs.VerificationKey)
			require.NotContains(t, attrs.Inputs, "effectivePermissions")
		})
	}
}

func TestLocalRemediationAuthorizationAndGates(t *testing.T) {
	for _, scenario := range []struct {
		name                   string
		shared, enabled, ready bool
		code                   codes.Code
	}{
		{"other certificate", false, true, true, codes.PermissionDenied},
		{"disabled", true, false, true, codes.Unimplemented},
		{"not ready", true, true, false, codes.Unavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			srv := NewServer(&fakeExecutor{}, "test")
			if scenario.enabled {
				srv.SetLocalRemediationExecutor(&fakeExecutor{})
			}
			srv.SetReady(scenario.ready)
			client := localTestClient(t, srv, scenario.shared)
			_, err := client.RunLocalRemediation(context.Background(), &pb.RunLocalRemediationRequest{Commands: []string{"true"}})
			require.Equal(t, scenario.code, status.Code(err))
		})
	}
	srv := NewServer(nil, "test")
	_, err := startTestServer(t, srv).RunLocalRemediation(context.Background(), &pb.RunLocalRemediationRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestLocalRemediationOutputIsSafeForProtobuf(t *testing.T) {
	for _, output := range []string{strings.Repeat("a", 4095) + "€", string([]byte{0xff, 0xfe}), "api_key: 0123456789abcdef0123456789abcdef"} {
		scrubbed := scrubLocalOutput(output)
		require.True(t, utf8.ValidString(scrubbed))
		require.NotContains(t, scrubbed, "0123456789abcdef0123456789abcdef")
		_, err := proto.Marshal(&pb.RemediationStepResult{Stdout: scrubbed})
		require.NoError(t, err)
	}
	require.Equal(t, strings.Repeat("a", 4095)+" [truncated]", scrubLocalOutput(strings.Repeat("a", 4095)+"€"))
}
