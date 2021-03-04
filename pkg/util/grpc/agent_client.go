package grpc

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
)

// GetDDAgentClient creates a pb.AgentClient for IPC with the main agent via gRPC. This call is blocking, so
// it is up to the caller to supply a context with appropriate timeout/cancel options
func GetDDAgentClient(ctx context.Context) (pb.AgentClient, error) {
	// This is needed as the server hangs when using "grpc.WithInsecure()"
	tlsConf := tls.Config{InsecureSkipVerify: true}

	opts := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.DefaultConfig}),
		grpc.WithBlock(),
		grpc.WithTransportCredentials(credentials.NewTLS(&tlsConf)),
	}

	target, err := getIPCAddressPort()
	if err != nil {
		return nil, err
	}

	log.Debugf("attempting to create grpc agent client connection to: %s", target)
	conn, err := grpc.DialContext(ctx, target, opts...)

	if err != nil {
		return nil, err
	}

	log.Debug("grpc agent client created")
	return pb.NewAgentClient(conn), nil
}

// getIPCAddressPort returns the host and port for connecting to the main agent
func getIPCAddressPort() (string, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(ipcAddress, config.Datadog.GetString("cmd_port")), nil
}
