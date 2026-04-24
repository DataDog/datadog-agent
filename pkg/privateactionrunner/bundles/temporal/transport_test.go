// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type fakeTemporalWorkflowService struct {
	describeWorkflowExecutionFn func(context.Context, *workflowservice.DescribeWorkflowExecutionRequest, ...grpc.CallOption) (*workflowservice.DescribeWorkflowExecutionResponse, error)
	getSystemInfoFn             func(context.Context, *workflowservice.GetSystemInfoRequest, ...grpc.CallOption) (*workflowservice.GetSystemInfoResponse, error)
	getWorkflowExecutionFn      func(context.Context, *workflowservice.GetWorkflowExecutionHistoryRequest, ...grpc.CallOption) (*workflowservice.GetWorkflowExecutionHistoryResponse, error)
	listWorkflowExecutionsFn    func(context.Context, *workflowservice.ListWorkflowExecutionsRequest, ...grpc.CallOption) (*workflowservice.ListWorkflowExecutionsResponse, error)
	startWorkflowExecutionFn    func(context.Context, *workflowservice.StartWorkflowExecutionRequest, ...grpc.CallOption) (*workflowservice.StartWorkflowExecutionResponse, error)
}

func (f *fakeTemporalWorkflowService) DescribeWorkflowExecution(
	ctx context.Context,
	in *workflowservice.DescribeWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	if f.describeWorkflowExecutionFn == nil {
		return &workflowservice.DescribeWorkflowExecutionResponse{}, nil
	}
	return f.describeWorkflowExecutionFn(ctx, in, opts...)
}

func (f *fakeTemporalWorkflowService) GetSystemInfo(
	ctx context.Context,
	in *workflowservice.GetSystemInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSystemInfoResponse, error) {
	if f.getSystemInfoFn == nil {
		return &workflowservice.GetSystemInfoResponse{}, nil
	}
	return f.getSystemInfoFn(ctx, in, opts...)
}

func (f *fakeTemporalWorkflowService) GetWorkflowExecutionHistory(
	ctx context.Context,
	in *workflowservice.GetWorkflowExecutionHistoryRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	if f.getWorkflowExecutionFn == nil {
		return &workflowservice.GetWorkflowExecutionHistoryResponse{}, nil
	}
	return f.getWorkflowExecutionFn(ctx, in, opts...)
}

func (f *fakeTemporalWorkflowService) ListWorkflowExecutions(
	ctx context.Context,
	in *workflowservice.ListWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	if f.listWorkflowExecutionsFn == nil {
		return &workflowservice.ListWorkflowExecutionsResponse{}, nil
	}
	return f.listWorkflowExecutionsFn(ctx, in, opts...)
}

func (f *fakeTemporalWorkflowService) StartWorkflowExecution(
	ctx context.Context,
	in *workflowservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StartWorkflowExecutionResponse, error) {
	if f.startWorkflowExecutionFn == nil {
		return &workflowservice.StartWorkflowExecutionResponse{}, nil
	}
	return f.startWorkflowExecutionFn(ctx, in, opts...)
}

func TestGetTemporalConnectionConfigAddressOnly(t *testing.T) {
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "address", Value: "localhost:7233"},
		},
	}

	config, err := getTemporalConnectionConfig(context.Background(), credentials)

	require.NoError(t, err)
	assert.Equal(t, "localhost:7233", config.hostPort)
	assert.Empty(t, config.apiKey)
	assert.Nil(t, config.tls)
}

func TestGetTemporalConnectionConfigTLSAndAPIKey(t *testing.T) {
	rootCA := mustCreateCertificatePEM(t)
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "serverAddress", Value: "temporal.example.com:7233"},
			{Name: "serverNameOverride", Value: "temporal.internal"},
			{Name: "serverRootCACertificate", Value: rootCA},
			{Name: "apiKey", Value: "secret-key"},
		},
	}

	config, err := getTemporalConnectionConfig(context.Background(), credentials)

	require.NoError(t, err)
	require.NotNil(t, config.tls)
	assert.Equal(t, "temporal.example.com:7233", config.hostPort)
	assert.Equal(t, "secret-key", config.apiKey)
	assert.Equal(t, uint16(tls.VersionTLS12), config.tls.MinVersion)
	assert.Equal(t, "temporal.internal", config.tls.ServerName)
	assert.NotNil(t, config.tls.RootCAs)
}

func TestGetTemporalConnectionConfigMTLS(t *testing.T) {
	certPEM, keyPEM := mustCreateCertificatePairPEM(t)
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "serverAddress", Value: "temporal.example.com:7233"},
			{Name: "clientCertPairCrt", Value: certPEM},
			{Name: "clientCertPairKey", Value: keyPEM},
		},
	}

	config, err := getTemporalConnectionConfig(context.Background(), credentials)

	require.NoError(t, err)
	require.NotNil(t, config.tls)
	require.Len(t, config.tls.Certificates, 1)
}

func TestGetTemporalConnectionConfigMTLSRequiresKeyAndCert(t *testing.T) {
	certPEM, _ := mustCreateCertificatePairPEM(t)
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "serverAddress", Value: "temporal.example.com:7233"},
			{Name: "clientCertPairCrt", Value: certPEM},
		},
	}

	_, err := getTemporalConnectionConfig(context.Background(), credentials)

	require.ErrorContains(t, err, "ensure both the client certificate and client key are provided")
}

func TestStartWorkflowBuildsRequestAndMetadata(t *testing.T) {
	var capturedMetadata metadata.MD
	var capturedRequest *workflowservice.StartWorkflowExecutionRequest
	transport := &grpcTemporalTransport{
		apiKey:        "secret-key",
		dataConverter: converter.GetDefaultDataConverter(),
		identity:      "test-identity",
		namespace:     "test-namespace",
		workflowServer: &fakeTemporalWorkflowService{
			startWorkflowExecutionFn: func(ctx context.Context, in *workflowservice.StartWorkflowExecutionRequest, _ ...grpc.CallOption) (*workflowservice.StartWorkflowExecutionResponse, error) {
				capturedRequest = in
				capturedMetadata, _ = metadata.FromOutgoingContext(ctx)
				return &workflowservice.StartWorkflowExecutionResponse{RunId: "run-123"}, nil
			},
		},
	}

	runID, err := transport.StartWorkflow(context.Background(), startWorkflowOptions{TaskQueue: "task-queue"}, "workflow-name", "arg-one", 42)

	require.NoError(t, err)
	assert.Equal(t, "run-123", runID)
	require.NotNil(t, capturedRequest)
	assert.Equal(t, "test-namespace", capturedRequest.GetNamespace())
	assert.Equal(t, "workflow-name", capturedRequest.GetWorkflowType().GetName())
	assert.Equal(t, "task-queue", capturedRequest.GetTaskQueue().GetName())
	assert.Equal(t, enumspb.TASK_QUEUE_KIND_NORMAL, capturedRequest.GetTaskQueue().GetKind())
	assert.Equal(t, "test-identity", capturedRequest.GetIdentity())
	assert.NotEmpty(t, capturedRequest.GetRequestId())
	_, err = uuid.Parse(capturedRequest.GetWorkflowId())
	require.NoError(t, err)
	require.Len(t, capturedMetadata.Get("authorization"), 1)
	assert.Equal(t, "Bearer secret-key", capturedMetadata.Get("authorization")[0])
	require.Len(t, capturedMetadata.Get(defaultTemporalNamespaceHeaderKey), 1)
	assert.Equal(t, "test-namespace", capturedMetadata.Get(defaultTemporalNamespaceHeaderKey)[0])
	require.Len(t, capturedMetadata.Get(defaultClientNameHeaderName), 1)
	assert.Equal(t, defaultClientNameHeaderValue, capturedMetadata.Get(defaultClientNameHeaderName)[0])

	var argOne string
	var argTwo int
	err = transport.dataConverter.FromPayloads(capturedRequest.GetInput(), &argOne, &argTwo)
	require.NoError(t, err)
	assert.Equal(t, "arg-one", argOne)
	assert.Equal(t, 42, argTwo)
}

func TestListWorkflowExecutionsBuildsRequest(t *testing.T) {
	var capturedRequest *workflowservice.ListWorkflowExecutionsRequest
	transport := &grpcTemporalTransport{
		namespace: "test-namespace",
		workflowServer: &fakeTemporalWorkflowService{
			listWorkflowExecutionsFn: func(_ context.Context, in *workflowservice.ListWorkflowExecutionsRequest, _ ...grpc.CallOption) (*workflowservice.ListWorkflowExecutionsResponse, error) {
				capturedRequest = in
				return &workflowservice.ListWorkflowExecutionsResponse{
					Executions: []*workflowpb.WorkflowExecutionInfo{{}},
				}, nil
			},
		},
	}

	workflows, err := transport.ListWorkflowExecutions(context.Background(), "ExecutionStatus = 'Running'")

	require.NoError(t, err)
	require.Len(t, workflows, 1)
	require.NotNil(t, capturedRequest)
	assert.Equal(t, "test-namespace", capturedRequest.GetNamespace())
	assert.Equal(t, "ExecutionStatus = 'Running'", capturedRequest.GetQuery())
}

func TestGetWorkflowResultResolvesRunIDAndFollowsContinueAsNew(t *testing.T) {
	transport := &grpcTemporalTransport{
		dataConverter: converter.GetDefaultDataConverter(),
		namespace:     "test-namespace",
		workflowServer: &fakeTemporalWorkflowService{
			describeWorkflowExecutionFn: func(_ context.Context, in *workflowservice.DescribeWorkflowExecutionRequest, _ ...grpc.CallOption) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
				assert.Equal(t, "test-namespace", in.GetNamespace())
				assert.Equal(t, "workflow-id", in.GetExecution().GetWorkflowId())
				return &workflowservice.DescribeWorkflowExecutionResponse{
					WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
						Execution: &commonpb.WorkflowExecution{RunId: "run-1"},
					},
				}, nil
			},
			getWorkflowExecutionFn: func(_ context.Context, in *workflowservice.GetWorkflowExecutionHistoryRequest, _ ...grpc.CallOption) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
				switch in.GetExecution().GetRunId() {
				case "run-1":
					return &workflowservice.GetWorkflowExecutionHistoryResponse{
						History: &historypb.History{
							Events: []*historypb.HistoryEvent{
								{
									EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW,
									Attributes: &historypb.HistoryEvent_WorkflowExecutionContinuedAsNewEventAttributes{
										WorkflowExecutionContinuedAsNewEventAttributes: &historypb.WorkflowExecutionContinuedAsNewEventAttributes{
											NewExecutionRunId: "run-2",
											TaskQueue:         &taskqueuepb.TaskQueue{Name: "queue"},
											WorkflowType:      &commonpb.WorkflowType{Name: "workflow-name"},
										},
									},
								},
							},
						},
					}, nil
				case "run-2":
					payloads, err := converter.GetDefaultDataConverter().ToPayloads("done")
					require.NoError(t, err)
					return &workflowservice.GetWorkflowExecutionHistoryResponse{
						History: &historypb.History{
							Events: []*historypb.HistoryEvent{
								{
									EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
									Attributes: &historypb.HistoryEvent_WorkflowExecutionCompletedEventAttributes{
										WorkflowExecutionCompletedEventAttributes: &historypb.WorkflowExecutionCompletedEventAttributes{
											Result: payloads,
										},
									},
								},
							},
						},
					}, nil
				default:
					t.Fatalf("unexpected run id %q", in.GetExecution().GetRunId())
					return nil, nil
				}
			},
		},
	}

	result, err := transport.GetWorkflowResult(context.Background(), "workflow-id", "")

	require.NoError(t, err)
	assert.Equal(t, "done", result)
}

func mustCreateCertificatePEM(t *testing.T) string {
	certPEM, _ := mustCreateCertificatePairPEM(t)
	return certPEM
}

func mustCreateCertificatePairPEM(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	template := &x509.Certificate{
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		NotAfter:              time.Now().Add(time.Hour),
		NotBefore:             time.Now().Add(-time.Minute),
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: "temporal-test"},
	}

	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return string(certificatePEM), string(privateKeyPEM)
}
