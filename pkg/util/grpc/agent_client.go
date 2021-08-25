package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
)

var defaultBackoffConfig = backoff.Config{
	BaseDelay:  1.0 * time.Second,
	Multiplier: 1.1,
	Jitter:     0.2,
	MaxDelay:   2 * time.Second,
}

// defaultAgentDialOpts default dial options to the main agent which blocks and retries based on the backoffConfig
var defaultAgentDialOpts = []grpc.DialOption{
	grpc.WithConnectParams(grpc.ConnectParams{Backoff: defaultBackoffConfig}),
	grpc.WithBlock(),
}

// GetDDAgentClient creates a pb.AgentClient for IPC with the main agent via gRPC. This call is blocking by default, so
// it is up to the caller to supply a context with appropriate timeout/cancel options
func GetDDAgentClient(ctx context.Context, opts ...grpc.DialOption) (pb.AgentClient, error) {
	if config.Datadog.GetString("cmd_port") == "-1" {
		return nil, errors.New("grpc client disabled via cmd_port: -1")
	}
	// This is needed as the server hangs when using "grpc.WithInsecure()"
	tlsConf := tls.Config{InsecureSkipVerify: true}

	if len(opts) == 0 {
		opts = defaultAgentDialOpts
	}

	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tlsConf)))

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
