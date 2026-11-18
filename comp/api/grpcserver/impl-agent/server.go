// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"errors"
	"time"

	googleGrpc "google.golang.org/grpc"
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
	remoteQueriesResolve *remotequeriesimpl.RemoteQueryResolveService
	healthPlatformStore  healthplatformstore.Component
}

// remoteAgentServer implements the dedicated RemoteAgent gRPC service, which owns the remote agent lifecycle
// (registration and refresh) and the reporting of operational events back to the Core Agent.
type remoteAgentServer struct {
	pb.UnimplementedRemoteAgentServer
	remoteAgentRegistry remoteagentregistry.Component
}

// remoteCommandProviderServer implements the RemoteCommandProvider gRPC service, which allows the Core Agent CLI
// to discover and execute commands exposed by registered remote agents.
type remoteCommandProviderServer struct {
	pb.UnimplementedRemoteCommandProviderServer
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
		AgentPID:          in.Pid,
		ProcessInstanceID: in.ProcessInstanceId,
		AgentFlavor:       in.Flavor,
		AgentDisplayName:  in.DisplayName,
		APIEndpointURI:    in.ApiEndpointUri,
		Services:          in.Services,
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
	tags := hosttags.Get(ctx, true, s.configComp, true)
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

// RemoteQueryExecuteStream starts an Agent-local Remote Queries run through a matched
// integration check and answers once the run has started: a final SUCCEEDED event, or
// a terminal error event when the request is rejected before the run starts. The run
// itself continues after the stream closes; the integration uploads bounded JSON page
// files directly to its-agent-intake and reports the outcome through its upload
// session, so the stream never carries progress or result bytes.
func (s *serverSecure) RemoteQueryExecuteStream(req *pb.RemoteQueryExecuteRequest, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	if s.remoteQueries == nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusExecutorUnavailable, "remote query executor is unavailable", stream)
	}

	execReq, err := remoteQueryExecuteRequestFromProto(req)
	if err != nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusInvalidRequest, err.Error(), stream)
	}

	result := s.remoteQueries.ExecuteStream(stream.Context(), execReq)
	if result.Error != nil {
		return remoteQueryExecuteStreamError(result.Error.Code, result.Error.Message, stream)
	}
	if err := stream.Send(&pb.RemoteQueryExecuteChunk{
		Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{Status: result.Status}}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: 1, Final: true})
}

// RemoteQueryResolve resolves an Agent-local Remote Queries target through the
// shared integration matcher without executing SQL: exactly one loaded check match
// answers matched, zero and multiple matches answer target_not_found and
// ambiguous_target, and failures to complete matching answer resolution_error.
// There is no resolve-time binding: execute resolves the target fresh instead of
// revalidating a resolve-time answer. The operation is side-effect-free by
// construction: no query, no result delivery, and no credentials ever cross the
// AgentSecure boundary.
func (s *serverSecure) RemoteQueryResolve(_ context.Context, req *pb.RemoteQueryResolveRequest) (*pb.RemoteQueryResolveResponse, error) {
	if s.remoteQueriesResolve == nil {
		return remoteQueryResolveErrorResponse(remotequeriesimpl.RemoteQueryStatusResolutionError, "remote query resolver is unavailable"), nil
	}
	result := s.remoteQueriesResolve.Resolve(remoteQueryResolveRequestFromProto(req))
	return remoteQueryResolveResponseFromResult(result), nil
}

// remoteQueryResolveRequestFromProto maps the credential-free AgentSecure resolve
// request to the typed resolver input. The mapping is a pure field copy: validation
// and normalization happen inside the resolver, which answers malformed input with
// a typed resolution_error instead of a transport error.
func remoteQueryResolveRequestFromProto(req *pb.RemoteQueryResolveRequest) remotequeriesimpl.RemoteQueryResolveRequest {
	return remotequeriesimpl.RemoteQueryResolveRequest{
		Integration: req.GetIntegration(),
		Target: remotequeriesimpl.RemoteQueryExecuteTarget{
			Host:             req.GetTarget().GetHost(),
			Port:             int(req.GetTarget().GetPort()),
			DBName:           req.GetTarget().GetDbname(),
			DatabaseInstance: req.GetTarget().GetDatabaseInstance(),
		},
	}
}

// remoteQueryResolveResponseFromResult maps the sanitized resolver result to the
// typed AgentSecure response. A matched answer carries the status only; the error
// mirrors the status and never carries credentials or raw integration
// configuration.
func remoteQueryResolveResponseFromResult(result remotequeriesimpl.RemoteQueryResolveResult) *pb.RemoteQueryResolveResponse {
	resp := &pb.RemoteQueryResolveResponse{Status: result.Status}
	if result.Error != nil {
		resp.ErrorCode = result.Error.Code
		resp.ErrorMessage = result.Error.Message
	}
	return resp
}

func remoteQueryResolveErrorResponse(status string, message string) *pb.RemoteQueryResolveResponse {
	return &pb.RemoteQueryResolveResponse{
		Status:       status,
		ErrorCode:    status,
		ErrorMessage: message,
	}
}

func remoteQueryExecuteRequestFromProto(req *pb.RemoteQueryExecuteRequest) (remotequeriesimpl.RemoteQueryExecuteRequest, error) {
	target := remotequeriesimpl.RemoteQueryExecuteTarget{
		Host:             req.GetTarget().GetHost(),
		Port:             int(req.GetTarget().GetPort()),
		DBName:           req.GetTarget().GetDbname(),
		DatabaseInstance: req.GetTarget().GetDatabaseInstance(),
	}
	execReq, err := remotequeriesimpl.NewRemoteQueryExecuteRequest(req.GetIntegration(), target, req.GetQuery(), req.GetIncludeSchema(), remoteQueryResultDeliveryFromProto(req.GetResultDelivery()))
	if err != nil {
		return remotequeriesimpl.RemoteQueryExecuteRequest{}, err
	}
	// The optional trace-continuation metadata is validated fail-open: absent or
	// invalid values leave the request executing exactly as before, and nothing
	// about the delivery handle changes. Only the positive keep priorities 1 and 2
	// survive the bridge — the exact domain the integration's strict carrier
	// validation accepts — so a hand-built request carrying a drop priority (-1,
	// 0) or any other value normalizes to absent instead of becoming a strict
	// request-validation failure in the integration.
	execReq.TraceContext = remotequeriesimpl.NewRemoteQueryTraceContext(
		req.GetTraceContext().GetTraceId(),
		req.GetTraceContext().GetParentId(),
		int(req.GetTraceContext().GetSamplingPriority()),
	)
	return execReq, nil
}

// remoteQueryResultDeliveryFromProto maps the backend-owned identity and limits.
// Neither an upload destination nor credentials cross the AgentSecure boundary;
// the core Agent resolves the destination from local configuration.
func remoteQueryResultDeliveryFromProto(delivery *pb.RemoteQueryResultDelivery) *remotequeriesimpl.RemoteQueryResultDelivery {
	if delivery == nil {
		return nil
	}
	out := &remotequeriesimpl.RemoteQueryResultDelivery{
		RunID:    delivery.GetRunId(),
		TaskID:   delivery.GetTaskId(),
		UploadID: delivery.GetUploadId(),
	}
	if limits := delivery.GetLimits(); limits != nil {
		out.Limits = &remotequeriesimpl.RemoteQueryUploadLimits{
			// Byte limits pass through as int64: truncating to int would silently
			// corrupt the 100 GiB total cap on 32-bit platforms (armhf).
			MaxFileBytes:   limits.GetMaxFileBytes(),
			MaxResultBytes: limits.GetMaxResultBytes(),
			MaxRowBytes:    limits.GetMaxRowBytes(),
			MaxColumns:     int(limits.GetMaxColumns()),
			MaxSchemaBytes: limits.GetMaxSchemaBytes(),
			MaxPages:       int(limits.GetMaxPages()),
			TimeoutMs:      int(limits.GetTimeoutMs()),
		}
	}
	return out
}

func remoteQueryExecuteStreamError(code string, message string, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	if err := stream.Send(&pb.RemoteQueryExecuteChunk{
		Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:    code,
			Message: message,
		}}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: 1, Final: true})
}

// ListCommands returns all commands exposed by registered remote agents that advertise the command provider service.
func (s *remoteCommandProviderServer) ListCommands(ctx context.Context, _ *pb.ListCommandsRequest) (*pb.ListCommandsResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	return &pb.ListCommandsResponse{Providers: s.remoteAgentRegistry.ListCommands(ctx)}, nil
}

// ExecuteCommand routes a command execution request to the selected remote provider and forwards its output frames.
func (s *remoteCommandProviderServer) ExecuteCommand(in *pb.ExecuteCommandRequest, stream googleGrpc.ServerStreamingServer[pb.ExecuteCommandResponse]) error {
	if s.remoteAgentRegistry == nil {
		return status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	if len(in.GetCommandPath()) == 0 {
		return status.Error(codes.InvalidArgument, "command_path is required")
	}

	return s.remoteAgentRegistry.ExecuteCommand(stream.Context(), in, stream.Send)
}
