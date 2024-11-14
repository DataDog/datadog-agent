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
	"strconv"
	"strings"
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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// External Data Prefixes
	// These prefixes are used to build the External Data Environment Variable.
	// This variable is then used for Origin Detection.
	externalDataInitPrefix          = "it-"
	externalDataContainerNamePrefix = "cn-"
	externalDataPodUIDPrefix        = "pu-"
)

type externalData struct {
	init          bool
	containerName string
	podUID        string
}

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
	Wmeta     optional.Option[workloadmeta.Component]
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

	cfg   config.Component
	log   log.Component
	wmeta optional.Option[workloadmeta.Component]

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

	datadogConfig taggercommon.DatadogConfig

	checksCardinality          types.TagCardinality
	dogstatsdCardinality       types.TagCardinality
	tlmUDPOriginDetectionError coretelemetry.Counter
}

// Options contains the options needed to configure the remote tagger.
type Options struct {
	Target       string
	TokenFetcher func() (string, error)
	Disabled     bool
}

// NewComponent returns a remote tagger
func NewComponent(req Requires) (Provides, error) {
	remoteTagger, err := NewRemoteTagger(req.Params, req.Config, req.Log, req.Telemetry, req.Wmeta)

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
func NewRemoteTagger(params tagger.RemoteParams, cfg config.Component, log log.Component, telemetryComp coretelemetry.Component, wmeta optional.Option[workloadmeta.Component]) (tagger.Component, error) {
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
		store:          newTagStore(cfg, telemetryStore),
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

	remotetagger.datadogConfig.DogstatsdEntityIDPrecedenceEnabled = cfg.GetBool("dogstatsd_entity_id_precedence")
	remotetagger.datadogConfig.OriginDetectionUnifiedEnabled = cfg.GetBool("origin_detection_unified")
	remotetagger.datadogConfig.DogstatsdOptOutEnabled = cfg.GetBool("dogstatsd_origin_optout_enabled")
	// we use to pull tagger metrics in dogstatsd. Pulling it later in the
	// pipeline improve memory allocation. We kept the old name to be
	// backward compatible and because origin detection only affect
	// dogstatsd metrics.
	remotetagger.tlmUDPOriginDetectionError = telemetryComp.NewCounter("dogstatsd", "udp_origin_detection_error", nil, "Dogstatsd UDP origin detection error count")
	remotetagger.wmeta = wmeta

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

	timeout := time.Duration(t.cfg.GetInt("remote_tagger_timeout_seconds")) * time.Second
	err = t.startTaggerStream(timeout)
	if err != nil {
		// tagger stopped before being connected
		if errors.Is(err, errTaggerStreamNotStarted) {
			return nil
		}
		return err
	}

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

func (t *remoteTagger) AgentTags(cardinality types.TagCardinality) ([]string, error) {
	ctrID, err := metrics.GetProvider(t.wmeta).GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return nil, err
	}

	if ctrID == "" {
		return nil, nil
	}

	entityID := types.NewEntityID(types.ContainerID, ctrID)
	return t.Tag(entityID, cardinality)
}

func (t *remoteTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return t.Tag(taggercommon.GetGlobalEntityID(), cardinality)
}

func (t *remoteTagger) SetNewCaptureTagger(tagger.Component) {}

func (t *remoteTagger) ResetCaptureTagger() {}

func (t *remoteTagger) EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	cardinality := taggerCardinality(originInfo.Cardinality, t.dogstatsdCardinality, t.log)

	productOrigin := originInfo.ProductOrigin
	// If origin_detection_unified is disabled, we use DogStatsD's Legacy Origin Detection.
	// TODO: remove this when origin_detection_unified is enabled by default
	if !t.datadogConfig.OriginDetectionUnifiedEnabled && productOrigin == taggertypes.ProductOriginDogStatsD {
		productOrigin = taggertypes.ProductOriginDogStatsDLegacy
	}

	containerIDFromSocketCutIndex := len(types.ContainerID) + types.GetSeparatorLengh()

	switch productOrigin {
	case taggertypes.ProductOriginDogStatsDLegacy:
		// The following was moved from the dogstatsd package
		// originFromUDS is the origin discovered via UDS origin detection (container ID).
		// originFromTag is the origin sent by the client via the dd.internal.entity_id tag (non-prefixed pod uid).
		// originFromMsg is the origin sent by the client via the container field (non-prefixed container ID).
		// entityIDPrecedenceEnabled refers to the dogstatsd_entity_id_precedence parameter.
		//
		//	---------------------------------------------------------------------------------
		//
		// | originFromUDS | originFromTag | entityIDPrecedenceEnabled || Result: udsOrigin  |
		// |---------------|---------------|---------------------------||--------------------|
		// | any           | any           | false                     || originFromUDS      |
		// | any           | any           | true                      || empty              |
		// | any           | empty         | any                       || originFromUDS      |
		//
		//	---------------------------------------------------------------------------------
		//
		//	---------------------------------------------------------------------------------
		//
		// | originFromTag          | originFromMsg   || Result: originFromClient            |
		// |------------------------|-----------------||-------------------------------------|
		// | not empty && not none  | any             || pod prefix + originFromTag          |
		// | empty                  | empty           || empty                               |
		// | none                   | empty           || empty                               |
		// | empty                  | not empty       || container prefix + originFromMsg    |
		// | none                   | not empty       || container prefix + originFromMsg    |
		if t.datadogConfig.DogstatsdOptOutEnabled && originInfo.Cardinality == "none" {
			originInfo.ContainerIDFromSocket = packets.NoOrigin
			originInfo.PodUID = ""
			originInfo.ContainerID = ""
			return
		}

		// We use the UDS socket origin if no origin ID was specify in the tags
		// or 'dogstatsd_entity_id_precedence' is set to False (default false).
		if originInfo.ContainerIDFromSocket != packets.NoOrigin &&
			(originInfo.PodUID == "" || !t.datadogConfig.DogstatsdEntityIDPrecedenceEnabled) &&
			len(originInfo.ContainerIDFromSocket) > containerIDFromSocketCutIndex {
			containerID := originInfo.ContainerIDFromSocket[containerIDFromSocketCutIndex:]
			originFromClient := types.NewEntityID(types.ContainerID, containerID)
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.log.Errorf("%s", err.Error())
			}
		}

		// originFromClient can either be originInfo.FromTag or originInfo.FromMsg
		var originFromClient types.EntityID
		if originInfo.PodUID != "" && originInfo.PodUID != "none" {
			// Check if the value is not "none" in order to avoid calling the tagger for entity that doesn't exist.
			// Currently only supported for pods
			originFromClient = types.NewEntityID(types.KubernetesPodUID, originInfo.PodUID)
		} else if originInfo.PodUID == "" && len(originInfo.ContainerID) > 0 {
			// originInfo.FromMsg is the container ID sent by the newer clients.
			originFromClient = types.NewEntityID(types.ContainerID, originInfo.ContainerID)
		}

		if !originFromClient.Empty() {
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.tlmUDPOriginDetectionError.Inc()
				t.log.Tracef("Cannot get tags for entity %s: %s", originFromClient, err)
			}
		}
	default:
		// Tag using Local Data
		if originInfo.ContainerIDFromSocket != packets.NoOrigin && len(originInfo.ContainerIDFromSocket) > containerIDFromSocketCutIndex {
			containerID := originInfo.ContainerIDFromSocket[containerIDFromSocketCutIndex:]
			originFromClient := types.NewEntityID(types.ContainerID, containerID)
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.log.Errorf("%s", err.Error())
			}
		}

		if err := t.AccumulateTagsFor(types.NewEntityID(types.ContainerID, originInfo.ContainerID), cardinality, tb); err != nil {
			t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.ContainerID, err)
		}

		if err := t.AccumulateTagsFor(types.NewEntityID(types.KubernetesPodUID, originInfo.PodUID), cardinality, tb); err != nil {
			t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.PodUID, err)
		}

		// Tag using External Data.
		// External Data is a list that contain prefixed-items, split by a ','. Current items are:
		// * "it-<init>" if the container is an init container.
		// * "cn-<container-name>" for the container name.
		// * "pu-<pod-uid>" for the pod UID.
		// Order does not matter.
		// Possible values:
		// * "it-false,cn-nginx,pu-3413883c-ac60-44ab-96e0-9e52e4e173e2"
		// * "cn-init,pu-cb4aba1d-0129-44f1-9f1b-b4dc5d29a3b3,it-true"
		if originInfo.ExternalData != "" {
			// Parse the external data and get the tags for the entity
			var parsedExternalData externalData
			var initParsingError error
			for _, item := range strings.Split(originInfo.ExternalData, ",") {
				switch {
				case strings.HasPrefix(item, externalDataInitPrefix):
					parsedExternalData.init, initParsingError = strconv.ParseBool(item[len(externalDataInitPrefix):])
					if initParsingError != nil {
						t.log.Tracef("Cannot parse bool from %s: %s", item[len(externalDataInitPrefix):], initParsingError)
					}
				case strings.HasPrefix(item, externalDataContainerNamePrefix):
					parsedExternalData.containerName = item[len(externalDataContainerNamePrefix):]
				case strings.HasPrefix(item, externalDataPodUIDPrefix):
					parsedExternalData.podUID = item[len(externalDataPodUIDPrefix):]
				}
			}

			// Accumulate tags for pod UID
			if parsedExternalData.podUID != "" {
				if err := t.AccumulateTagsFor(types.NewEntityID(types.KubernetesPodUID, parsedExternalData.podUID), cardinality, tb); err != nil {
					t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.ContainerID, err)
				}
			}

			// Generate container ID from External Data
			generatedContainerID, err := t.generateContainerIDFromExternalData(parsedExternalData, metrics.GetProvider(t.wmeta).GetMetaCollector())
			if err != nil {
				t.log.Tracef("Failed to generate container ID from %s: %s", originInfo.ExternalData, err)
			}

			// Accumulate tags for generated container ID
			if generatedContainerID != "" {
				if err := t.AccumulateTagsFor(types.NewEntityID(types.ContainerID, generatedContainerID), cardinality, tb); err != nil {
					t.log.Tracef("Cannot get tags for entity %s: %s", generatedContainerID, err)
				}
			}
		}
	}

	if err := t.AccumulateTagsFor(taggercommon.GetGlobalEntityID(), cardinality, tb); err != nil {
		t.log.Error(err.Error())
	}
}

// generateContainerIDFromExternalData generates a container ID from the external data
func (t *remoteTagger) generateContainerIDFromExternalData(e externalData, metricsProvider provider.ContainerIDForPodUIDAndContNameRetriever) (string, error) {
	return metricsProvider.ContainerIDForPodUIDAndContName(e.podUID, e.containerName, e.init, time.Second)
}

// taggerCardinality converts tagger cardinality string to types.TagCardinality
// It should be defaulted to DogstatsdCardinality if the string is empty or unknown
func taggerCardinality(cardinality string,
	defaultCardinality types.TagCardinality,
	l log.Component) types.TagCardinality {
	if cardinality == "" {
		return defaultCardinality
	}

	taggerCardinality, err := types.StringToTagCardinality(cardinality)
	if err != nil {
		l.Tracef("Couldn't convert cardinality tag: %v", err)
		return defaultCardinality
	}

	return taggerCardinality
}

func (t *remoteTagger) ChecksCardinality() types.TagCardinality {
	return t.checksCardinality
}

func (t *remoteTagger) DogstatsdCardinality() types.TagCardinality {
	return t.dogstatsdCardinality
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *remoteTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.store.subscribe(subscriptionID, filter)
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
