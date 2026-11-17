// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAWSConfig(endpoint string) aws.Config {
	return aws.Config{
		Region:       "us-west-2",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "session-token"),
		HTTPClient:   http.DefaultClient,
	}
}

func TestDescribeClusterSignsAndParsesRequest(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		// Fields beyond name and arn are ignored.
		_, _ = w.Write([]byte(`{"cluster":{"name":"Orders_Prod","arn":"arn:aws:eks:us-west-2:123456789012:cluster/Orders_Prod","status":"ACTIVE","version":"1.31","tags":{"a":"b"}}}`))
	}))
	defer srv.Close()

	client := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).(*signedDescribeClusterClient)
	client.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	out, err := client.DescribeCluster(t.Context(), "Orders_Prod")
	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	assert.Equal(t, "Orders_Prod", aws.ToString(out.Cluster.Name))
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/Orders_Prod", aws.ToString(out.Cluster.Arn))

	require.NotNil(t, got)
	assert.Equal(t, http.MethodGet, got.Method)
	// The case-sensitive EKS name is sent as is.
	assert.Equal(t, "/clusters/Orders_Prod", got.URL.Path)
	assert.Equal(t, "20260102T030405Z", got.Header.Get("X-Amz-Date"))
	assert.Equal(t, "session-token", got.Header.Get("X-Amz-Security-Token"))
	auth := got.Header.Get("Authorization")
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260102/us-west-2/eks/aws4_request, SignedHeaders="), auth)
	assert.Contains(t, auth, "host")
	assert.Contains(t, auth, "Signature=")
}

func TestDescribeClusterParsesAPIErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     string
		body       string
		wantCode   string
		wantMsg    string
		wantTTL    time.Duration
		requestID  string
		wantStatus int
	}{
		{
			name: "access denied from header", status: 403,
			header:   "AccessDeniedException:http://internal.amazon.com/coral/com.amazonaws.eks/",
			body:     `{"message":"User is not authorized to perform eks:DescribeCluster"}`,
			wantCode: "AccessDeniedException", wantMsg: "User is not authorized to perform eks:DescribeCluster",
			wantTTL: failureCacheTTL, requestID: "req-1",
		},
		{
			name: "not found from body type", status: 404,
			body:     `{"__type":"com.amazonaws.eks#ResourceNotFoundException","Message":"No cluster found for name: orders."}`,
			wantCode: "ResourceNotFoundException", wantMsg: "No cluster found for name: orders.",
			wantTTL: failureCacheTTL,
		},
		{
			name: "throttled", status: 429, header: "TooManyRequestsException",
			wantCode: "TooManyRequestsException", wantTTL: transientFailureCacheTTL,
		},
		{
			name: "server error without code", status: 500, body: "oops",
			wantCode: "UnknownError", wantTTL: transientFailureCacheTTL,
		},
		{
			name: "expired credentials", status: 403, header: "ExpiredTokenException",
			wantCode: "ExpiredTokenException", wantTTL: transientFailureCacheTTL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.header != "" {
					w.Header().Set("X-Amzn-ErrorType", tt.header)
				}
				if tt.requestID != "" {
					w.Header().Set("X-Amzn-RequestId", tt.requestID)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
			var apiErr *apiError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.status, apiErr.StatusCode)
			assert.Equal(t, tt.wantCode, apiErr.ErrorCode())
			assert.Equal(t, tt.wantMsg, apiErr.ErrorMessage())
			if tt.requestID != "" {
				assert.Contains(t, err.Error(), tt.requestID)
			}
			assert.Equal(t, tt.wantTTL, failureTTL(err))
		})
	}
}

func TestDescribeClusterTransportFailureIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endpoint := srv.URL
	srv.Close() // nothing listens on the endpoint any more

	_, err := newSignedDescribeClusterClient(testAWSConfig(endpoint)).DescribeCluster(t.Context(), "orders")
	require.Error(t, err)
	assert.Equal(t, transientFailureCacheTTL, failureTTL(err))
}

func TestDescribeClusterRejectsMalformedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	assert.ErrorIs(t, err, ErrInvalidClusterIdentity)
}

func TestDescribeClusterCredentialFailures(t *testing.T) {
	cfg := testAWSConfig("http://127.0.0.1:1")
	cfg.Credentials = nil
	_, err := newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	require.Error(t, err)

	credErr := errors.New("no web identity token")
	cfg.Credentials = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, credErr
	})
	_, err = newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	assert.ErrorIs(t, err, credErr)
}

func TestGetClusterIdentityThroughSignedClient(t *testing.T) {
	name := "orders-signed"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clusters/"+name {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"cluster":{"name":"` + name + `","arn":"arn:aws:eks:us-west-2:123456789012:cluster/` + name + `"}}`))
	}))
	defer srv.Close()

	resetIdentityCache(t)
	origLoad := loadAWSConfig
	t.Cleanup(func() { loadAWSConfig = origLoad })
	loadAWSConfig = func(context.Context) (aws.Config, error) { return testAWSConfig(srv.URL), nil }

	id, err := GetClusterIdentity(t.Context(), name)
	require.NoError(t, err)
	assert.Equal(t, &ClusterIdentity{
		ClusterName: name,
		ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/" + name,
		AccountID:   "123456789012",
		Region:      "us-west-2",
	}, id)
}

func TestResolveEndpoint(t *testing.T) {
	fips := awsconfig.EnvConfig{UseFIPSEndpoint: aws.FIPSEndpointStateEnabled}
	dualStack := awsconfig.EnvConfig{UseDualStackEndpoint: aws.DualStackEndpointStateEnabled}
	both := awsconfig.EnvConfig{UseFIPSEndpoint: aws.FIPSEndpointStateEnabled, UseDualStackEndpoint: aws.DualStackEndpointStateEnabled}

	tests := []struct {
		region  string
		sources []interface{}
		want    string
	}{
		{region: "us-west-2", want: "https://eks.us-west-2.amazonaws.com"},
		{region: "eu-central-1", want: "https://eks.eu-central-1.amazonaws.com"},
		{region: "cn-north-1", want: "https://eks.cn-north-1.amazonaws.com.cn"},
		{region: "us-gov-west-1", want: "https://eks.us-gov-west-1.amazonaws.com"},
		{region: "us-iso-east-1", want: "https://eks.us-iso-east-1.c2s.ic.gov"},
		{region: "us-isob-east-1", want: "https://eks.us-isob-east-1.sc2s.sgov.gov"},
		{region: "eusc-de-east-1", want: "https://eks.eusc-de-east-1.amazonaws.eu"},
		{region: "us-west-2", sources: []interface{}{fips}, want: "https://fips.eks.us-west-2.amazonaws.com"},
		{region: "us-gov-east-1", sources: []interface{}{fips}, want: "https://eks.us-gov-east-1.amazonaws.com"},
		{region: "us-iso-east-1", sources: []interface{}{fips}, want: "https://eks-fips.us-iso-east-1.c2s.ic.gov"},
		{region: "us-west-2", sources: []interface{}{dualStack}, want: "https://eks.us-west-2.api.aws"},
		{region: "us-west-2", sources: []interface{}{both}, want: "https://eks-fips.us-west-2.api.aws"},
	}
	for _, tt := range tests {
		t.Run(tt.region+"/"+tt.want, func(t *testing.T) {
			got, err := resolveEndpoint(t.Context(), aws.Config{Region: tt.region, ConfigSources: tt.sources})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := resolveEndpoint(t.Context(), aws.Config{})
	assert.Error(t, err, "a missing Region must not produce an endpoint")
	_, err = resolveEndpoint(t.Context(), aws.Config{Region: "us-west-2/evil"})
	assert.Error(t, err, "a Region must be a single host label")
}

func TestResolveEndpointHonorsConfiguredEndpoints(t *testing.T) {
	envSources := []interface{}{awsconfig.EnvConfig{}}

	t.Run("global endpoint", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "https://global.example")
		got, err := resolveEndpoint(t.Context(), aws.Config{Region: "us-west-2", BaseEndpoint: aws.String("https://global.example"), ConfigSources: envSources})
		require.NoError(t, err)
		assert.Equal(t, "https://global.example", got)
	})

	t.Run("service endpoint overrides global", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "https://global.example")
		t.Setenv("AWS_ENDPOINT_URL_EKS", "https://eks.example")
		got, err := resolveEndpoint(t.Context(), aws.Config{Region: "us-west-2", BaseEndpoint: aws.String("https://global.example"), ConfigSources: envSources})
		require.NoError(t, err)
		assert.Equal(t, "https://eks.example", got)
	})

	t.Run("configured endpoints ignored", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL_EKS", "https://eks.example")
		sources := []interface{}{awsconfig.EnvConfig{IgnoreConfiguredEndpoints: aws.Bool(true)}}
		got, err := resolveEndpoint(t.Context(), aws.Config{Region: "us-west-2", ConfigSources: sources})
		require.NoError(t, err)
		assert.Equal(t, "https://eks.us-west-2.amazonaws.com", got)
	})
}

func okClusterHandler(name string, record func(*http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if record != nil {
			record(r.Clone(context.Background()))
		}
		_, _ = w.Write([]byte(`{"cluster":{"name":"` + name + `","arn":"arn:aws:eks:us-west-2:123456789012:cluster/` + name + `"}}`))
	}
}

func TestDescribeClusterKeepsEndpointPathAndQuery(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(okClusterHandler("orders", func(r *http.Request) { got = r }))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL+"/prefix/?x=1")).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/prefix/clusters/orders", got.URL.Path)
	assert.Equal(t, "x=1", got.URL.RawQuery)
}

func TestParseAPIErrorMatchesSDKSanitizing(t *testing.T) {
	resp := func(header string) *http.Response {
		r := &http.Response{StatusCode: 400, Header: http.Header{}}
		if header != "" {
			r.Header.Set("X-Amzn-ErrorType", header)
		}
		return r
	}
	code := func(err error) string {
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		return apiErr.ErrorCode()
	}
	// The SDK keeps the text before the first ':' and then after the first '#'.
	assert.Equal(t, "InvalidParameterException", code(parseAPIError(resp("InvalidParameterException:http://x/"), nil)))
	assert.Equal(t, "Outer#Inner", code(parseAPIError(resp("ns#Outer#Inner"), nil)))
	assert.Equal(t, "ResourceNotFoundException", code(parseAPIError(resp(""), []byte(`{"code":"com.amazonaws.eks#ResourceNotFoundException"}`))))
	// No code anywhere.
	assert.Equal(t, "UnknownError", code(parseAPIError(resp(""), []byte(`not json`))))
}

func TestResolveEndpointTakesFirstIgnoreConfiguredEndpointsSetting(t *testing.T) {
	t.Setenv("AWS_ENDPOINT_URL_EKS", "https://eks.example")
	// The environment explicitly keeps configured endpoints; the shared config would ignore
	// them. Like the SDK, the first source that sets the flag wins.
	sources := []interface{}{
		awsconfig.EnvConfig{IgnoreConfiguredEndpoints: aws.Bool(false)},
		awsconfig.SharedConfig{IgnoreConfiguredEndpoints: aws.Bool(true)},
	}
	got, err := resolveEndpoint(t.Context(), aws.Config{Region: "us-west-2", ConfigSources: sources})
	require.NoError(t, err)
	assert.Equal(t, "https://eks.example", got)
}

func TestFIPSPseudoRegions(t *testing.T) {
	for _, region := range []string{"us-east-1-fips", "fips-us-east-1"} {
		t.Run(region, func(t *testing.T) {
			got, err := resolveEndpoint(t.Context(), aws.Config{Region: region})
			require.NoError(t, err)
			assert.Equal(t, "https://fips.eks.us-east-1.amazonaws.com", got)

			// The request is signed for, and the ARN checked against, the real Region.
			var req *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req = r.Clone(context.Background())
				_, _ = w.Write([]byte(`{"cluster":{"name":"orders","arn":"arn:aws:eks:us-east-1:123456789012:cluster/orders"}}`))
			}))
			defer srv.Close()
			cfg := testAWSConfig(srv.URL)
			cfg.Region = region
			stubAWS(t, nil)
			loadAWSConfig = func(context.Context) (aws.Config, error) { return cfg, nil }
			newDescribeClusterAPI = newSignedDescribeClusterClient

			id, err := GetClusterIdentity(t.Context(), "orders")
			require.NoError(t, err)
			assert.Equal(t, "us-east-1", id.Region)
			assert.Contains(t, req.Header.Get("Authorization"), "/us-east-1/eks/aws4_request")
		})
	}
}
