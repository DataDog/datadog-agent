// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	grpcCredentials "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const (
	defaultClientNameHeaderValue             = "temporal-go"
	defaultClientNameHeaderName              = "client-name"
	defaultClientVersionHeaderName           = "client-version"
	defaultGetHistoryTimeout                 = 65 * time.Second
	defaultGetSystemInfoTimeout              = 5 * time.Second
	defaultKeepAliveTime                     = 30 * time.Second
	defaultKeepAliveTimeout                  = 15 * time.Second
	defaultMaxPayloadSize                    = 128 * 1024 * 1024
	defaultMaxRPCTimeout                     = 10 * time.Second
	defaultMinRPCTimeout                     = 1 * time.Second
	defaultRetryBackoffCoefficient           = 2.0
	defaultRetryExpirationInterval           = 60 * time.Second
	defaultRetryInitialInterval              = 200 * time.Millisecond
	defaultSDKVersion                        = "1.39.0"
	defaultSupportedServerVersionsHeaderName = "supported-server-versions"
	defaultSupportedServerVersionsValue      = ">=1.0.0 <2.0.0"
	defaultTemporalNamespaceHeaderKey        = "temporal-namespace"
	defaultTemporalNamespace                 = "default"
	maxConnectTimeout                        = 20 * time.Second
	pollRetryBackoffMaxInterval              = 10 * time.Second
)

type startWorkflowOptions struct {
	ID        string
	TaskQueue string
}

type temporalTransport interface {
	Close()
	GetWorkflowResult(ctx context.Context, workflowID string, runID string) (string, error)
	ListWorkflowExecutions(ctx context.Context, query string) ([]*workflowpb.WorkflowExecutionInfo, error)
	StartWorkflow(ctx context.Context, options startWorkflowOptions, workflowType string, args ...any) (string, error)
}

type temporalWorkflowService interface {
	DescribeWorkflowExecution(ctx context.Context, in *workflowservice.DescribeWorkflowExecutionRequest, opts ...grpc.CallOption) (*workflowservice.DescribeWorkflowExecutionResponse, error)
	GetSystemInfo(ctx context.Context, in *workflowservice.GetSystemInfoRequest, opts ...grpc.CallOption) (*workflowservice.GetSystemInfoResponse, error)
	GetWorkflowExecutionHistory(ctx context.Context, in *workflowservice.GetWorkflowExecutionHistoryRequest, opts ...grpc.CallOption) (*workflowservice.GetWorkflowExecutionHistoryResponse, error)
	ListWorkflowExecutions(ctx context.Context, in *workflowservice.ListWorkflowExecutionsRequest, opts ...grpc.CallOption) (*workflowservice.ListWorkflowExecutionsResponse, error)
	StartWorkflowExecution(ctx context.Context, in *workflowservice.StartWorkflowExecutionRequest, opts ...grpc.CallOption) (*workflowservice.StartWorkflowExecutionResponse, error)
}

type grpcTemporalTransport struct {
	apiKey         string
	conn           io.Closer
	dataConverter  converter.DataConverter
	identity       string
	namespace      string
	retryInternal  bool
	workflowServer temporalWorkflowService
}

func newTemporalClient(
	ctx context.Context,
	credentials *privateconnection.PrivateCredentials,
	namespace string,
) (temporalTransport, error) {
	connectionConfig, err := getTemporalConnectionConfig(ctx, credentials)
	if err != nil {
		return nil, err
	}

	workflowServer, conn, retryInternal, err := dialTemporalTransport(ctx, connectionConfig)
	if err != nil {
		return nil, err
	}

	return &grpcTemporalTransport{
		apiKey:         connectionConfig.apiKey,
		conn:           conn,
		dataConverter:  converter.GetDefaultDataConverter(),
		identity:       getTemporalIdentity(),
		namespace:      namespace,
		retryInternal:  retryInternal,
		workflowServer: workflowServer,
	}, nil
}

func dialTemporalTransport(
	ctx context.Context,
	connectionConfig temporalConnectionConfig,
) (temporalWorkflowService, *grpc.ClientConn, bool, error) {
	dialOptions := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           newConnectBackoffConfig(),
			MinConnectTimeout: maxConnectTimeout,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(defaultMaxPayloadSize),
			grpc.MaxCallSendMsgSize(defaultMaxPayloadSize),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                defaultKeepAliveTime,
			Timeout:             defaultKeepAliveTimeout,
			PermitWithoutStream: true,
		}),
	}

	if connectionConfig.tls != nil {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(grpcCredentials.NewTLS(connectionConfig.tls)))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(connectionConfig.hostPort, dialOptions...)
	if err != nil {
		return nil, nil, false, err
	}

	workflowServer := workflowservice.NewWorkflowServiceClient(conn)
	retryInternal, err := loadRetryInternalCapability(ctx, workflowServer, connectionConfig.apiKey)
	if err != nil {
		_ = conn.Close()
		return nil, nil, false, err
	}

	return workflowServer, conn, retryInternal, nil
}

func loadRetryInternalCapability(
	ctx context.Context,
	workflowServer temporalWorkflowService,
	apiKey string,
) (bool, error) {
	callCtx, cancel := newTemporalRPCContext(ctx, "", apiKey, false, defaultGetSystemInfoTimeout)
	defer cancel()

	response, err := workflowServer.GetSystemInfo(callCtx, &workflowservice.GetSystemInfoRequest{})
	if err != nil {
		normalizedErr := normalizeTemporalError(err)
		var unimplementedErr *serviceerror.Unimplemented
		if errors.As(normalizedErr, &unimplementedErr) {
			return true, nil
		}
		return false, fmt.Errorf("failed reaching server: %w", normalizedErr)
	}

	capabilities := response.GetCapabilities()
	if capabilities == nil {
		return true, nil
	}
	return !capabilities.GetInternalErrorDifferentiation(), nil
}

func newConnectBackoffConfig() backoff.Config {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.BaseDelay = defaultRetryInitialInterval
	backoffConfig.MaxDelay = pollRetryBackoffMaxInterval
	return backoffConfig
}

func getTemporalIdentity() string {
	hostName, err := os.Hostname()
	if err != nil {
		hostName = "Unknown"
	}
	return fmt.Sprintf("%d@%s@", os.Getpid(), hostName)
}

func (c *grpcTemporalTransport) StartWorkflow(
	ctx context.Context,
	options startWorkflowOptions,
	workflowType string,
	args ...any,
) (string, error) {
	workflowID := options.ID
	if workflowID == "" {
		workflowID = uuid.NewString()
	}

	input, err := c.dataConverter.ToPayloads(args...)
	if err != nil {
		return "", err
	}

	request := &workflowservice.StartWorkflowExecutionRequest{
		Identity:   c.identity,
		Input:      input,
		Namespace:  c.namespace,
		RequestId:  uuid.NewString(),
		TaskQueue:  &taskqueuepb.TaskQueue{Name: options.TaskQueue, Kind: enumspb.TASK_QUEUE_KIND_NORMAL},
		WorkflowId: workflowID,
		WorkflowType: &commonpb.WorkflowType{
			Name: workflowType,
		},
	}

	var response *workflowservice.StartWorkflowExecutionResponse
	err = c.invokeWithRetry(ctx, false, func(callCtx context.Context) error {
		var callErr error
		response, callErr = c.workflowServer.StartWorkflowExecution(callCtx, request)
		return callErr
	})
	if err != nil {
		return "", err
	}

	return response.GetRunId(), nil
}

func (c *grpcTemporalTransport) ListWorkflowExecutions(
	ctx context.Context,
	query string,
) ([]*workflowpb.WorkflowExecutionInfo, error) {
	request := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: c.namespace,
		Query:     query,
	}

	var response *workflowservice.ListWorkflowExecutionsResponse
	err := c.invokeWithRetry(ctx, false, func(callCtx context.Context) error {
		var callErr error
		response, callErr = c.workflowServer.ListWorkflowExecutions(callCtx, request)
		return callErr
	})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}
	return response.GetExecutions(), nil
}

func (c *grpcTemporalTransport) GetWorkflowResult(
	ctx context.Context,
	workflowID string,
	runID string,
) (string, error) {
	currentRunID, err := c.resolveRunID(ctx, workflowID, runID)
	if err != nil {
		return "", err
	}

	for {
		closeEvent, err := c.getCloseEvent(ctx, workflowID, currentRunID)
		if err != nil {
			return "", err
		}

		switch closeEvent.GetEventType() {
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
			attributes := closeEvent.GetWorkflowExecutionCompletedEventAttributes()
			if nextRunID := attributes.GetNewExecutionRunId(); nextRunID != "" {
				currentRunID = nextRunID
				continue
			}

			if attributes.GetResult() == nil {
				return "", nil
			}

			var result string
			if err := c.dataConverter.FromPayloads(attributes.GetResult(), &result); err != nil {
				return "", err
			}
			return result, nil
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
			attributes := closeEvent.GetWorkflowExecutionFailedEventAttributes()
			if nextRunID := attributes.GetNewExecutionRunId(); nextRunID != "" {
				currentRunID = nextRunID
				continue
			}
			return "", fmt.Errorf("workflow execution failed: %s", attributes.GetFailure().GetMessage())
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
			attributes := closeEvent.GetWorkflowExecutionTimedOutEventAttributes()
			if nextRunID := attributes.GetNewExecutionRunId(); nextRunID != "" {
				currentRunID = nextRunID
				continue
			}
			return "", errors.New("workflow execution timed out")
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW:
			nextRunID := closeEvent.GetWorkflowExecutionContinuedAsNewEventAttributes().GetNewExecutionRunId()
			if nextRunID == "" {
				return "", errors.New("workflow continued as new without a new run ID")
			}
			currentRunID = nextRunID
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
			return "", errors.New("workflow execution canceled")
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:
			return "", errors.New("workflow execution terminated")
		default:
			return "", fmt.Errorf("unexpected event type %s when handling workflow execution result", closeEvent.GetEventType())
		}
	}
}

func (c *grpcTemporalTransport) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *grpcTemporalTransport) resolveRunID(
	ctx context.Context,
	workflowID string,
	runID string,
) (string, error) {
	if runID != "" {
		return runID, nil
	}

	request := &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: c.namespace,
		Execution: &commonpb.WorkflowExecution{WorkflowId: workflowID},
	}

	var response *workflowservice.DescribeWorkflowExecutionResponse
	err := c.invokeWithRetry(ctx, false, func(callCtx context.Context) error {
		var callErr error
		response, callErr = c.workflowServer.DescribeWorkflowExecution(callCtx, request)
		return callErr
	})
	if err != nil {
		return "", err
	}
	if response.GetWorkflowExecutionInfo() == nil || response.GetWorkflowExecutionInfo().GetExecution() == nil {
		return "", nil
	}
	return response.GetWorkflowExecutionInfo().GetExecution().GetRunId(), nil
}

func (c *grpcTemporalTransport) getCloseEvent(
	ctx context.Context,
	workflowID string,
	runID string,
) (*historypb.HistoryEvent, error) {
	request := &workflowservice.GetWorkflowExecutionHistoryRequest{
		Execution: &commonpb.WorkflowExecution{
			RunId:      runID,
			WorkflowId: workflowID,
		},
		HistoryEventFilterType: enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT,
		Namespace:              c.namespace,
		SkipArchival:           true,
		WaitNewEvent:           true,
	}

	for {
		var response *workflowservice.GetWorkflowExecutionHistoryResponse
		err := c.invokeWithRetry(ctx, true, func(callCtx context.Context) error {
			var callErr error
			response, callErr = c.workflowServer.GetWorkflowExecutionHistory(callCtx, request)
			return callErr
		})
		if err != nil {
			return nil, err
		}

		history, err := getHistoryFromResponse(response, enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT)
		if err != nil {
			return nil, err
		}
		if len(history.GetEvents()) == 0 && len(response.GetNextPageToken()) != 0 {
			request.NextPageToken = response.GetNextPageToken()
			continue
		}
		if len(history.GetEvents()) == 0 {
			return nil, errors.New("workflow close event not found")
		}
		return history.GetEvents()[len(history.GetEvents())-1], nil
	}
}

func getHistoryFromResponse(
	response *workflowservice.GetWorkflowExecutionHistoryResponse,
	filterType enumspb.HistoryEventFilterType,
) (*historypb.History, error) {
	if response.GetRawHistory() != nil {
		return deserializeBlobDataToHistoryEvents(response.GetRawHistory(), filterType)
	}
	if response.GetHistory() == nil {
		return &historypb.History{}, nil
	}
	return response.GetHistory(), nil
}

func deserializeBlobDataToHistoryEvents(
	dataBlobs []*commonpb.DataBlob,
	filterType enumspb.HistoryEventFilterType,
) (*historypb.History, error) {
	var historyEvents []*historypb.HistoryEvent

	for _, batch := range dataBlobs {
		events, err := deserializeBatchEvents(batch)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			return nil, serviceerror.NewInternal("corrupted history event batch, empty events")
		}
		historyEvents = append(historyEvents, events...)
	}

	if filterType == enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT && len(historyEvents) > 0 {
		historyEvents = []*historypb.HistoryEvent{historyEvents[len(historyEvents)-1]}
	}

	return &historypb.History{Events: historyEvents}, nil
}

func deserializeBatchEvents(data *commonpb.DataBlob) ([]*historypb.HistoryEvent, error) {
	if data == nil || len(data.GetData()) == 0 {
		return nil, nil
	}

	events := &historypb.History{}
	var err error
	switch data.GetEncodingType() {
	case enumspb.ENCODING_TYPE_JSON:
		err = protojson.Unmarshal(data.GetData(), events)
	case enumspb.ENCODING_TYPE_PROTO3:
		err = proto.Unmarshal(data.GetData(), events)
	default:
		return nil, errors.New("DeserializeBatchEvents invalid encoding")
	}
	if err != nil {
		return nil, err
	}
	return events.GetEvents(), nil
}

func (c *grpcTemporalTransport) invokeWithRetry(
	ctx context.Context,
	isLongPoll bool,
	call func(callCtx context.Context) error,
) error {
	retryCtx := ctx
	var retryCancel context.CancelFunc
	if _, hasDeadline := retryCtx.Deadline(); !hasDeadline {
		retryCtx, retryCancel = context.WithDeadline(ctx, time.Now().Add(defaultRetryExpirationInterval))
		defer retryCancel()
	}

	for attempt := 0; ; attempt++ {
		callCtx, cancel := newTemporalRPCContext(retryCtx, c.namespace, c.apiKey, isLongPoll, 0)
		err := call(callCtx)
		cancel()
		if err == nil {
			return nil
		}

		normalizedErr := normalizeTemporalError(err)
		if !shouldRetry(normalizedErr, c.retryInternal) {
			return normalizedErr
		}

		backoffDuration := getRetryBackoff(attempt, retryCtx)
		timer := time.NewTimer(backoffDuration)
		select {
		case <-retryCtx.Done():
			timer.Stop()
			return retryCtx.Err()
		case <-timer.C:
		}
	}
}

func newTemporalRPCContext(
	ctx context.Context,
	namespace string,
	apiKey string,
	isLongPoll bool,
	timeoutOverride time.Duration,
) (context.Context, context.CancelFunc) {
	timeout := getRPCTimeout(ctx)
	if isLongPoll {
		timeout = defaultGetHistoryTimeout
	}
	if timeoutOverride > 0 {
		timeout = timeoutOverride
	}

	ctx = appendOutgoingMetadata(ctx, namespace, apiKey)
	return context.WithTimeout(ctx, timeout)
}

func appendOutgoingMetadata(ctx context.Context, namespace string, apiKey string) context.Context {
	existingMetadata, _ := metadata.FromOutgoingContext(ctx)
	outgoingMetadata := existingMetadata.Copy()
	if outgoingMetadata == nil {
		outgoingMetadata = metadata.New(nil)
	}

	if len(outgoingMetadata.Get(defaultClientNameHeaderName)) == 0 {
		outgoingMetadata.Set(defaultClientNameHeaderName, defaultClientNameHeaderValue)
	}
	if len(outgoingMetadata.Get(defaultClientVersionHeaderName)) == 0 {
		outgoingMetadata.Set(defaultClientVersionHeaderName, defaultSDKVersion)
	}
	if len(outgoingMetadata.Get(defaultSupportedServerVersionsHeaderName)) == 0 {
		outgoingMetadata.Set(defaultSupportedServerVersionsHeaderName, defaultSupportedServerVersionsValue)
	}
	if namespace != "" && len(outgoingMetadata.Get(defaultTemporalNamespaceHeaderKey)) == 0 {
		outgoingMetadata.Set(defaultTemporalNamespaceHeaderKey, namespace)
	}
	if apiKey != "" && len(outgoingMetadata.Get("authorization")) == 0 {
		outgoingMetadata.Set("authorization", "Bearer "+apiKey)
	}

	return metadata.NewOutgoingContext(ctx, outgoingMetadata)
}

func getRPCTimeout(ctx context.Context) time.Duration {
	timeout := defaultMaxRPCTimeout
	now := time.Now()
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline && deadline.After(now) {
		timeout = deadline.Sub(now) / 2
		if timeout < defaultMinRPCTimeout {
			return defaultMinRPCTimeout
		}
		if timeout > defaultMaxRPCTimeout {
			return defaultMaxRPCTimeout
		}
	}
	return timeout
}

func getRetryBackoff(attempt int, ctx context.Context) time.Duration {
	timeout := defaultRetryExpirationInterval
	now := time.Now()
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline && deadline.After(now) {
		timeout = deadline.Sub(now)
	}

	maximumInterval := timeout / 10
	if maximumInterval < defaultRetryInitialInterval {
		maximumInterval = defaultRetryInitialInterval
	}

	nextInterval := float64(defaultRetryInitialInterval) * math.Pow(defaultRetryBackoffCoefficient, float64(attempt))
	return minDuration(time.Duration(nextInterval), maximumInterval)
}

func minDuration(a time.Duration, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func normalizeTemporalError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return serviceerror.FromStatus(status.Convert(err))
}

func shouldRetry(err error, retryInternal bool) bool {
	switch status.Code(err) {
	case codes.Aborted, codes.ResourceExhausted, codes.Unavailable, codes.Unknown:
		return true
	case codes.Internal:
		return retryInternal
	default:
		return false
	}
}
