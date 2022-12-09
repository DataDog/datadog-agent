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
	"strings"
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
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
)

var errTaggerStreamNotStarted = errors.New("tagger stream not started")

// Tagger holds a connection to a remote tagger, processes incoming events from
// it, and manages the storage of entities to allow querying.
type Tagger struct {
	store   *tagStore
	ready   bool
	options Options

	conn   *grpc.ClientConn
	client pb.AgentSecureClient
	stream pb.AgentSecure_TaggerStreamEntitiesClient

	streamCtx    context.Context
	streamCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc

	telemetryTicker *time.Ticker
}

type Options struct {
	Target       string
	TokenFetcher func() (string, error)
}

func NodeAgentOptions() (Options, error) {
	return Options{
		Target:       fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		TokenFetcher: security.FetchAuthToken,
	}, nil
}

func CLCRunnerOptions() (Options, error) {
	target, err := clusteragent.GetClusterAgentEndpoint()
	if err != nil {
		return Options{}, fmt.Errorf("unable to get cluster agent endpoint: %w", err)
	}

	// gRPC targets do not have a protocol. the DCA endpoint is always HTTPS,
	// so a simple `TrimPrefix` is enough.
	target = strings.TrimPrefix(target, "https://")

	return Options{
		Target:       target,
		TokenFetcher: security.GetClusterAgentAuthToken,
	}, nil
}

// NewTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
func NewTagger(options Options) *Tagger {
	return &Tagger{
		options: options,
		store:   newTagStore(),
	}
}

// Init initializes the connection to the remote tagger and starts watching for
// events.
func (t *Tagger) Init(ctx context.Context) error {
	t.telemetryTicker = time.NewTicker(1 * time.Minute)

	t.ctx, t.cancel = context.WithCancel(ctx)

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
		t.options.Target,
		grpc.WithTransportCredentials(creds),
		grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
			return net.Dial("tcp", url)
		}),
	)
	if err != nil {
		return err
	}

	t.client = pb.NewAgentSecureClient(t.conn)

	timeout := time.Duration(config.Datadog.GetInt("remote_tagger_timeout_seconds")) * time.Second
	err = t.startTaggerStream(timeout)
	if err != nil {
		// tagger stopped before being connected
		if err == errTaggerStreamNotStarted {
			return nil
		}
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

	log.Info("remote tagger stopped successfully")

	return nil
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *Tagger) Tag(entityID string, cardinality collectors.TagCardinality) ([]string, error) {
	entity := t.store.getEntity(entityID)
	if entity != nil {
		telemetry.QueriesByCardinality(cardinality).Success.Inc()
		return entity.GetTags(cardinality), nil
	}

	telemetry.QueriesByCardinality(cardinality).EmptyTags.Inc()

	return []string{}, nil
}

// AccumulateTagsFor returns tags for a given entity at the desired cardinality.
func (t *Tagger) AccumulateTagsFor(entityID string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.Tag(entityID, cardinality)
	if err != nil {
		return err
	}
	tb.Append(tags...)
	return nil
}

// Standard returns the standard tags for a given entity.
func (t *Tagger) Standard(entityID string) ([]string, error) {
	entity := t.store.getEntity(entityID)
	if entity == nil {
		return []string{}, nil
	}

	return entity.StandardTags, nil
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {
	entity := t.store.getEntity(entityID)
	if entity == nil {
		return nil, fmt.Errorf("Entity not found for entityID")
	}

	return entity, nil
}

// List returns all the entities currently stored by the tagger.
func (t *Tagger) List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	entities := t.store.listEntities()
	resp := tagger_api.TaggerListResponse{
		Entities: make(map[string]tagger_api.TaggerListEntity),
	}

	for _, e := range entities {
		resp.Entities[e.ID] = tagger_api.TaggerListEntity{
			Tags: map[string][]string{
				remoteSource: e.GetTags(collectors.HighCardinality),
			},
		}
	}

	return resp
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
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
		case <-t.telemetryTicker.C:
			t.store.collectTelemetry()
			continue
		case <-t.ctx.Done():
			return
		default:
		}

		if t.stream == nil {
			if err := t.startTaggerStream(noTimeout); err != nil {
				log.Warnf("error received trying to start stream with target %q: %s", t.options.Target, err)
				continue
			}
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
			// can no longer be considered ready, and the stream
			// must be re-established.
			t.ready = false
			t.stream = nil

			log.Warnf("error received from remote tagger: %s", err)

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
	// returning early when there are no events prevents a keep-alive sent
	// from the core agent from wiping the store clean in case the remote
	// tagger was previously in an unready (but filled) state.
	if len(response.Events) == 0 {
		return nil
	}

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
			return &backoff.PermanentError{Err: errTaggerStreamNotStarted}
		default:
		}

		token, err := t.options.TokenFetcher()
		if err != nil {
			log.Infof("unable to fetch auth token, will possibly retry: %s", err)
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
