// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

const (
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
)

var errWorkloadmetaStreamNotStarted = errors.New("workloadmeta stream not started")

type RemoteGrpcClient interface {
	// StreamEntites establishes the stream between the client and the remote gRPC server.
	StreamEntities(ctx context.Context, opts ...grpc.CallOption) (Stream, error)
}

type Stream interface {
	// Recv returns a response of the gRPC server
	Recv() (interface{}, error)
}

type StreamHandler interface {
	// Port returns the targeted port
	Port() int
	// IsEnabled returns if the feature is enabled
	IsEnabled() bool
	// NewClient returns a client to connect to a remote gRPC server.
	NewClient(cc grpc.ClientConnInterface) RemoteGrpcClient
	// HandleResponse handles a response from the remote gRPC server.
	HandleResponse(response interface{}) ([]workloadmeta.CollectorEvent, error)
	// HandleResync is called on resynchronization.
	HandleResync(store workloadmeta.Store, events []workloadmeta.CollectorEvent)
}

// GenericCollector is a generic remote workloadmeta collector with resync mechanisms.
type GenericCollector struct {
	CollectorID   string
	StreamHandler StreamHandler

	store        workloadmeta.Store
	resyncNeeded bool

	client RemoteGrpcClient
	stream Stream

	streamCtx    context.Context
	streamCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc

	Insecure bool // for testing
}

func (c *GenericCollector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !c.StreamHandler.IsEnabled() {
		return fmt.Errorf("collector %s is not enabled", c.CollectorID)
	}

	c.store = store

	c.ctx, c.cancel = context.WithCancel(ctx)

	opts := []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("tcp", url)
	})}

	if c.Insecure {
		// for test purposes
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// NOTE: we're using InsecureSkipVerify because the gRPC server only
		// persists its TLS certs in memory, and we currently have no
		// infrastructure to make them available to clients. This is NOT
		// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
		// connection.
		creds := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	conn, err := grpc.DialContext(
		c.ctx,
		fmt.Sprintf(":%v", c.StreamHandler.Port()),
		opts...,
	)
	if err != nil {
		return err
	}

	c.client = c.StreamHandler.NewClient(conn)

	log.Info("remote workloadmeta initialized successfully")
	go c.Run()

	return nil
}

func (c *GenericCollector) Pull(context.Context) error {
	return nil
}

func (c *GenericCollector) startWorkloadmetaStream(maxElapsed time.Duration) error {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 500 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = maxElapsed

	return backoff.Retry(func() error {
		select {
		case <-c.ctx.Done():
			return &backoff.PermanentError{Err: errWorkloadmetaStreamNotStarted}
		default:
		}

		token, err := security.FetchAuthToken()
		if err != nil {
			err = fmt.Errorf("unable to fetch authentication token: %w", err)
			log.Warnf("unable to establish entity stream between agents, will possibly retry: %s", err)
			return err
		}

		c.streamCtx, c.streamCancel = context.WithCancel(
			metadata.NewOutgoingContext(
				c.ctx,
				metadata.MD{
					"authorization": []string{
						fmt.Sprintf("Bearer %s", token),
					},
				},
			),
		)

		c.stream, err = c.client.StreamEntities(c.streamCtx)
		if err != nil {
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		log.Info("workloadmeta stream established successfully")
		return nil
	}, expBackoff)
}

func (c *GenericCollector) Run() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if c.stream == nil {
			if err := c.startWorkloadmetaStream(noTimeout); err != nil {
				log.Warnf("error received trying to start stream: %s", err)
				continue
			}
		}

		var response interface{}
		err := grpcutil.DoWithTimeout(func() error {
			var err error
			response, err = c.stream.Recv()
			return err
		}, streamRecvTimeout)
		if err != nil {
			c.streamCancel()

			telemetry.RemoteClientErrors.Inc(c.CollectorID)

			// when Recv() returns an error, the stream is aborted and the
			// contents of our store are considered out of sync. The stream must
			// be re-established. No events are notified to the store until the
			// connection is re-established.
			c.stream = nil
			c.resyncNeeded = true

			log.Warnf("error received from remote workloadmeta: %s", err)

			continue
		}

		collectorEvents, err := c.StreamHandler.HandleResponse(response)
		if err != nil {
			log.Warnf("error processing event received from remote workloadmeta: %s", err)
			continue
		}

		if c.resyncNeeded {
			c.StreamHandler.HandleResync(c.store, collectorEvents)
			c.resyncNeeded = false
		}

		c.store.Notify(collectorEvents)
	}
}
