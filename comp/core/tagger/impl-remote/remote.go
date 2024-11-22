// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remotetaggerimpl implements a remote Tagger.
package remotetaggerimpl

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

const (
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
)

var errTaggerStreamNotStarted = errors.New("tagger stream not started")

// Requires defines the dependencies for the remote tagger.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Params    tagger.RemoteParams
	Telemetry coretelemetry.Component
}

// Provides contains the fields provided by the remote tagger constructor.
type Provides struct {
	compdef.Out

	Comp tagger.Component
}

type remoteTagger struct {
	store   *tagStore
	ready   bool
	options Options

	cfg config.Component
	log log.Component

	conn   *grpc.ClientConn
	client pb.AgentSecureClient
	stream pb.AgentSecure_TaggerStreamEntitiesClient

	streamCtx    context.Context
	streamCancel context.CancelFunc
	filter       *types.Filter

	ctx    context.Context
	cancel context.CancelFunc

	telemetryTicker *time.Ticker
	telemetryStore  *telemetry.Store

	checksCardinality    types.TagCardinality
	dogstatsdCardinality types.TagCardinality
}

// Options contains the options needed to configure the remote tagger.
type Options struct {
	Target       string
	TokenFetcher func() (string, error)
	Disabled     bool
}

// NewComponent returns a remote tagger
func NewComponent(req Requires) (Provides, error) {
	remoteTagger, err := NewRemoteTagger(req.Params, req.Config, req.Log, req.Telemetry)

	if err != nil {
		return Provides{}, err
	}

	req.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {
		mainCtx, _ := common.GetMainCtxCancel()
		return remoteTagger.Start(mainCtx)
	}})
	req.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		return remoteTagger.Stop()
	}})

	return Provides{
		Comp: remoteTagger,
	}, nil
}

// NewRemoteTagger creates a new remote tagger.
// TODO: (components) remove once we pass the remote tagger instance to pkg/security/resolvers/tags/resolver.go
func NewRemoteTagger(params tagger.RemoteParams, cfg config.Component, log log.Component, telemetryComp coretelemetry.Component) (tagger.Component, error) {
	telemetryStore := telemetry.NewStore(telemetryComp)

	target, err := params.RemoteTarget(cfg)
	if err != nil {
		return nil, err
	}

	remotetagger := &remoteTagger{
		options: Options{
			Target:       target,
			TokenFetcher: params.RemoteTokenFetcher(cfg),
		},
		cfg:            cfg,
		store:          newTagStore(telemetryStore),
		telemetryStore: telemetryStore,
		filter:         params.RemoteFilter,
		log:            log,
	}

	checkCard := cfg.GetString("checks_tag_cardinality")
	dsdCard := cfg.GetString("dogstatsd_tag_cardinality")
	remotetagger.checksCardinality, err = types.StringToTagCardinality(checkCard)
	if err != nil {
		log.Warnf("failed to parse check tag cardinality, defaulting to low. Error: %s", err)
		remotetagger.checksCardinality = types.LowCardinality
	}

	remotetagger.dogstatsdCardinality, err = types.StringToTagCardinality(dsdCard)
	if err != nil {
		log.Warnf("failed to parse dogstatsd tag cardinality, defaulting to low. Error: %s", err)
		remotetagger.dogstatsdCardinality = types.LowCardinality
	}

	return remotetagger, nil
}

// Start creates the connection to the remote tagger and starts watching for
// events.
func (t *remoteTagger) Start(ctx context.Context) error {
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
	t.conn, err = grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		t.ctx,
		t.options.Target,
		grpc.WithTransportCredentials(creds),
		grpc.WithContextDialer(func(_ context.Context, url string) (net.Conn, error) {
			return net.Dial("tcp", url)
		}),
	)
	if err != nil {
		return err
	}

	t.client = pb.NewAgentSecureClient(t.conn)

	t.log.Info("remote tagger initialized successfully")

	go t.run()

	return nil
}

// Stop closes the connection to the remote tagger and stops event collection.
func (t *remoteTagger) Stop() error {
	t.cancel()

	err := t.conn.Close()
	if err != nil {
		return err
	}

	t.telemetryTicker.Stop()

	t.log.Info("remote tagger stopped successfully")

	return nil
}

// ReplayTagger returns the replay tagger instance
// This is a no-op for the remote tagger
func (t *remoteTagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *remoteTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *remoteTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	entity := t.store.getEntity(entityID)
	if entity != nil {
		t.telemetryStore.QueriesByCardinality(cardinality).Success.Inc()
		return entity.GetTags(cardinality), nil
	}

	t.telemetryStore.QueriesByCardinality(cardinality).EmptyTags.Inc()

	return []string{}, nil
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *remoteTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// AccumulateTagsFor returns tags for a given entity at the desired cardinality.
func (t *remoteTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.Tag(entityID, cardinality)
	if err != nil {
		return err
	}
	tb.Append(tags...)
	return nil
}

// Standard returns the standard tags for a given entity.
func (t *remoteTagger) Standard(entityID types.EntityID) ([]string, error) {
	entity := t.store.getEntity(entityID)
	if entity == nil {
		return []string{}, nil
	}

	return entity.StandardTags, nil
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *remoteTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	entity := t.store.getEntity(entityID)
	if entity == nil {
		return nil, fmt.Errorf("Entity not found for entityID")
	}

	return entity, nil
}

// List returns all the entities currently stored by the tagger.
func (t *remoteTagger) List() types.TaggerListResponse {
	entities := t.store.listEntities()
	resp := types.TaggerListResponse{
		Entities: make(map[string]types.TaggerListEntity),
	}

	for _, e := range entities {
		resp.Entities[e.ID.String()] = types.TaggerListEntity{
			Tags: map[string][]string{
				remoteSource: e.GetTags(types.HighCardinality),
			},
		}
	}

	return resp
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func (t *remoteTagger) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	tags, err := t.Tag(entityID, cardinality)
	if err != nil {
		return ""
	}
	return utils.ComputeTagsHash(tags)
}

// AgentTags is a no-op in the remote tagger.
// Agents using the remote tagger are not supposed to rely on this function,
// because to get the container ID where the agent is running we'd need to
// introduce some dependencies that we don't want to have in the remote
// tagger.
// The only user of this function that uses the remote tagger is the cluster
// check runner, but it gets its tags from the cluster-agent which doesn't
// store tags for containers. So this function is a no-op.
func (t *remoteTagger) AgentTags(_ types.TagCardinality) ([]string, error) {
	return nil, nil
}

func (t *remoteTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return t.Tag(taggercommon.GetGlobalEntityID(), cardinality)
}

func (t *remoteTagger) SetNewCaptureTagger(tagger.Component) {}

func (t *remoteTagger) ResetCaptureTagger() {}

// EnrichTags enriches the tags with the global tags.
// Agents running the remote tagger don't have the ability to enrich tags based
// on the origin info. Only the core agent or dogstatsd can have origin info,
// and they always use the local tagger.
// This function can only add the global tags.
func (t *remoteTagger) EnrichTags(tb tagset.TagsAccumulator, _ taggertypes.OriginInfo) {
	if err := t.AccumulateTagsFor(taggercommon.GetGlobalEntityID(), t.dogstatsdCardinality, tb); err != nil {
		t.log.Error(err.Error())
	}
}

func (t *remoteTagger) ChecksCardinality() types.TagCardinality {
	return t.checksCardinality
}

func (t *remoteTagger) DogstatsdCardinality() types.TagCardinality {
	return t.dogstatsdCardinality
}

// Subscribe currently returns a non-nil error indicating that the method is not supported
// for remote tagger. Currently, there are no use cases for client subscribing to remote tagger events
func (t *remoteTagger) Subscribe(string, *types.Filter) (types.Subscription, error) {
	return nil, errors.New("subscription to the remote tagger is not currently supported")
}

func (t *remoteTagger) run() {
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
				t.log.Warnf("error received trying to start stream with target %q: %s", t.options.Target, err)
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

			t.telemetryStore.ClientStreamErrors.Inc()

			// when Recv() returns an error, the stream is aborted
			// and the contents of our store are considered out of
			// sync and therefore no longer valid, so the tagger
			// can no longer be considered ready, and the stream
			// must be re-established.
			t.ready = false
			t.stream = nil

			t.log.Warnf("error received from remote tagger: %s", err)

			continue
		}

		t.telemetryStore.Receives.Inc()

		err = t.processResponse(response)
		if err != nil {
			t.log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}
	}
}

func (t *remoteTagger) processResponse(response *pb.StreamTagsResponse) error {
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
			t.log.Warnf("error processing event received from remote tagger: %s", err)
			continue
		}

		entity := ev.Entity
		events = append(events, types.EntityEvent{
			EventType: eventType,
			Entity: types.Entity{
				ID:                          types.NewEntityID(types.EntityIDPrefix(entity.Id.Prefix), entity.Id.Uid),
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
func (t *remoteTagger) startTaggerStream(maxElapsed time.Duration) error {
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
			t.log.Infof("unable to fetch auth token, will possibly retry: %s", err)
			return err
		}

		t.streamCtx, t.streamCancel = context.WithCancel(
			metadata.NewOutgoingContext(t.ctx, metadata.MD{
				"authorization": []string{fmt.Sprintf("Bearer %s", token)},
			}),
		)

		prefixes := make([]string, 0)
		for prefix := range t.filter.GetPrefixes() {
			prefixes = append(prefixes, string(prefix))
		}

		t.stream, err = t.client.TaggerStreamEntities(t.streamCtx, &pb.StreamTagsRequest{
			Cardinality: pb.TagCardinality(t.filter.GetCardinality()),
			StreamingID: uuid.New().String(),
			Prefixes:    prefixes,
		})
		if err != nil {
			t.log.Infof("unable to establish stream, will possibly retry: %s", err)
			return err
		}

		t.log.Info("tagger stream established successfully")

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

// TODO(components): verify the grpclog is initialized elsewhere and cleanup
func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())
}
