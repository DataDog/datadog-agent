// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteimpl implements a remote Tagger.
package remoteimpl

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/rss"
)

const (
	noTimeout         = 0 * time.Minute
	streamRecvTimeout = 10 * time.Minute
	cacheExpiration   = 1 * time.Minute
)

var (
	errTaggerStreamNotStarted                        = errors.New("tagger stream not started")
	errTaggerFailedGenerateContainerIDFromOriginInfo = errors.New("tagger failed to generate container ID from origin info")
)

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

	Comp     tagger.Component
	Endpoint api.AgentEndpointProvider
}

type remoteTagger struct {
	store   *tagStore
	ready   bool
	options Options

	cfg config.Component
	log log.Component

	// conn   *grpc.ClientConn
	token  string
	client *httputils.HttpStream

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
	rss.Before("remoteTagger")
	defer rss.After("remoteTagger")
	remoteTagger, err := newRemoteTagger(req.Params, req.Config, req.Log, req.Telemetry)

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
		Comp:     remoteTagger,
		Endpoint: api.NewAgentEndpointProvider(remoteTagger.writeList, "/tagger-list", "GET"),
	}, nil
}

func newRemoteTagger(params tagger.RemoteParams, cfg config.Component, log log.Component, telemetryComp coretelemetry.Component) (*remoteTagger, error) {
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

	token, err := t.options.TokenFetcher()
	if err != nil {
		t.log.Infof("unable to fetch auth token: %s", err)
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("https://localhost%v/v1/grpc/tagger/stream_entities", t.options.Target)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		t.log.Warnf("Failed to create request: %v", err)
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	t.client = httputils.NewStream(client, req)

	t.log.Info("remote tagger initialized successfully")

	go t.run()

	return nil
}

// Stop closes the connection to the remote tagger and stops event collection.
func (t *remoteTagger) Stop() error {
	t.cancel()

	t.client.Close()

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
	prefix, id, err := types.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// GenerateContainerIDFromOriginInfo returns a container ID for the given Origin Info.
func (t *remoteTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	fail := true
	defer func() {
		if fail {
			t.telemetryStore.OriginInfoRequests.Inc("failed")
		} else {
			t.telemetryStore.OriginInfoRequests.Inc("success")
		}
	}()

	key := cache.BuildAgentKey(
		"remoteTagger",
		"originInfo",
		origindetection.OriginInfoString(originInfo),
	)

	cachedContainerID, err := cache.GetWithExpiration(key, func() (containerID string, err error) {
		return "", err
	}, cacheExpiration)

	if err != nil {
		return "", err
	}
	fail = false
	return cachedContainerID, nil
}

// // queryContainerIDFromOriginInfo calls the local tagger to get the container ID from the Origin Info.
// func (t *remoteTagger) queryContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (containerID string, err error) {
// 	expBackoff := backoff.NewExponentialBackOff()
// 	expBackoff.InitialInterval = 200 * time.Millisecond
// 	expBackoff.MaxInterval = 1 * time.Second
// 	expBackoff.MaxElapsedTime = 15 * time.Second

// 	err = backoff.Retry(func() error {
// 		select {
// 		case <-t.ctx.Done():
// 			return &backoff.PermanentError{Err: errTaggerFailedGenerateContainerIDFromOriginInfo}
// 		default:
// 		}

// 		// Fetch the auth token
// 		if t.token == "" {
// 			var authError error
// 			t.token, authError = t.options.TokenFetcher()
// 			if authError != nil {
// 				_ = t.log.Errorf("unable to fetch auth token, will possibly retry: %s", authError)
// 				return authError
// 			}
// 		}

// 		// Create the context with the auth token
// 		queryCtx, queryCancel := context.WithCancel(
// 			metadata.NewOutgoingContext(t.ctx, metadata.MD{
// 				"authorization": []string{fmt.Sprintf("Bearer %s", t.token)},
// 			}),
// 		)
// 		defer queryCancel()

// 		// Call the gRPC method to get the container ID from the origin info
// 		containerIDResponse, err := t.client.TaggerGenerateContainerIDFromOriginInfo(queryCtx, &pb.GenerateContainerIDFromOriginInfoRequest{
// 			ExternalData: &pb.GenerateContainerIDFromOriginInfoRequest_ExternalData{
// 				Init:          &originInfo.ExternalData.Init,
// 				ContainerName: &originInfo.ExternalData.ContainerName,
// 				PodUID:        &originInfo.ExternalData.PodUID,
// 			},
// 		})
// 		if err != nil {
// 			_ = t.log.Errorf("unable to generate container ID from origin info, will retry: %s", err)
// 			return err
// 		}

// 		if containerIDResponse == nil {
// 			_ = t.log.Warnf("unable to generate container ID from origin info, will retry: %s", err)
// 			return errors.New("containerIDResponse is nil")
// 		}
// 		containerID = containerIDResponse.ContainerID

// 		t.log.Debugf("Container ID generated successfully from origin info %+v: %s", originInfo, containerID)
// 		return nil
// 	}, expBackoff)

// 	return containerID, err
// }

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
	return t.Tag(types.GetGlobalEntityID(), cardinality)
}

func (t *remoteTagger) SetNewCaptureTagger(tagger.Component) {}

func (t *remoteTagger) ResetCaptureTagger() {}

// EnrichTags enriches the tags with the global tags.
// Agents running the remote tagger don't have the ability to enrich tags based
// on the origin info. Only the core agent or dogstatsd can have origin info,
// and they always use the local tagger.
// This function can only add the global tags.
func (t *remoteTagger) EnrichTags(tb tagset.TagsAccumulator, _ taggertypes.OriginInfo) {
	if err := t.AccumulateTagsFor(types.GetGlobalEntityID(), t.dogstatsdCardinality, tb); err != nil {
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

type id struct {
	Prefix string `json:"prefix,omitempty"`
	Uid    string `json:"uid,omitempty"`
}

type entity struct {
	Id                          id       `json:"id,omitempty"`
	Hash                        string   `json:"hash,omitempty"`
	HighCardinalityTags         []string `json:"highCardinalityTags,omitempty"`
	OrchestratorCardinalityTags []string `json:"orchestratorCardinalityTags,omitempty"`
	LowCardinalityTags          []string `json:"lowCardinalityTags,omitempty"`
	StandardTags                []string `json:"standardTags,omitempty"`
}

type taggerEvent struct {
	Type   string `json:"type,omitempty"`
	Entity entity `json:"entity,omitempty"`
}

type results struct {
	Results map[string][]taggerEvent `json:"result,omitempty"`
}

func (t *remoteTagger) run() {
	go func() {
		for {
			select {
			case <-t.telemetryTicker.C:
				t.store.collectTelemetry()
				continue
			case <-t.ctx.Done():
				return
			default:
			}
		}
	}()
	t.client.Connect()
	for {
		select {
		case data := <-t.client.Data:
			results := results{}
			err := json.Unmarshal(data, &results)
			if err != nil {
				t.log.Warnf("Failed to parse json: %v", err)
				break
			}

			t.telemetryStore.Receives.Inc()
			t.log.Debugf("Got tagger information: %+v", results)
			err = t.processResponse(results)
			if err != nil {
				t.log.Warnf("error processing event received from remote tagger: %s", err)
				continue
			}
		case clientErr := <-t.client.Error:
			t.log.Warnf("Error from remote tagger: %v", clientErr)
		case <-t.client.Exit:
			t.log.Debug("Tagger Stream closed.")
			return
		}
	}
}

func (t *remoteTagger) processResponse(results results) error {
	// returning early when there are no events prevents a keep-alive sent
	// from the core agent from wiping the store clean in case the remote
	// tagger was previously in an unready (but filled) state.
	if len(results.Results) == 0 {
		return nil
	}
	var events []types.EntityEvent

	for _, resultEvents := range results.Results {
		events = make([]types.EntityEvent, 0, len(resultEvents))
		for _, ev := range resultEvents {
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
	}

	// // if the tagger was not ready by this point, it means an error
	// // occurred and the contents of the store are no longer valid and need
	// // to be replaced by the batch coming from the current response
	replaceStoreContents := !t.ready

	err := t.store.processEvents(events, replaceStoreContents)
	if err != nil {
		return err
	}

	t.ready = true

	return nil
}

func (t *remoteTagger) writeList(w http.ResponseWriter, _ *http.Request) {
	response := t.List()

	jsonTags, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, t.log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

func convertEventType(t string) (types.EventType, error) {
	switch t {
	case "ADDED":
		return types.EventTypeAdded, nil
	case "MODIFIED":
		return types.EventTypeModified, nil
	case "DELETED":
		return types.EventTypeDeleted, nil
	}

	return types.EventTypeAdded, fmt.Errorf("unknown event type: %q", t)
}

// TODO(components): verify the grpclog is initialized elsewhere and cleanup
// func init() {
// 	grpclog.SetLoggerV2(grpcutil.NewLogger())
// }
