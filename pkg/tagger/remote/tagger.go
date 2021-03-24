// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultTimeout    = 5 * time.Minute
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
)

// Tagger holds a connection to a remote tagger, processes incoming events from
// it, and manages the storage of entities to allow querying.
type Tagger struct {
	store *tagStore
	ready bool

	conn   *grpc.ClientConn
	client pb.AgentSecureClient
	stream pb.AgentSecure_TaggerStreamEntitiesClient

	streamCtx    context.Context
	streamCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc

	health          *health.Handle
	telemetryTicker *time.Ticker
}

// NewTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
func NewTagger() *Tagger {
	return &Tagger{
		store: newTagStore(),
	}
}

// Init initializes the connection to the remote tagger and starts watching for
// events.
func (t *Tagger) Init() error {
	t.health = health.RegisterLiveness("tagger")
	t.telemetryTicker = time.NewTicker(1 * time.Minute)

	t.ctx, t.cancel = context.WithCancel(context.Background())

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	var err error
	t.conn, err = grpc.DialContext(
		t.ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	t.client = pb.NewAgentSecureClient(t.conn)

	err = t.startTaggerStream(defaultTimeout)
	if err != nil {
		return err
	}

	log.Info("remote tagger initialized successfully")

	go t.run()

	return nil
}

// Stop closes the connection to the remote tagger and stops event collection.
func (t *Tagger) Stop() error {
	t.cancel()

	err := t.conn.Close()
	if err != nil {
		return err
	}

	t.telemetryTicker.Stop()
	err = t.health.Deregister()
	if err != nil {
		return err
	}

	log.Info("remote tagger stopped successfully")

	return nil
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *Tagger) Tag(entityID string, cardinality collectors.TagCardinality) ([]string, error) {
	telemetry.Queries.Inc(collectors.TagCardinalityToString(cardinality))

	entity := t.store.getEntity(entityID)
	if entity != nil {
		return entity.GetTags(cardinality), nil
	}

	return []string{}, nil
}

// TagBuilder returns tags for a given entity at the desired cardinality.
func (t *Tagger) TagBuilder(entityID string, cardinality collectors.TagCardinality, tb *util.TagsBuilder) error {
	tags, err := t.Tag(entityID, cardinality)
	if err == nil {
		tb.Append(tags...)
	}
	return err
}

// Standard returns the standard tags for a given entity.
func (t *Tagger) Standard(entityID string) ([]string, error) {
	entity := t.store.getEntity(entityID)
	if entity == nil {
		return []string{}, nil
	}

	return entity.StandardTags, nil
}

// List returns all the entities currently stored by the tagger.
func (t *Tagger) List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	entities := t.store.listEntities()
	resp := response.TaggerListResponse{
		Entities: make(map[string]response.TaggerListEntity),
	}

	for _, e := range entities {
		resp.Entities[e.ID] = response.TaggerListEntity{
			Tags: e.GetTags(collectors.HighCardinality),
		}
	}

	return resp
}

// Subscribe returns a list of existing entities in the store, alongside a
// channel that receives events whenever an entity is added, modified or
// deleted.
func (t *Tagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return t.store.subscribe(cardinality)
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (t *Tagger) Unsubscribe(ch chan []types.EntityEvent) {
	t.store.unsubscribe(ch)
}

func (t *Tagger) run() {
	for {
		select {
		case <-t.health.C:
		case <-t.telemetryTicker.C:
			t.store.collectTelemetry()
		case <-t.ctx.Done():
			return
		default:
		}

		var response *pb.StreamTagsResponse
		err := grpcutil.DoWithTimeout(func() error {
			var err error
			response, err = t.stream.Recv()
			return err
		}, streamRecvTimeout)

		if err != nil {
			t.streamCancel()

			telemetry.ClientStreamErrors.Inc()

			// when Recv() returns an error, the stream is aborted
			// and the contents of our store are considered out of
			// sync and therefore no longer valid, so the tagger
			// can no longer be considered ready
			t.ready = false

			log.Warnf("error received from remote tagger: %s", err)

			// startTaggerStream(noTimeout) will never return
			// unless a stream can be established, or the tagger
			// has been stopped, which means the error handling
			// here is just a sanity check.
			if err := t.startTaggerStream(noTimeout); err != nil {
				log.Warnf("error received trying to start stream: %s", err)
			}
			continue
		}

		telemetry.Receives.Inc()

		err = t.processResponse(response)
		if err != nil {
			log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}
	}
}

func (t *Tagger) processResponse(response *pb.StreamTagsResponse) error {
	events := make([]types.EntityEvent, 0, len(response.Events))
	for _, ev := range response.Events {
		eventType, err := convertEventType(ev.Type)
		if err != nil {
			log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}

		entity := ev.Entity
		events = append(events, types.EntityEvent{
			EventType: eventType,
			Entity: types.Entity{
				ID:                          convertEntityID(entity.Id),
				HighCardinalityTags:         entity.HighCardinalityTags,
				OrchestratorCardinalityTags: entity.OrchestratorCardinalityTags,
				LowCardinalityTags:          entity.LowCardinalityTags,
				StandardTags:                entity.StandardTags,
			},
		})
	}

	// if the tagger was not ready by this point, it means an error
	// occurred and the contents of the store are no longer valid and need
	// to be replaced by the batch coming from the current response
	replaceStoreContents := !t.ready

	err := t.store.processEvents(events, replaceStoreContents)
	if err != nil {
		return err
	}

	t.ready = true

	return nil
}

// startTaggerStream tries to establish a stream with the remote gRPC endpoint.
// Since the entire remote tagger really depends on this working, it'll keep on
// retrying with an exponential backoff until maxElapsed (or forever if
// maxElapsed == 0) or the tagger is stopped.
func (t *Tagger) startTaggerStream(maxElapsed time.Duration) error {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 500 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = maxElapsed

	return backoff.Retry(func() error {
		select {
		case <-t.ctx.Done():
			return nil
		default:
		}

		token, err := security.FetchAuthToken()
		if err != nil {
			err = fmt.Errorf("unable to fetch authentication token: %w", err)
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		t.streamCtx, t.streamCancel = context.WithCancel(
			metadata.NewOutgoingContext(t.ctx, metadata.MD{
				"authorization": []string{fmt.Sprintf("Bearer %s", token)},
			}),
		)

		t.stream, err = t.client.TaggerStreamEntities(t.streamCtx, &pb.StreamTagsRequest{
			Cardinality: pb.TagCardinality_HIGH,
		})

		if err != nil {
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		log.Info("tagger stream established successfully")

		return nil
	}, expBackoff)
}

func convertEventType(t pb.EventType) (types.EventType, error) {
	switch t {
	case pb.EventType_ADDED:
		return types.EventTypeAdded, nil
	case pb.EventType_MODIFIED:
		return types.EventTypeModified, nil
	case pb.EventType_DELETED:
		return types.EventTypeDeleted, nil
	}

	return types.EventTypeAdded, fmt.Errorf("unknown event type: %q", t)
}

func convertEntityID(id *pb.EntityId) string {
	return fmt.Sprintf("%s://%s", id.Prefix, id.Uid)
}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())
}
