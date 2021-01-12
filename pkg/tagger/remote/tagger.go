// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tagger holds a connection to a remote tagger, processes incoming events from
// it, and manages the storage of entities to allow querying.
type Tagger struct {
	store *tagStore

	conn   *grpc.ClientConn
	client pb.AgentSecureClient
	stream pb.AgentSecure_TaggerStreamEntitiesClient

	ctx    context.Context
	cancel context.CancelFunc

	health *health.Handle
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

	t.ctx, t.cancel = context.WithCancel(context.Background())

	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("unable to fetch authentication token: %w", err)
	}

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	t.ctx = metadata.NewOutgoingContext(t.ctx, md)

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	t.conn, err = grpc.DialContext(
		t.ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	t.client = pb.NewAgentSecureClient(t.conn)

	err = t.startTaggerStream(5 * time.Minute)
	if err != nil {
		return err
	}

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

	err = t.health.Deregister()
	if err != nil {
		return err
	}

	return nil
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *Tagger) Tag(entityID string, cardinality collectors.TagCardinality) ([]string, error) {
	telemetry.Queries.Inc(collectors.TagCardinalityToString(cardinality))

	entity, err := t.store.getEntity(entityID)
	if err != nil {
		return nil, err
	}

	return entity.GetTags(cardinality), nil
}

// Standard returns the standard tags for a given entity.
func (t *Tagger) Standard(entityID string) ([]string, error) {
	entity, err := t.store.getEntity(entityID)
	if err != nil {
		return nil, err
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
		case <-t.ctx.Done():
			return
		default:
		}

		response, err := t.stream.Recv()
		if err != nil {
			log.Warnf("error received from remote tagger: %s", err)

			// NOTE: when Recv() returns an error, the stream is
			// aborted and needs to be re-established, so we do
			// that here. also, startTaggerStream(0) will never
			// return unless a stream can be established, or the
			// tagger has been stopped, which means the error
			// handling here is just a sanity check.
			if err := t.startTaggerStream(0); err != nil {
				log.Warnf("error received trying to start stream: %s", err)
			}
			continue
		}

		eventType, err := convertEventType(response.Type)
		if err != nil {
			log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}

		err = t.store.processEvent(types.EntityEvent{
			EventType: eventType,
			Entity: types.Entity{
				ID:                          convertEntityID(response.Entity.Id),
				HighCardinalityTags:         response.Entity.HighCardinalityTags,
				OrchestratorCardinalityTags: response.Entity.OrchestratorCardinalityTags,
				LowCardinalityTags:          response.Entity.LowCardinalityTags,
				StandardTags:                response.Entity.StandardTags,
			},
		})
		if err != nil {
			log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}
	}
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

		var err error
		t.stream, err = t.client.TaggerStreamEntities(t.ctx, &pb.StreamTagsRequest{
			Cardinality: pb.TagCardinality_HIGH,
		})

		if err != nil {
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		// when the stream is aborted, the store needs to be reset as
		// its contents can no longer be relied to be up to date, and
		// the new stream is responsible for re-adding all the still
		// existing entities back. for the time in between, this can
		// cause queries to the tagger to return nothing.
		t.store.reset()

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
