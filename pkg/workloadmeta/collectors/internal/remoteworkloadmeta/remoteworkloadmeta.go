// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteworkloadmeta

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
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

const (
	collectorID       = "remote-workloadmeta"
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
)

// Note: some code in this collector is the same as in the remote-tagger.

var errWorkloadmetaStreamNotStarted = errors.New("workloadmeta stream not started")

type collector struct {
	store        workloadmeta.Store
	resyncNeeded bool

	conn   *grpc.ClientConn
	client pb.AgentSecureClient
	stream pb.AgentSecure_WorkloadmetaStreamEntitiesClient

	streamCtx    context.Context
	streamCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())

	workloadmeta.RegisterRemoteCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	c.store = store

	c.ctx, c.cancel = context.WithCancel(ctx)

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	var err error
	c.conn, err = grpc.DialContext(
		c.ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
		grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
			return net.Dial("tcp", url)
		}),
	)
	if err != nil {
		return err
	}

	c.client = pb.NewAgentSecureClient(c.conn)

	log.Info("remote workloadmeta initialized successfully")

	go c.run()

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	return nil
}

func (c *collector) startWorkloadmetaStream(maxElapsed time.Duration) error {
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
			log.Infof("unable to establish stream, will possibly retry: %s", err)
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

		c.stream, err = c.client.WorkloadmetaStreamEntities(
			c.streamCtx,
			&pb.WorkloadmetaStreamRequest{
				Filter: nil, // Subscribes to all events
			},
		)

		if err != nil {
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		log.Info("workloadmeta stream established successfully")

		return nil
	}, expBackoff)
}

func (c *collector) run() {
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

		var response *pb.WorkloadmetaStreamResponse
		err := grpcutil.DoWithTimeout(func() error {
			var err error
			response, err = c.stream.Recv()
			return err
		}, streamRecvTimeout)
		if err != nil {
			c.streamCancel()

			telemetry.RemoteClientErrors.Inc()

			// when Recv() returns an error, the stream is aborted and the
			// contents of our store are considered out of sync. The stream must
			// be re-established. No events are notified to the store until the
			// connection is re-established.
			c.stream = nil
			c.resyncNeeded = true

			log.Warnf("error received from remote workloadmeta: %s", err)

			continue
		}

		err = c.processResponse(response)
		if err != nil {
			log.Warnf("error processing event received from remote workloadmeta: %s", err)
			continue
		}
	}
}

func (c *collector) processResponse(response *pb.WorkloadmetaStreamResponse) error {
	var collectorEvents []workloadmeta.CollectorEvent

	for _, protoEvent := range response.Events {
		workloadmetaEvent, err := utils.WorkloadmetaEventFromProtoEvent(protoEvent)
		if err != nil {
			return err
		}

		collectorEvent := workloadmeta.CollectorEvent{
			Type:   workloadmetaEvent.Type,
			Source: workloadmeta.SourceRemoteWorkloadmeta,
			Entity: workloadmetaEvent.Entity,
		}

		collectorEvents = append(collectorEvents, collectorEvent)
	}

	if c.resyncNeeded {
		var entities []workloadmeta.Entity
		for _, event := range collectorEvents {
			entities = append(entities, event.Entity)
		}

		// This should be the first response that we got from workloadmeta after
		// we lost the connection and specified that a re-sync is needed. So, at
		// this point we know that "entities" contains all the existing entities
		// in the store, because when a client subscribes to workloadmeta, the
		// first response is always a bundle of events with all the existing
		// entities in the store that match the filters specified (see
		// workloadmeta.Store#Subscribe).
		c.store.Reset(entities, workloadmeta.SourceRemoteWorkloadmeta)
		c.resyncNeeded = false
		return nil
	}

	c.store.Notify(collectorEvents)
	return nil
}
