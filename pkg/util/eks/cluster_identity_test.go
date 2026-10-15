// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	awsekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEKSClient struct {
	output *awseks.DescribeClusterOutput
	err    error
	name   string
	calls  int
}

func (m *mockEKSClient) DescribeCluster(_ context.Context, input *awseks.DescribeClusterInput, _ ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error) {
	m.calls++
	m.name = aws.ToString(input.Name)
	return m.output, m.err
}

func clusterOutput(name, clusterARN string) *awseks.DescribeClusterOutput {
	return &awseks.DescribeClusterOutput{Cluster: &awsekstypes.Cluster{Name: aws.String(name), Arn: aws.String(clusterARN)}}
}

func TestResolveClusterIdentity(t *testing.T) {
	client := &mockEKSClient{output: clusterOutput("orders", "arn:aws:eks:us-west-2:123456789012:cluster/orders")}

	identity, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", client)

	require.NoError(t, err)
	assert.Equal(t, "orders", client.name)
	assert.Equal(t, &ClusterIdentity{
		ClusterName: "orders",
		ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
		AccountID:   "123456789012",
		Region:      "us-west-2",
	}, identity)
}

func TestResolveClusterIdentitySupportsPartitions(t *testing.T) {
	for _, clusterARN := range []string{
		"arn:aws-cn:eks:cn-north-1:123456789012:cluster/orders",
		"arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/orders",
		"arn:aws-iso:eks:us-iso-east-1:123456789012:cluster/orders",
		"arn:aws-eusc:eks:eusc-de-east-1:123456789012:cluster/orders",
	} {
		t.Run(clusterARN, func(t *testing.T) {
			client := &mockEKSClient{output: clusterOutput("orders", clusterARN)}
			identity, err := resolveClusterIdentity(t.Context(), "orders", "", client)
			require.NoError(t, err)
			assert.Equal(t, clusterARN, identity.ClusterARN)
		})
	}
}

func TestResolveClusterIdentityRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name          string
		requestedName string
		requestRegion string
		output        *awseks.DescribeClusterOutput
	}{
		{name: "nil output", requestedName: "orders", requestRegion: "us-west-2"},
		{name: "nil cluster", requestedName: "orders", requestRegion: "us-west-2", output: &awseks.DescribeClusterOutput{}},
		{name: "nil ARN", requestedName: "orders", requestRegion: "us-west-2", output: &awseks.DescribeClusterOutput{Cluster: &awsekstypes.Cluster{Name: aws.String("orders")}}},
		{name: "name mismatch", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("payments", "arn:aws:eks:us-west-2:123456789012:cluster/payments")},
		{name: "malformed ARN", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("orders", "not-an-arn")},
		{name: "wrong service", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("orders", "arn:aws:ecs:us-west-2:123456789012:cluster/orders")},
		{name: "invalid account", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("orders", "arn:aws:eks:us-west-2:123:cluster/orders")},
		{name: "Region mismatch", requestedName: "orders", requestRegion: "us-east-1", output: clusterOutput("orders", "arn:aws:eks:us-west-2:123456789012:cluster/orders")},
		{name: "resource mismatch", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("orders", "arn:aws:eks:us-west-2:123456789012:cluster/payments")},
		{name: "wrong resource type", requestedName: "orders", requestRegion: "us-west-2", output: clusterOutput("orders", "arn:aws:eks:us-west-2:123456789012:nodegroup/orders")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := resolveClusterIdentity(t.Context(), tt.requestedName, tt.requestRegion, &mockEKSClient{output: tt.output})
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, ErrInvalidClusterIdentity)
		})
	}
}

func TestResolveClusterIdentityReturnsAPIErrors(t *testing.T) {
	apiErr := errors.New("access denied")
	identity, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", &mockEKSClient{err: apiErr})
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, apiErr)
}

func TestGetClusterIdentityRejectsInvalidNamesBeforeLoadingAWSConfig(t *testing.T) {
	for _, name := range []string{"", "-orders", "orders.example", "orders/name", " orders "} {
		t.Run(name, func(t *testing.T) {
			identity, err := GetClusterIdentity(t.Context(), name)
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, ErrInvalidClusterName)
		})
	}
}

func TestGetClusterIdentityAcceptsNumericPrefix(t *testing.T) {
	assert.True(t, clusterNamePattern.MatchString("123-orders"))
}

func stubAWS(t *testing.T, client *mockEKSClient) {
	t.Helper()
	origLoad, origNew := loadAWSConfig, newDescribeClusterAPI
	t.Cleanup(func() { loadAWSConfig, newDescribeClusterAPI = origLoad, origNew })
	loadAWSConfig = func(context.Context) (aws.Config, error) { return aws.Config{Region: "us-west-2"}, nil }
	newDescribeClusterAPI = func(aws.Config) describeClusterAPI { return client }
}

func TestGetClusterIdentityCachesSuccess(t *testing.T) {
	name := "orders-" + t.Name()
	client := &mockEKSClient{output: clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name)}
	stubAWS(t, client)

	first, err := GetClusterIdentity(t.Context(), name)
	require.NoError(t, err)
	second, err := GetClusterIdentity(t.Context(), name)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, client.calls, "second lookup must be served from cache")
}

func TestGetClusterIdentityCachesFailure(t *testing.T) {
	name := "orders-" + t.Name()
	client := &mockEKSClient{err: errors.New("AccessDeniedException")}
	stubAWS(t, client)

	_, err := GetClusterIdentity(t.Context(), name)
	require.Error(t, err)
	_, err = GetClusterIdentity(t.Context(), name)
	require.Error(t, err)

	assert.Equal(t, 1, client.calls, "a failing resolution must not be retried on every request")
}

func TestGetClusterIdentityCachesConfigLoadFailure(t *testing.T) {
	name := "orders-" + t.Name()
	client := &mockEKSClient{output: clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name)}
	stubAWS(t, client)
	configErr := errors.New("resolve EKS region: IMDS unavailable")
	configLoads := 0
	loadAWSConfig = func(context.Context) (aws.Config, error) { configLoads++; return aws.Config{}, configErr }

	identity, err := GetClusterIdentity(t.Context(), name)
	assert.Nil(t, identity)
	require.ErrorIs(t, err, configErr)
	assert.EqualError(t, err, "load AWS configuration: resolve EKS region: IMDS unavailable")

	_, err = GetClusterIdentity(t.Context(), name)
	require.ErrorIs(t, err, configErr)
	assert.Equal(t, 1, configLoads, "config failure must be negatively cached")
	assert.Equal(t, 0, client.calls, "config failure must never reach DescribeCluster")
}

func TestFailureTTLClassifiesTransientVsPermanent(t *testing.T) {
	// Permanent: self-consistency and access failures do not clear on their own.
	assert.Equal(t, failureCacheTTL, failureTTL(ErrInvalidClusterIdentity))
	assert.Equal(t, failureCacheTTL, failureTTL(&smithy.GenericAPIError{Code: "AccessDeniedException", Message: "no"}))
	assert.Equal(t, failureCacheTTL, failureTTL(&smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "no"}))

	// Transient: context deadline, throttling, and server-fault errors are retried soon.
	assert.Equal(t, transientFailureCacheTTL, failureTTL(context.DeadlineExceeded))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(fmt.Errorf("wrap: %w", context.Canceled)))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down", Fault: smithy.FaultClient}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&smithy.GenericAPIError{Code: "InternalServerErrorException", Message: "boom", Fault: smithy.FaultServer}))
}

func TestGetClusterIdentityRejectsOverlongName(t *testing.T) {
	// clusterNamePattern bounds the name at 100 chars; a 101-char name is rejected before any API call.
	id, err := GetClusterIdentity(context.Background(), "a"+strings.Repeat("b", 100))
	assert.Nil(t, id)
	assert.ErrorIs(t, err, ErrInvalidClusterName)
}
