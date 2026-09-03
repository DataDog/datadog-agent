// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	autodiscoverystream "github.com/DataDog/datadog-agent/comp/core/autodiscovery/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configstreamServer "github.com/DataDog/datadog-agent/comp/core/configstream/server"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerProto "github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterServer "github.com/DataDog/datadog-agent/comp/core/workloadfilter/server"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	dsdReplay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rcservicemrf "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/def"
	remotequeriesimpl "github.com/DataDog/datadog-agent/comp/remotequeries/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type agentServer struct {
	hostname hostnameinterface.Component

	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	taggerServer         *taggerserver.Server
	tagProcessor         option.Option[tagger.Processor]
	workloadmetaServer   *workloadmetaServer.Server
	workloadfilterServer *workloadfilterServer.Server
	configService        option.Option[rcservice.Component]
	configServiceMRF     option.Option[rcservicemrf.Component]
	dogstatsdServer      dogstatsdServer.Component
	capture              dsdReplay.Component
	pidMap               pidmap.Component
	remoteAgentRegistry  remoteagentregistry.Component
	autodiscovery        autodiscovery.Component
	configComp           config.Component
	configStreamServer   *configstreamServer.Server
	remoteQueries        *remotequeriesimpl.RemoteQueryExecuteService
	healthPlatformStore  healthplatformstore.Component
}

// remoteAgentServer implements the dedicated RemoteAgent gRPC service, which owns the remote agent lifecycle
// (registration and refresh) and the reporting of operational events back to the Core Agent.
type remoteAgentServer struct {
	pb.UnimplementedRemoteAgentServer
	remoteAgentRegistry remoteagentregistry.Component
}

func (s *agentServer) GetHostname(ctx context.Context, _ *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := s.hostname.Get(ctx)
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *agentServer) AuthFuncOverride(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) TaggerStreamEntities(req *pb.StreamTagsRequest, srv pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(req, srv)
}

// TaggerGenerateContainerIDFromOriginInfo generates a container ID from the Origin Info.
// This function takes an Origin Info but only uses the ExternalData part of it, this is done for backward compatibility.
func (s *serverSecure) TaggerGenerateContainerIDFromOriginInfo(ctx context.Context, req *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	return s.taggerServer.TaggerGenerateContainerIDFromOriginInfo(ctx, req)
}

func (s *serverSecure) TaggerFetchEntity(ctx context.Context, req *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, req)
}

// DogstatsdCaptureTrigger triggers a dogstatsd traffic capture for the
// duration specified in the request. If a capture is already in progress,
// an error response is sent back.
func (s *serverSecure) DogstatsdCaptureTrigger(_ context.Context, req *pb.CaptureTriggerRequest) (*pb.CaptureTriggerResponse, error) {
	d, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	p, err := s.capture.StartCapture(req.GetPath(), d, req.GetCompressed())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	return &pb.CaptureTriggerResponse{Path: p}, nil
}

// DogstatsdSetTaggerState allows setting a captured tagger state in the
// Tagger facilities. This endpoint is used when traffic replays are in
// progress. An empty state or nil request will result in the Tagger
// capture state being reset to nil.
func (s *serverSecure) DogstatsdSetTaggerState(_ context.Context, req *pb.TaggerState) (*pb.TaggerStateResponse, error) {
	// Reset and return if no state pushed
	if req == nil || req.State == nil {
		log.Debugf("API: empty request or state")
		s.pidMap.SetPidMap(nil)
		return &pb.TaggerStateResponse{Loaded: false}, nil
	}

	tagProcessor, isSet := s.tagProcessor.Get()
	if !isSet || tagProcessor == nil {
		log.Debug("Tag processor is unavailable. Cannot set tagger state.")
		return &pb.TaggerStateResponse{Loaded: false}, errors.New("tag processor is unavailable")
	}

	state := make([]*taggerTypes.TagInfo, 0, len(req.State))

	// better stores these as the native type
	for id, entity := range req.State {
		entityID, err := taggerProto.Pb2TaggerEntityID(entity.Id)
		if err != nil {
			log.Errorf("Error getting identity ID for %v: %v", id, err)
			continue
		}

		state = append(state, &taggerTypes.TagInfo{
			Source:               "replay",
			EntityID:             *entityID,
			HighCardTags:         entity.HighCardinalityTags,
			OrchestratorCardTags: entity.OrchestratorCardinalityTags,
			LowCardTags:          entity.LowCardinalityTags,
			StandardTags:         entity.StandardTags,
			ExpiryDate:           time.Now().Add(time.Duration(req.Duration) * time.Millisecond * 2),
		})
	}

	tagProcessor.ProcessTagInfo(state)
	s.pidMap.SetPidMap(req.PidMap)

	log.Debugf("API: loaded state successfully")

	return &pb.TaggerStateResponse{Loaded: true}, nil
}

var rcNotInitializedErr = status.Error(codes.Unimplemented, "remote configuration service not initialized")
var mrfRcNotInitializedErr = status.Error(codes.Unimplemented, "MRF remote configuration service not initialized")

func (s *serverSecure) ClientGetConfigs(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigGetState()
}

func (s *serverSecure) ClientGetConfigsHA(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigStateHA(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ConfigGetState()
}

func (s *serverSecure) ResetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.ResetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()

	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigResetState()
}

// WorkloadmetaStreamEntities streams entities from the workloadmeta store applying the given filter
func (s *serverSecure) WorkloadmetaStreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

// RegisterRemoteAgent is the AgentSecure copy of the remote agent registration RPC.
//
// Deprecated: this RPC has moved to the dedicated RemoteAgent service. It remains here so existing clients keep working
// and can migrate at their own pace; new clients should use RemoteAgent.RegisterRemoteAgent.
func (s *serverSecure) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	return registerRemoteAgent(s.remoteAgentRegistry, in)
}

// RefreshRemoteAgent is the AgentSecure copy of the remote agent refresh RPC.
//
// Deprecated: this RPC has moved to the dedicated RemoteAgent service. It remains here so existing clients keep working
// and can migrate at their own pace; new clients should use RemoteAgent.RefreshRemoteAgent.
func (s *serverSecure) RefreshRemoteAgent(_ context.Context, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	return refreshRemoteAgent(s.remoteAgentRegistry, in)
}

func (s *remoteAgentServer) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	return registerRemoteAgent(s.remoteAgentRegistry, in)
}

func (s *remoteAgentServer) RefreshRemoteAgent(_ context.Context, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	return refreshRemoteAgent(s.remoteAgentRegistry, in)
}

// ReportRemoteAgentEvent routes operational events reported by a remote agent to the remote agent registry.
func (s *remoteAgentServer) ReportRemoteAgentEvent(_ context.Context, in *pb.ReportRemoteAgentEventRequest) (*pb.ReportRemoteAgentEventResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	events := make([]remoteagentregistry.RemoteAgentEvent, 0, len(in.Events))
	for _, pbEvent := range in.Events {
		event := remoteagentregistry.RemoteAgentEvent{Message: pbEvent.Message}
		switch pbEvent.Details.(type) {
		case *pb.Event_InvalidApiKey:
			event.Details = &remoteagentregistry.InvalidAPIKey{}
		}
		events = append(events, event)
	}

	if err := s.remoteAgentRegistry.ReportRemoteAgentEvent(in.SessionId, events); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &pb.ReportRemoteAgentEventResponse{}, nil
}

// registerRemoteAgent is the shared implementation of the RegisterRemoteAgent RPC, used by both the dedicated
// RemoteAgent service and the deprecated AgentSecure copy so the two cannot drift.
func registerRemoteAgent(registry remoteagentregistry.Component, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	if registry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	registration := &remoteagentregistry.RegistrationData{
		AgentPID:         in.Pid,
		AgentFlavor:      in.Flavor,
		AgentDisplayName: in.DisplayName,
		APIEndpointURI:   in.ApiEndpointUri,
		Services:         in.Services,
	}
	sessionID, recommendedRefreshIntervalSecs, err := registry.RegisterRemoteAgent(registration)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterRemoteAgentResponse{
		RecommendedRefreshIntervalSecs: recommendedRefreshIntervalSecs,
		SessionId:                      sessionID,
	}, nil
}

// refreshRemoteAgent is the shared implementation of the RefreshRemoteAgent RPC, used by both the dedicated
// RemoteAgent service and the deprecated AgentSecure copy so the two cannot drift.
func refreshRemoteAgent(registry remoteagentregistry.Component, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	if registry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	found := registry.RefreshRemoteAgent(in.SessionId)
	if !found {
		return nil, status.Error(codes.NotFound, "no remote agent found with session ID")
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
}

func (s *serverSecure) validateSessionID(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if s.remoteAgentRegistry == nil {
		return status.Error(codes.Unavailable, "remote agent registry not available")
	}
	if found := s.remoteAgentRegistry.RefreshRemoteAgent(sessionID); !found {
		return status.Error(codes.Unauthenticated, "invalid or expired remote agent session")
	}
	return nil
}

func (s *serverSecure) ReportHealthIssue(_ context.Context, in *pb.ReportHealthIssueRequest) (*emptypb.Empty, error) {
	if err := s.validateSessionID(in.GetRemoteAgentSessionId()); err != nil {
		return nil, err
	}

	issue := in.GetIssue()
	if issue == nil {
		return nil, status.Error(codes.InvalidArgument, "issue cannot be nil")
	}
	if issue.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue id cannot be empty")
	}
	if issue.GetIssueName() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue_name cannot be empty")
	}

	if err := s.healthPlatformStore.ReportIssue(issue); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store issue: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *serverSecure) ResolveHealthIssue(_ context.Context, in *pb.ResolveHealthIssueRequest) (*emptypb.Empty, error) {
	if err := s.validateSessionID(in.GetRemoteAgentSessionId()); err != nil {
		return nil, err
	}
	if in.GetIssueId() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue_id cannot be empty")
	}

	s.healthPlatformStore.ResolveIssue(in.GetIssueId())
	return &emptypb.Empty{}, nil
}

func (s *serverSecure) AutodiscoveryStreamConfig(_ *emptypb.Empty, out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	return autodiscoverystream.Config(s.autodiscovery, out)
}

func (s *serverSecure) GetHostTags(ctx context.Context, _ *pb.HostTagRequest) (*pb.HostTagReply, error) {
	tags := hosttags.Get(ctx, true, s.configComp)
	return &pb.HostTagReply{System: tags.System, GoogleCloudPlatform: tags.GoogleCloudPlatform}, nil
}

func (s *serverSecure) StreamConfigEvents(in *pb.ConfigStreamRequest, out pb.AgentSecure_StreamConfigEventsServer) error {
	return s.configStreamServer.StreamConfigEvents(in, out)
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}

func (s *serverSecure) CreateConfigSubscription(stream pb.AgentSecure_CreateConfigSubscriptionServer) error {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return rcNotInitializedErr
	}
	return rcService.CreateConfigSubscription(stream)
}

func (s *serverSecure) WorkloadFilterEvaluate(ctx context.Context, req *pb.WorkloadFilterEvaluateRequest) (*pb.WorkloadFilterEvaluateResponse, error) {
	return s.workloadfilterServer.WorkloadFilterEvaluate(ctx, req)
}

// RemoteQueryExecuteStream executes an Agent-local Remote Queries request through a
// matched integration check. The Agent is a control-plane forwarder: the integration
// uploads bounded JSON page files directly to its-agent-intake, so the stream carries
// only progress metadata, the final compact run receipt, and errors — never bulk
// result bytes.
func (s *serverSecure) RemoteQueryExecuteStream(req *pb.RemoteQueryExecuteRequest, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	if s.remoteQueries == nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusExecutorUnavailable, "remote query executor is unavailable", stream)
	}

	execReq, err := remoteQueryExecuteRequestFromProto(req)
	if err != nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusInvalidRequest, err.Error(), stream)
	}

	forwarder := newRemoteQueryIPCStreamForwarder(stream, req.GetIntegration())
	result := s.remoteQueries.ExecuteStream(stream.Context(), execReq, forwarder.Send)
	if result.Error != nil {
		return remoteQueryExecuteStreamErrorAt(result.Error.Code, result.Error.Message, stream, forwarder.NextChunkIndex())
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: forwarder.NextChunkIndex(), Final: true})
}

// remoteQueryIPCStreamForwarder streams metadata-only events over the secure IPC
// boundary. It owns chunk indexing and appends agent-side timing attributes to the
// final event; there is no data buffering because no bulk bytes ever flow.
type remoteQueryIPCStreamForwarder struct {
	stream      pb.AgentSecure_RemoteQueryExecuteStreamServer
	integration string
	chunkIndex  int32

	start           time.Time
	firstEventAt    time.Time
	sendCalls       uint64
	sendDuration    time.Duration
	maxSendDuration time.Duration
}

func newRemoteQueryIPCStreamForwarder(stream pb.AgentSecure_RemoteQueryExecuteStreamServer, integration string) *remoteQueryIPCStreamForwarder {
	return &remoteQueryIPCStreamForwarder{stream: stream, integration: integration, start: time.Now()}
}

func (f *remoteQueryIPCStreamForwarder) NextChunkIndex() int32 {
	return f.chunkIndex
}

// Send converts one check stream event into a typed proto event and sends it as one chunk.
func (f *remoteQueryIPCStreamForwarder) Send(event check.RemoteQueryStreamEvent) error {
	if f.firstEventAt.IsZero() {
		f.firstEventAt = time.Now()
	}
	protoEvent, err := remoteQueryStreamEventFromCheckEvent(event, f.integration)
	if err != nil {
		return err
	}
	f.addTimingAttributes(protoEvent)
	return f.sendProtoEvent(protoEvent)
}

func (f *remoteQueryIPCStreamForwarder) sendProtoEvent(event *pb.RemoteQueryExecuteStreamEvent) error {
	start := time.Now()
	if err := f.stream.Send(&pb.RemoteQueryExecuteChunk{Event: event, ChunkIndex: f.chunkIndex}); err != nil {
		return err
	}
	duration := time.Since(start)
	f.sendCalls++
	f.sendDuration += duration
	if duration > f.maxSendDuration {
		f.maxSendDuration = duration
	}
	f.chunkIndex++
	return nil
}

func (f *remoteQueryIPCStreamForwarder) addTimingAttributes(event *pb.RemoteQueryExecuteStreamEvent) {
	final := event.GetFinal()
	if final == nil {
		return
	}
	if final.Attributes == nil {
		final.Attributes = map[string]string{}
	}
	elapsed := time.Since(f.start)
	final.Attributes["agent_ipc_send_calls"] = strconv.FormatUint(f.sendCalls+1, 10)
	final.Attributes["agent_first_event_latency_ms"] = formatDurationMillis(f.firstEventAt.Sub(f.start))
	final.Attributes["agent_total_stream_ms"] = formatDurationMillis(elapsed)
	final.Attributes["agent_ipc_send_total_ms"] = formatDurationMillis(f.sendDuration)
	final.Attributes["agent_ipc_send_max_ms"] = formatDurationMillis(f.maxSendDuration)
}

func formatDurationMillis(duration time.Duration) string {
	if duration <= 0 {
		return "0"
	}
	return strconv.FormatFloat(duration.Seconds()*1000, 'f', 3, 64)
}

func remoteQueryExecuteRequestFromProto(req *pb.RemoteQueryExecuteRequest) (remotequeriesimpl.RemoteQueryExecuteRequest, error) {
	target := remotequeriesimpl.RemoteQueryExecuteTarget{
		Host:             req.GetTarget().GetHost(),
		Port:             int(req.GetTarget().GetPort()),
		DBName:           req.GetTarget().GetDbname(),
		DatabaseInstance: req.GetTarget().GetDatabaseInstance(),
	}
	return remotequeriesimpl.NewRemoteQueryExecuteRequest(req.GetIntegration(), target, req.GetQuery(), req.GetIncludeSchema(), remoteQueryResultDeliveryFromProto(req.GetResultDelivery()))
}

// remoteQueryResultDeliveryFromProto maps the backend-injected upload instructions. The
// Agent forwards baseUrl and token opaquely: the intake mints and owns the URL, the token
// is scoped to the upload session, and neither is ever logged.
func remoteQueryResultDeliveryFromProto(delivery *pb.RemoteQueryResultDelivery) *remotequeriesimpl.RemoteQueryResultDelivery {
	if delivery == nil {
		return nil
	}
	out := &remotequeriesimpl.RemoteQueryResultDelivery{
		RunID:           delivery.GetRunId(),
		TaskID:          delivery.GetTaskId(),
		ArtifactVersion: int(delivery.GetArtifactVersion()),
		UploadID:        delivery.GetUploadId(),
		BaseURL:         delivery.GetBaseUrl(),
		Token:           delivery.GetToken(),
		PartBytes:       int(delivery.GetPartBytes()),
	}
	if limits := delivery.GetLimits(); limits != nil {
		out.Limits = &remotequeriesimpl.RemoteQueryUploadLimits{
			MaxFileBytes:   int(limits.GetMaxFileBytes()),
			MaxResultBytes: int(limits.GetMaxResultBytes()),
			MaxRowBytes:    int(limits.GetMaxRowBytes()),
			MaxColumns:     int(limits.GetMaxColumns()),
			MaxSchemaBytes: int(limits.GetMaxSchemaBytes()),
			MaxPages:       int(limits.GetMaxPages()),
			TimeoutMs:      int(limits.GetTimeoutMs()),
		}
	}
	return out
}

// remoteQueryStreamEventFromCheckEvent converts a metadata-only check event into the
// typed proto stream event. The integration name is attached by the Agent from the
// dispatch request. Unknown event types — including any legacy inline data event — fail
// closed so a stale integration cannot smuggle bulk bytes through AgentSecure.
func remoteQueryStreamEventFromCheckEvent(event check.RemoteQueryStreamEvent, integration string) (*pb.RemoteQueryExecuteStreamEvent, error) {
	metadata := map[string]interface{}{}
	if strings.TrimSpace(event.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(event.MetadataJSON), &metadata); err != nil {
			return nil, err
		}
	}
	sequence := uint64FromMetadata(metadata, "sequence")
	out := &pb.RemoteQueryExecuteStreamEvent{Sequence: sequence}
	switch event.Type {
	case "metadata":
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{
			Operation:   stringFromMetadata(metadata, "operation"),
			Integration: integration,
			Attributes:  stringAttributes(metadata, "operation", "sequence"),
		}}
	case "final":
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{
			Status:        stringFromMetadata(metadata, "status"),
			UploadReceipt: uploadReceiptFromMetadata(metadata),
			Attributes:    progressAttributes(metadata, "status", "sequence", "upload_receipt"),
		}}
	case "error":
		errorMetadata := mapFromMetadata(metadata, "error")
		code := stringFromMetadata(errorMetadata, "code")
		if code == "" {
			code = stringFromMetadata(metadata, "code")
		}
		message := stringFromMetadata(errorMetadata, "message")
		if message == "" {
			message = stringFromMetadata(metadata, "message")
		}
		retryable, hasRetryable := boolValueFromMetadata(errorMetadata, "retryable")
		if !hasRetryable {
			retryable = boolFromMetadata(metadata, "retryable")
		}
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:       code,
			Message:    message,
			Retryable:  retryable,
			Attributes: progressAttributes(metadata, "code", "message", "retryable", "error", "sequence"),
		}}
	default:
		return nil, errors.New("unknown remote query stream event type")
	}
	return out, nil
}

func stringFromMetadata(metadata map[string]interface{}, key string) string {
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

func boolFromMetadata(metadata map[string]interface{}, key string) bool {
	v, _ := boolValueFromMetadata(metadata, key)
	return v
}

func boolValueFromMetadata(metadata map[string]interface{}, key string) (bool, bool) {
	if v, ok := metadata[key].(bool); ok {
		return v, true
	}
	return false, false
}

func mapFromMetadata(metadata map[string]interface{}, key string) map[string]interface{} {
	if v, ok := metadata[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

func uint64FromMetadata(metadata map[string]interface{}, keys ...string) uint64 {
	for _, key := range keys {
		switch v := metadata[key].(type) {
		case float64:
			if v > 0 {
				return uint64(v)
			}
		case int:
			if v > 0 {
				return uint64(v)
			}
		case json.Number:
			if n, err := strconv.ParseUint(string(v), 10, 64); err == nil {
				return n
			}
		case string:
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

func int64FromMetadata(metadata map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		switch v := metadata[key].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case json.Number:
			if n, err := strconv.ParseInt(string(v), 10, 64); err == nil {
				return n
			}
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// uploadReceiptFromMetadata parses the compact run receipt carried in the final event
// metadata into the typed proto receipt: exactly uploadId, pageCount, totalRows,
// totalBytes. Returns nil when no receipt is present.
func uploadReceiptFromMetadata(metadata map[string]interface{}) *pb.RemoteQueryUploadReceipt {
	raw, ok := metadata["upload_receipt"].(map[string]interface{})
	if !ok {
		return nil
	}
	return &pb.RemoteQueryUploadReceipt{
		UploadId:   stringFromMetadata(raw, "uploadId"),
		PageCount:  int64FromMetadata(raw, "pageCount"),
		TotalRows:  int64FromMetadata(raw, "totalRows"),
		TotalBytes: int64FromMetadata(raw, "totalBytes"),
	}
}

// stringAttributes maps scalar metadata values into string attributes, skipping the
// excluded keys and any nested objects (the resultDelivery echo never surfaces).
func stringAttributes(metadata map[string]interface{}, exclude ...string) map[string]string {
	excluded := make(map[string]struct{}, len(exclude))
	for _, key := range exclude {
		excluded[key] = struct{}{}
	}
	attrs := make(map[string]string)
	for key, value := range metadata {
		if _, ok := excluded[key]; ok {
			continue
		}
		switch v := value.(type) {
		case string:
			attrs[key] = v
		case float64:
			attrs[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			attrs[key] = strconv.FormatBool(v)
		}
	}
	return attrs
}

// progressAttributes extends stringAttributes with the flattened run progress stats the
// integration reports (rowsEmitted, pagesEmitted, partsEmitted, bytesEmitted,
// elapsedMs). The stats are compact counters, never bulk result bytes.
func progressAttributes(metadata map[string]interface{}, exclude ...string) map[string]string {
	attrs := stringAttributes(metadata, exclude...)
	if stats, ok := metadata["stats"].(map[string]interface{}); ok {
		for key, value := range stats {
			switch v := value.(type) {
			case string:
				attrs["stats."+key] = v
			case float64:
				attrs["stats."+key] = strconv.FormatFloat(v, 'f', -1, 64)
			case bool:
				attrs["stats."+key] = strconv.FormatBool(v)
			}
		}
	}
	return attrs
}

func remoteQueryExecuteStreamError(code string, message string, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	return remoteQueryExecuteStreamErrorAt(code, message, stream, 0)
}

func remoteQueryExecuteStreamErrorAt(code string, message string, stream pb.AgentSecure_RemoteQueryExecuteStreamServer, chunkIndex int32) error {
	if err := stream.Send(&pb.RemoteQueryExecuteChunk{
		ChunkIndex: chunkIndex,
		Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:    code,
			Message: message,
		}}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: chunkIndex + 1, Final: true})
}
