// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"time"

	backoffticker "github.com/cenkalti/backoff/v5"
	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

// SecurityAgentAPIClient is used to send request to security module
type SecurityAgentAPIClient struct {
	SecurityAgentAPIClient api.SecurityAgentAPIClient
	conn                   *grpc.ClientConn
	errLogTicker           *backoffticker.Ticker
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connect error logs
func newLogBackoffTicker() *backoffticker.Ticker {
	expBackoff := backoffticker.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.Reset()
	return backoffticker.NewTicker(expBackoff)
}

func (c *SecurityAgentAPIClient) logConnectError(err error) {
	select {
	case <-c.errLogTicker.C:
		msg := fmt.Sprintf("error while connecting to the runtime security agent: %v", err)

		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.Unavailable:
				msg += ", please check that the runtime security agent is enabled in the security-agent.yaml config file"
			}
		}
		seclog.Errorf("%s", msg)
	default:
		// do nothing
	}
}

// SendEvents sends events to the security agent
func (c *SecurityAgentAPIClient) SendEvents(ctx context.Context, msgs chan *api.SecurityEventMessage, onConnectCb func()) {
	for {
		seclog.Trace("connecting to security agent event grpc server")

		stream, err := c.SecurityAgentAPIClient.SendEvent(context.Background())
		if err != nil {
			c.logConnectError(err)

			// Wait for 1 second before trying to send events again
			time.Sleep(time.Second)
			continue
		}

		seclog.Infof("connected to security agent event grpc server")
		onConnectCb()

	LOOP:
		for {
			select {
			case msg := <-msgs:
				err := stream.Send(msg)
				if err != nil {
					seclog.Errorf("error sending event: %v", err)
					break LOOP
				}
			case <-ctx.Done():
				return
			}
		}

		_, _ = stream.CloseAndRecv()

		// Wait for 1 second before trying to send events again
		time.Sleep(time.Second)
	}
}

// SendActivityDumps sends activity dumps to the security agent
func (c *SecurityAgentAPIClient) SendActivityDumps(ctx context.Context, msgs chan *api.ActivityDumpStreamMessage) {
	for {
		seclog.Trace("connecting to security agent activity dump grpc server")

		stream, err := c.SecurityAgentAPIClient.SendActivityDumpStream(context.Background())
		if err != nil {
			c.logConnectError(err)

			// Wait for 1 second before trying to send events again
			time.Sleep(time.Second)
			continue
		}

		seclog.Infof("connected to security agent activity dump grpc server")

	LOOP:
		for {
			select {
			case msg := <-msgs:
				err := stream.Send(msg)
				if err != nil {
					seclog.Errorf("error sending activity dump: %v", err)
					break LOOP
				}
			case <-ctx.Done():
				return
			}
		}

		_, _ = stream.CloseAndRecv()

		// Wait for 1 second before trying to send events again
		time.Sleep(time.Second)
	}
}

// NewSecurityAgentAPIClient instantiates a new SecurityAgentAPIClient
func NewSecurityAgentAPIClient(cfg *config.RuntimeSecurityConfig) (*SecurityAgentAPIClient, error) {
	if cfg.SocketPath == "" {
		return nil, errors.New("runtime_security_config.socket must be set, events will not be sent to the security agent")
	}

	seclog.Infof("connecting to security agent via socket: %s", cfg.SocketPath)
	family, socketPath := socket.GetSocketAddress(cfg.SocketPath)
	if family == "unix" {
		if runtime.GOOS == "windows" {
			return nil, errors.New("unix sockets are not supported on Windows")
		}

		socketPath = "unix://" + socketPath
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(api.VTProtoCodecName)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay: time.Second,
				MaxDelay:  time.Second,
			},
		}),
	}

	seclog.Infof("using socket family '%s' and path '%s' to connect to security agent", family, socketPath)
	if family == "vsock" {
		cmdPort, parseErr := strconv.Atoi(socketPath)
		if parseErr != nil {
			return nil, parseErr
		}

		if cmdPort <= 0 {
			return nil, fmt.Errorf("invalid port '%s' for vsock", cfg.SocketPath)
		}

		socketPath = "passthrough:target"
		opts = append(opts, grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return vsock.Dial(vsock.Host, uint32(cmdPort), &vsock.Config{})
		}))
	}

	conn, err := grpc.NewClient(
		socketPath,
		opts...,
	)

	if err != nil {
		return nil, err
	}

	return &SecurityAgentAPIClient{
		conn:                   conn,
		SecurityAgentAPIClient: api.NewSecurityAgentAPIClient(conn),
		errLogTicker:           newLogBackoffTicker(),
	}, nil
}
