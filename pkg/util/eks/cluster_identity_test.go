// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	eksid "github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type mockEKSClient struct {
	output *describeClusterOutput
	err    error
	name   string
	calls  int
}

func (m *mockEKSClient) DescribeCluster(_ context.Context, clusterName string) (*describeClusterOutput, error) {
	m.calls++
	m.name = clusterName
	return m.output, m.err
}

func clusterOutput(name, clusterARN string) *describeClusterOutput {
	return &describeClusterOutput{Cluster: &describedCluster{Name: aws.String(name), Arn: aws.String(clusterARN)}}
}

func TestResolveClusterIdentity(t *testing.T) {
	client := &mockEKSClient{output: clusterOutput("orders", "arn:aws:eks:us-west-2:123456789012:cluster/orders")}

	identity, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", nil, client)

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
			identity, err := resolveClusterIdentity(t.Context(), "orders", "", nil, client)
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
		output        *describeClusterOutput
	}{
		{name: "nil output", requestedName: "orders", requestRegion: "us-west-2"},
		{name: "nil cluster", requestedName: "orders", requestRegion: "us-west-2", output: &describeClusterOutput{}},
		{name: "nil ARN", requestedName: "orders", requestRegion: "us-west-2", output: &describeClusterOutput{Cluster: &describedCluster{Name: aws.String("orders")}}},
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
			identity, err := resolveClusterIdentity(t.Context(), tt.requestedName, tt.requestRegion, nil, &mockEKSClient{output: tt.output})
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, ErrInvalidClusterIdentity)
		})
	}
}

func TestResolveClusterIdentityReturnsAPIErrors(t *testing.T) {
	apiErr := errors.New("access denied")
	identity, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", nil, &mockEKSClient{err: apiErr})
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, apiErr)
}

func TestGetClusterIdentityRejectsInvalidNamesBeforeLoadingAWSConfig(t *testing.T) {
	for _, name := range []string{"", "-orders", "orders.example", "orders/name", " orders "} {
		t.Run(name, func(t *testing.T) {
			identity, err := GetClusterIdentity(t.Context(), name, nil)
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, ErrInvalidClusterName)
		})
	}
}

func TestGetClusterIdentityAcceptsNumericPrefix(t *testing.T) {
	assert.True(t, eksid.IsValidClusterName("123-orders"))
}

// resetIdentityCache drops every cached resolution, so a test (or a -count>1 rerun) never sees
// a result cached by an earlier one.
func resetIdentityCache(t *testing.T) {
	t.Helper()
	cache.Cache.Flush()
	t.Cleanup(cache.Cache.Flush)
}

// resetDescribeClient drops the client built from a previous test's AWS configuration.
func resetDescribeClient(t *testing.T) {
	t.Helper()
	reset := func() {
		describeClient.Lock()
		describeClient.client, describeClient.region = nil, ""
		describeClient.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func stubAWS(t *testing.T, client describeClusterAPI) {
	t.Helper()
	resetIdentityCache(t)
	resetDescribeClient(t)
	origLoad, origNew := loadAWSConfig, newDescribeClusterAPI
	t.Cleanup(func() { loadAWSConfig, newDescribeClusterAPI = origLoad, origNew })
	loadAWSConfig = func(context.Context) (aws.Config, error) { return aws.Config{Region: "us-west-2"}, nil }
	newDescribeClusterAPI = func(aws.Config) describeClusterAPI { return client }
}

func TestGetClusterIdentityCachesSuccess(t *testing.T) {
	name := "orders-" + t.Name()
	client := &mockEKSClient{output: clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name)}
	stubAWS(t, client)

	first, err := GetClusterIdentity(t.Context(), name, nil)
	require.NoError(t, err)
	second, err := GetClusterIdentity(t.Context(), name, nil)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, client.calls, "second lookup must be served from cache")
	// Each call must return a distinct pointer so a caller cannot mutate the cached value.
	assert.NotSame(t, first, second, "callers must not alias the process-global cached value")
}

func TestGetClusterIdentityCachesFailure(t *testing.T) {
	name := "orders-" + t.Name()
	client := &mockEKSClient{err: errors.New("AccessDeniedException")}
	stubAWS(t, client)

	_, err := GetClusterIdentity(t.Context(), name, nil)
	require.Error(t, err)
	_, err = GetClusterIdentity(t.Context(), name, nil)
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

	identity, err := GetClusterIdentity(t.Context(), name, nil)
	assert.Nil(t, identity)
	require.ErrorIs(t, err, configErr)
	assert.EqualError(t, err, "load AWS configuration: resolve EKS region: IMDS unavailable")

	_, err = GetClusterIdentity(t.Context(), name, nil)
	require.ErrorIs(t, err, configErr)
	assert.Equal(t, 1, configLoads, "config failure must be negatively cached")
	assert.Equal(t, 0, client.calls, "config failure must never reach DescribeCluster")
}

func TestFailureTTLClassifiesTransientVsPermanent(t *testing.T) {
	// Permanent: self-consistency and access failures do not clear on their own.
	assert.Equal(t, failureCacheTTL, failureTTL(ErrInvalidClusterIdentity))
	assert.Equal(t, failureCacheTTL, failureTTL(&apiError{StatusCode: 403, Code: "AccessDeniedException"}))
	assert.Equal(t, failureCacheTTL, failureTTL(&apiError{StatusCode: 404, Code: "ResourceNotFoundException"}))

	// Transient: context deadline/cancellation and the SDK's retryable error codes.
	assert.Equal(t, deadlineFailureCacheTTL, failureTTL(context.DeadlineExceeded))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 400, Code: "ThrottlingException"}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 400, Code: "RequestLimitExceeded"}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 429, Code: "TooManyRequestsException"}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 500, Code: "ServerException"}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 503, Code: "ServiceUnavailableException"}))
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 403, Code: "ExpiredTokenException"}))
}

func TestGetClusterIdentityRejectsOverlongName(t *testing.T) {
	// IsValidClusterName bounds the name at 100 chars; a 101-char name is rejected before any API call.
	id, err := GetClusterIdentity(context.Background(), "a"+strings.Repeat("b", 100), nil)
	assert.Nil(t, id)
	assert.ErrorIs(t, err, ErrInvalidClusterName)
}

// blockingEKSClient answers DescribeCluster only once release is closed, or fails when its own
// ctx is done.
type blockingEKSClient struct {
	started chan struct{}
	release chan struct{}
	output  *describeClusterOutput
}

func (b *blockingEKSClient) DescribeCluster(ctx context.Context, _ string) (*describeClusterOutput, error) {
	close(b.started)
	select {
	case <-b.release:
		return b.output, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestGetClusterIdentityCallerCancelDoesNotFailOtherCallers(t *testing.T) {
	name := "orders-cancel"
	client := &blockingEKSClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
		output:  clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name),
	}
	stubAWS(t, client)

	// The first node disconnects while the resolve it started is still running.
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	firstErr := make(chan error, 1)
	go func() {
		_, err := GetClusterIdentity(firstCtx, name, nil)
		firstErr <- err
	}()
	<-client.started

	// A second node is waiting on the same resolve.
	secondResult := make(chan error, 1)
	go func() {
		_, err := GetClusterIdentity(t.Context(), name, nil)
		secondResult <- err
	}()

	cancelFirst()
	require.ErrorIs(t, <-firstErr, context.Canceled, "a cancelled caller returns its own ctx error")

	close(client.release)
	require.NoError(t, <-secondResult, "a waiter must not inherit another caller's cancellation")

	id, err := GetClusterIdentity(t.Context(), name, nil)
	require.NoError(t, err, "a caller's cancellation must not be cached")
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/"+name, id.ClusterARN)
}

func TestFailureTTLDoesNotCacheCancellation(t *testing.T) {
	name := "orders-cancelled-error"
	stubAWS(t, &mockEKSClient{err: context.Canceled})

	_, err := GetClusterIdentity(t.Context(), name, nil)
	require.ErrorIs(t, err, context.Canceled)
	_, cacheKey := identityKeys(name, nil)
	_, found := cache.Cache.Get(cacheKey)
	assert.False(t, found, "context.Canceled must never be cached")
}

// codedError and operationError mimic the AWS SDK's smithy.GenericAPIError and
// smithy.OperationError: an error code reached through an Unwrap chain.
type codedError struct{ code string }

func (e *codedError) Error() string     { return "api error " + e.code }
func (e *codedError) ErrorCode() string { return e.code }

type operationError struct{ err error }

func (e *operationError) Error() string { return "operation error STS: " + e.err.Error() }
func (e *operationError) Unwrap() error { return e.err }

func TestFailureTTLClassifiesCredentialErrorsAsTransient(t *testing.T) {
	// Credentials.Retrieve returns STS errors as wrapped SDK errors, not as *apiError. These
	// clear once the projected token is rotated or the identity provider recovers.
	for _, code := range []string{"ExpiredToken", "ExpiredTokenException", "InvalidIdentityToken", "IDPCommunicationError", "RequestExpired"} {
		t.Run(code, func(t *testing.T) {
			err := fmt.Errorf("load AWS configuration: retrieve AWS credentials: %w", &operationError{&codedError{code}})
			assert.Equal(t, transientFailureCacheTTL, failureTTL(err))
		})
	}
	assert.Equal(t, failureCacheTTL, failureTTL(fmt.Errorf("wrap: %w", &operationError{&codedError{"AccessDenied"}})))
}

func TestGetClusterIdentityLoadsAWSConfigOnce(t *testing.T) {
	stubAWS(t, &mockEKSClient{output: clusterOutput("orders-a", "arn:aws:eks:us-west-2:123456789012:cluster/orders-a")})
	loads, clients := 0, 0
	loadAWSConfig = func(context.Context) (aws.Config, error) { loads++; return aws.Config{Region: "us-west-2"}, nil }
	newDescribeClusterAPI = func(aws.Config) describeClusterAPI {
		clients++
		return describeFunc(func(name string) (*describeClusterOutput, error) {
			return clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name), nil
		})
	}

	for _, name := range []string{"orders-a", "orders-b", "orders-c"} {
		_, err := GetClusterIdentity(t.Context(), name, nil)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, loads, "the AWS configuration and its credentials cache are built once")
	assert.Equal(t, 1, clients, "the client, and the clock skew it learned, are kept")
}

func TestGetClusterIdentityRetriesFailedAWSConfigLoad(t *testing.T) {
	stubAWS(t, nil)
	loads := 0
	loadAWSConfig = func(context.Context) (aws.Config, error) {
		loads++
		if loads == 1 {
			return aws.Config{}, errors.New("IMDS unavailable")
		}
		return aws.Config{Region: "us-west-2"}, nil
	}
	newDescribeClusterAPI = func(aws.Config) describeClusterAPI {
		return describeFunc(func(name string) (*describeClusterOutput, error) {
			return clusterOutput(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name), nil
		})
	}

	_, err := GetClusterIdentity(t.Context(), "orders-a", nil)
	require.Error(t, err)
	_, err = GetClusterIdentity(t.Context(), "orders-b", nil)
	require.NoError(t, err, "a failed load is not kept")
}

type describeFunc func(name string) (*describeClusterOutput, error)

func (f describeFunc) DescribeCluster(_ context.Context, name string) (*describeClusterOutput, error) {
	return f(name)
}

// testCAPEM returns a self-signed CA certificate in PEM form, like the cluster CA mounted into
// every pod at /var/run/secrets/kubernetes.io/serviceaccount/ca.crt.
func testCAPEM(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(0, 0).Add(100 * 365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func clusterOutputWithCA(name, clusterARN string, caPEM []byte) *describeClusterOutput {
	out := clusterOutput(name, clusterARN)
	out.Cluster.CertificateAuthority = &certificateAuthority{Data: aws.String(base64.StdEncoding.EncodeToString(caPEM))}
	return out
}

func TestResolveClusterIdentityVerifiesClusterCA(t *testing.T) {
	const arn = "arn:aws:eks:us-west-2:123456789012:cluster/orders"
	ours := testCAPEM(t, "kubernetes")
	other := testCAPEM(t, "kubernetes")

	t.Run("match", func(t *testing.T) {
		client := &mockEKSClient{output: clusterOutputWithCA("orders", arn, ours)}
		id, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", ours, client)
		require.NoError(t, err)
		assert.Equal(t, arn, id.ClusterARN)
	})

	t.Run("another cluster with the same name", func(t *testing.T) {
		// A stale or reused cluster name (blue/green) resolves to a different cluster whose
		// ARN passes every other check. Its CA is not the one this pod trusts.
		client := &mockEKSClient{output: clusterOutputWithCA("orders", arn, other)}
		id, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", ours, client)
		assert.Nil(t, id)
		assert.ErrorIs(t, err, ErrClusterMismatch)
		assert.Equal(t, failureCacheTTL, failureTTL(err))
	})

	t.Run("no CA in the answer", func(t *testing.T) {
		client := &mockEKSClient{output: clusterOutput("orders", arn)}
		_, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", ours, client)
		assert.ErrorIs(t, err, ErrClusterMismatch)
	})

	t.Run("not verified when the caller has no CA", func(t *testing.T) {
		client := &mockEKSClient{output: clusterOutput("orders", arn)}
		_, err := resolveClusterIdentity(t.Context(), "orders", "us-west-2", nil, client)
		require.NoError(t, err)
	})
}

func TestGetClusterIdentityCacheKeyIncludesCA(t *testing.T) {
	const name = "orders-ca-key"
	ours := testCAPEM(t, "kubernetes")
	client := &mockEKSClient{output: clusterOutputWithCA(name, "arn:aws:eks:us-west-2:123456789012:cluster/"+name, ours)}
	stubAWS(t, client)

	_, err := GetClusterIdentity(t.Context(), name, testCAPEM(t, "kubernetes"))
	require.ErrorIs(t, err, ErrClusterMismatch)
	// A mismatch cached for one CA must not answer a lookup with another.
	_, err = GetClusterIdentity(t.Context(), name, ours)
	require.NoError(t, err)
}

func TestGetClusterIdentityWarnsOncePerResolution(t *testing.T) {
	name := "orders-warn"
	stubAWS(t, &mockEKSClient{err: &apiError{StatusCode: 403, Code: "AccessDeniedException", Message: "not authorized"}})
	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(&buf, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })

	for range 3 {
		_, err := GetClusterIdentity(t.Context(), name, nil)
		require.Error(t, err)
	}
	log.Flush()
	assert.Equal(t, 1, strings.Count(buf.String(), "WARN"), buf.String())
	assert.Contains(t, buf.String(), "AccessDeniedException")
}

func TestGetClusterIdentityKeepsCallerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The caller's overall budget bounds DescribeCluster, even though the resolve is not
		// cancelled with the caller.
		client := &blockingEKSClient{started: make(chan struct{}), release: make(chan struct{})}
		stubAWS(t, client)
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := GetClusterIdentity(ctx, "orders-deadline", nil)
		require.Error(t, err)
		assert.Equal(t, 500*time.Millisecond, time.Since(start))
		synctest.Wait()
	})
}

type panickingEKSClient struct{}

func (panickingEKSClient) DescribeCluster(context.Context, string) (*describeClusterOutput, error) {
	panic("boom")
}

func TestGetClusterIdentityRecoversPanics(t *testing.T) {
	stubAWS(t, panickingEKSClient{})
	id, err := GetClusterIdentity(t.Context(), "orders-panic", nil)
	assert.Nil(t, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// hangingEKSClient answers only when its ctx is done, like an endpoint that never answers.
type hangingEKSClient struct{ calls atomic.Int32 }

func (h *hangingEKSClient) DescribeCluster(ctx context.Context, _ string) (*describeClusterOutput, error) {
	h.calls.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestGetClusterIdentityRemembersItsOwnTimeoutBriefly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A cold Cluster Agent can run out of its own resolve budget. That says little about
		// EKS, so it is remembered for a couple of seconds only: long enough to absorb a burst
		// of node requests, short enough for a node to see the next resolve within its budget.
		client := &hangingEKSClient{}
		stubAWS(t, client)
		_, err := GetClusterIdentity(t.Context(), "orders-own-timeout", nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		_, err = GetClusterIdentity(t.Context(), "orders-own-timeout", nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, int32(1), client.calls.Load())

		time.Sleep(deadlineFailureCacheTTL + time.Millisecond)
		_, _ = GetClusterIdentity(t.Context(), "orders-own-timeout", nil)
		assert.Equal(t, int32(2), client.calls.Load())
		assert.Less(t, deadlineFailureCacheTTL, transientFailureCacheTTL)
	})
}
