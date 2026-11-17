// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
			header:   "AccessDeniedException:http://example.com/",
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
	resetDescribeClient(t)
	origLoad := loadAWSConfig
	t.Cleanup(func() { loadAWSConfig = origLoad })
	loadAWSConfig = func(context.Context) (aws.Config, error) { return testAWSConfig(srv.URL), nil }

	id, err := GetClusterIdentity(t.Context(), name, nil)
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

func TestDescribeClusterKeepsEndpointPath(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(okClusterHandler("orders", func(r *http.Request) { got = r }))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL+"/prefix/?x=1")).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/prefix/clusters/orders", got.URL.Path)
	assert.Empty(t, got.URL.RawQuery)
}

func TestParseAPIErrorMatchesSDKSanitizing(t *testing.T) {
	resp := func(errorType string) (int, http.Header) {
		header := http.Header{}
		if errorType != "" {
			header.Set("X-Amzn-ErrorType", errorType)
		}
		return http.StatusBadRequest, header
	}
	parseErr := func(errorType string, body []byte) error {
		status, header := resp(errorType)
		return parseAPIError(status, header, body)
	}
	code := func(err error) string {
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		return apiErr.ErrorCode()
	}
	// The SDK keeps the text before the first ':' and then after the first '#'.
	assert.Equal(t, "InvalidParameterException", code(parseErr("InvalidParameterException:http://x/", nil)))
	assert.Equal(t, "Outer#Inner", code(parseErr("ns#Outer#Inner", nil)))
	assert.Equal(t, "ResourceNotFoundException", code(parseErr("", []byte(`{"code":"com.amazonaws.eks#ResourceNotFoundException"}`))))
	// No code anywhere.
	assert.Equal(t, "UnknownError", code(parseErr("", []byte(`not json`))))
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

func TestResolveEndpointRejectsCustomEndpointWithVariants(t *testing.T) {
	// The generated client (service/eks v1.105.5) refuses these combinations instead of
	// silently ignoring FIPS or dual-stack, and so does the signed client.
	fips := awsconfig.EnvConfig{UseFIPSEndpoint: aws.FIPSEndpointStateEnabled}
	dualStack := awsconfig.EnvConfig{UseDualStackEndpoint: aws.DualStackEndpointStateEnabled}
	both := awsconfig.EnvConfig{UseFIPSEndpoint: aws.FIPSEndpointStateEnabled, UseDualStackEndpoint: aws.DualStackEndpointStateEnabled}
	for _, tc := range []struct {
		name    string
		region  string
		sources []interface{}
		want    string
	}{
		{"fips", "us-west-2", []interface{}{fips}, "Invalid Configuration: FIPS and custom endpoint are not supported"},
		{"dualstack", "us-west-2", []interface{}{dualStack}, "Invalid Configuration: Dualstack and custom endpoint are not supported"},
		{"both", "us-west-2", []interface{}{both}, "Invalid Configuration: FIPS and custom endpoint are not supported"},
		{"fips pseudo Region", "us-east-1-fips", nil, "Invalid Configuration: FIPS and custom endpoint are not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveEndpoint(t.Context(), aws.Config{Region: tc.region, BaseEndpoint: aws.String("https://custom.example"), ConfigSources: tc.sources})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, failureCacheTTL, failureTTL(err), "a configuration error does not clear on its own")
		})
	}
}

func TestDescribeClusterDropsEndpointQuery(t *testing.T) {
	// The generated client keeps a configured endpoint's path prefix and drops its query.
	capture := &captureHTTPClient{}
	cfg := testAWSConfig("https://custom.example/prefix?x=1")
	cfg.HTTPClient = capture
	_, _ = newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	require.NotNil(t, capture.req)
	assert.Equal(t, "https://custom.example/prefix/clusters/orders", capture.req.URL.String())
}

// redirectHTTPClient sends every request to a test server, keeping the signed headers, so a
// test can use a real AWS endpoint host without a custom endpoint.
type redirectHTTPClient struct{ target string }

func (c redirectHTTPClient) Do(r *http.Request) (*http.Response, error) {
	target, err := url.Parse(c.target)
	if err != nil {
		return nil, err
	}
	out := r.Clone(r.Context())
	out.URL.Scheme, out.URL.Host, out.Host = target.Scheme, target.Host, ""
	out.RequestURI = ""
	return http.DefaultClient.Do(out)
}

func TestFIPSPseudoRegions(t *testing.T) {
	// The generated client matches -fips-, fips- and -fips anywhere in the Region.
	for _, region := range []string{"us-east-1-fips", "fips-us-east-1", "us-fips-east-1"} {
		t.Run(region, func(t *testing.T) {
			got, err := resolveEndpoint(t.Context(), aws.Config{Region: region})
			require.NoError(t, err)
			assert.Equal(t, "https://fips.eks.us-east-1.amazonaws.com", got)

			// The request is signed for, and the ARN checked against, the real Region. (The
			// generated client signs with the pseudo Region as given.)
			var req *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req = r.Clone(context.Background())
				_, _ = w.Write([]byte(`{"cluster":{"name":"orders","arn":"arn:aws:eks:us-east-1:123456789012:cluster/orders"}}`))
			}))
			defer srv.Close()
			cfg := aws.Config{
				Region:      region,
				Credentials: testAWSConfig("").Credentials,
				HTTPClient:  redirectHTTPClient{target: srv.URL},
			}
			stubAWS(t, nil)
			loadAWSConfig = func(context.Context) (aws.Config, error) { return cfg, nil }
			newDescribeClusterAPI = newSignedDescribeClusterClient

			id, err := GetClusterIdentity(t.Context(), "orders", nil)
			require.NoError(t, err)
			assert.Equal(t, "us-east-1", id.Region)
			assert.Contains(t, req.Header.Get("Authorization"), "/us-east-1/eks/aws4_request")
		})
	}
}

func TestDescribeClusterSendsUserAgent(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(okClusterHandler("orders", func(r *http.Request) { got = r }))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	ua := got.Header.Get("User-Agent")
	assert.True(t, strings.HasPrefix(ua, "datadog-cluster-agent/"), ua)
	assert.Contains(t, ua, " aws-sdk-go-v2/"+aws.SDKVersion)
}

// Every answer carries the server clock in its Date header, like AWS responses do.
func skewedServer(t *testing.T, serverNow *time.Time, code string, attempts *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*attempts++
		w.Header().Set("Date", serverNow.Format(http.TimeFormat))
		signed, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		require.NoError(t, err)
		if d := signed.Sub(*serverNow); d > 5*time.Minute || d < -5*time.Minute {
			w.Header().Set("X-Amzn-ErrorType", code)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Signature expired"}`))
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
}

func TestDescribeClusterCorrectsClockSkew(t *testing.T) {
	clientNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	// The codes the SDK's clock skew correction reacts to (aws/retry clockSkewCodes).
	for _, code := range []string{"InvalidSignatureException", "SignatureDoesNotMatch", "AuthFailure", "RequestTimeTooSkewed", "AccessDeniedException"} {
		t.Run(code, func(t *testing.T) {
			attempts := 0
			serverNow := clientNow.Add(20 * time.Minute)
			srv := skewedServer(t, &serverNow, code, &attempts)
			defer srv.Close()

			client := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).(*signedDescribeClusterClient)
			client.now = func() time.Time { return clientNow }

			out, err := client.DescribeCluster(t.Context(), "orders")
			require.NoError(t, err)
			assert.Equal(t, "orders", aws.ToString(out.Cluster.Name))
			assert.Equal(t, 2, attempts, "one re-signed attempt")

			// The offset is kept, so the next call is signed correctly the first time.
			_, err = client.DescribeCluster(t.Context(), "orders")
			require.NoError(t, err)
			assert.Equal(t, 3, attempts)
		})
	}
}

func TestDescribeClusterHealsClockSkew(t *testing.T) {
	// The node clock was 20 minutes behind and the offset was learned. Then NTP fixes the
	// clock, so the stored offset is now 20 minutes wrong and must heal, as in the SDK.
	clientNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	serverNow := clientNow.Add(20 * time.Minute)
	attempts := 0
	srv := skewedServer(t, &serverNow, "InvalidSignatureException", &attempts)
	defer srv.Close()
	client := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).(*signedDescribeClusterClient)
	client.now = func() time.Time { return clientNow }

	_, err := client.DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)

	clientNow = serverNow
	attempts = 0
	_, err = client.DescribeCluster(t.Context(), "orders")
	require.NoError(t, err, "the stale offset heals")
	assert.Equal(t, 2, attempts)

	attempts = 0
	_, err = client.DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.Equal(t, 1, attempts, "and stays healed")
}

func TestDescribeClusterAccessDeniedWithoutSkewIsNotRetried(t *testing.T) {
	// AccessDeniedException counts as clock skew only when the clocks really differ.
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("X-Amzn-ErrorType", "AccessDeniedException")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
	assert.Equal(t, failureCacheTTL, failureTTL(err))
}

func TestDescribeClusterDoesNotCorrectSmallSkew(t *testing.T) {
	clientNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	attempts := 0
	// A server 2 minutes ahead is within the threshold, so a signature error is real.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Date", clientNow.Add(2*time.Minute).Format(http.TimeFormat))
		w.Header().Set("X-Amzn-ErrorType", "InvalidSignatureException")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	client := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).(*signedDescribeClusterClient)
	client.now = func() time.Time { return clientNow }

	_, err := client.DescribeCluster(t.Context(), "orders")
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
	assert.Equal(t, failureCacheTTL, failureTTL(err), "a bad signature with the clocks in sync is not a clock problem")
}

type flakyHTTPClient struct {
	failures int
	calls    int
}

func TestDescribeClusterRetriesOnceOnTransportError(t *testing.T) {
	srv := httptest.NewServer(okClusterHandler("orders", nil))
	defer srv.Close()

	flaky := &flakyHTTPClient{failures: 1}
	cfg := testAWSConfig(srv.URL)
	cfg.HTTPClient = flaky
	_, err := newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.Equal(t, 2, flaky.calls)

	// Only one retry.
	flaky = &flakyHTTPClient{failures: 2}
	cfg.HTTPClient = flaky
	_, err = newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	require.Error(t, err)
	assert.Equal(t, 2, flaky.calls)
}

func TestDescribeClusterRetriesOnceOnThrottling(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("X-Amzn-ErrorType", "TooManyRequestsException")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestDescribeClusterRetriesOnceOn5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestDescribeClusterDoesNotRetryWithoutTimeLeft(t *testing.T) {
	srv := httptest.NewServer(okClusterHandler("orders", nil))
	defer srv.Close()

	flaky := &flakyHTTPClient{failures: 1}
	cfg := testAWSConfig(srv.URL)
	cfg.HTTPClient = flaky
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := newSignedDescribeClusterClient(cfg).DescribeCluster(ctx, "orders")
	require.Error(t, err)
	assert.Equal(t, 1, flaky.calls, "a retry is only worth it with more than 500ms left")
}

func (f *flakyHTTPClient) Do(r *http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}
	return http.DefaultClient.Do(r)
}

func TestResolveEndpointFromSharedConfigServicesSection(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(configFile, []byte(`[profile eks-test]
region = us-west-2
services = custom

[services custom]
eks =
  endpoint_url = https://shared-config.example
`), 0o600))
	t.Setenv("AWS_CONFIG_FILE", configFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_PROFILE", "eks-test")
	t.Setenv("AWS_ENDPOINT_URL", "")
	t.Setenv("AWS_ENDPOINT_URL_EKS", "")
	t.Setenv("AWS_REGION", "")

	cfg, err := awsconfig.LoadDefaultConfig(t.Context())
	require.NoError(t, err)
	got, err := resolveEndpoint(t.Context(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "https://shared-config.example", got)
}

// TestResolveEndpointGolden pins the endpoint for every partition and FIPS and dual-stack
// combination. The expected values were produced by the generated client,
// aws-sdk-go-v2/service/eks v1.105.5, for the same inputs.
func TestResolveEndpointGolden(t *testing.T) {
	variants := map[string]awsconfig.EnvConfig{
		"default":        {},
		"fips":           {UseFIPSEndpoint: aws.FIPSEndpointStateEnabled},
		"dualstack":      {UseDualStackEndpoint: aws.DualStackEndpointStateEnabled},
		"fips+dualstack": {UseFIPSEndpoint: aws.FIPSEndpointStateEnabled, UseDualStackEndpoint: aws.DualStackEndpointStateEnabled},
	}
	golden := []struct{ region, variant, want string }{
		{"us-west-2", "default", "https://eks.us-west-2.amazonaws.com"},
		{"us-west-2", "fips", "https://fips.eks.us-west-2.amazonaws.com"},
		{"us-west-2", "dualstack", "https://eks.us-west-2.api.aws"},
		{"us-west-2", "fips+dualstack", "https://eks-fips.us-west-2.api.aws"},
		{"cn-north-1", "default", "https://eks.cn-north-1.amazonaws.com.cn"},
		{"cn-north-1", "fips", "https://eks-fips.cn-north-1.amazonaws.com.cn"},
		{"cn-north-1", "dualstack", "https://eks.cn-north-1.api.amazonwebservices.com.cn"},
		{"cn-north-1", "fips+dualstack", "https://eks-fips.cn-north-1.api.amazonwebservices.com.cn"},
		{"eusc-de-east-1", "default", "https://eks.eusc-de-east-1.amazonaws.eu"},
		{"eusc-de-east-1", "fips", "https://eks-fips.eusc-de-east-1.amazonaws.eu"},
		{"eusc-de-east-1", "dualstack", "https://eks.eusc-de-east-1.api.amazonwebservices.eu"},
		{"eusc-de-east-1", "fips+dualstack", "https://eks-fips.eusc-de-east-1.api.amazonwebservices.eu"},
		{"us-iso-east-1", "default", "https://eks.us-iso-east-1.c2s.ic.gov"},
		{"us-iso-east-1", "fips", "https://eks-fips.us-iso-east-1.c2s.ic.gov"},
		{"us-iso-east-1", "dualstack", "https://eks.us-iso-east-1.api.aws.ic.gov"},
		{"us-iso-east-1", "fips+dualstack", "https://eks-fips.us-iso-east-1.api.aws.ic.gov"},
		{"us-isob-east-1", "default", "https://eks.us-isob-east-1.sc2s.sgov.gov"},
		{"us-isob-east-1", "fips", "https://eks-fips.us-isob-east-1.sc2s.sgov.gov"},
		{"us-isob-east-1", "dualstack", "https://eks.us-isob-east-1.api.aws.scloud"},
		{"us-isob-east-1", "fips+dualstack", "https://eks-fips.us-isob-east-1.api.aws.scloud"},
		{"eu-isoe-west-1", "default", "https://eks.eu-isoe-west-1.cloud.adc-e.uk"},
		{"eu-isoe-west-1", "fips", "https://eks-fips.eu-isoe-west-1.cloud.adc-e.uk"},
		{"eu-isoe-west-1", "dualstack", "https://eks.eu-isoe-west-1.api.cloud-aws.adc-e.uk"},
		{"eu-isoe-west-1", "fips+dualstack", "https://eks-fips.eu-isoe-west-1.api.cloud-aws.adc-e.uk"},
		{"us-isof-south-1", "default", "https://eks.us-isof-south-1.csp.hci.ic.gov"},
		{"us-isof-south-1", "fips", "https://eks-fips.us-isof-south-1.csp.hci.ic.gov"},
		{"us-isof-south-1", "dualstack", "https://eks.us-isof-south-1.api.aws.hci.ic.gov"},
		{"us-isof-south-1", "fips+dualstack", "https://eks-fips.us-isof-south-1.api.aws.hci.ic.gov"},
		{"us-gov-west-1", "default", "https://eks.us-gov-west-1.amazonaws.com"},
		{"us-gov-west-1", "fips", "https://eks.us-gov-west-1.amazonaws.com"},
		{"us-gov-west-1", "dualstack", "https://eks.us-gov-west-1.api.aws"},
		{"us-gov-west-1", "fips+dualstack", "https://eks-fips.us-gov-west-1.api.aws"},
	}
	for _, g := range golden {
		t.Run(g.region+"/"+g.variant, func(t *testing.T) {
			got, err := resolveEndpoint(t.Context(), aws.Config{Region: g.region, ConfigSources: []interface{}{variants[g.variant]}})
			require.NoError(t, err)
			assert.Equal(t, g.want, got)
		})
	}
}

// captureHTTPClient records the request and fails it, so a test can inspect a signed request
// without any server.
type captureHTTPClient struct{ req *http.Request }

func (c *captureHTTPClient) Do(r *http.Request) (*http.Response, error) {
	c.req = r
	return nil, errors.New("captured")
}

func TestDescribeClusterExactSignature(t *testing.T) {
	// A fixed clock and fixed credentials give one exact signature, so any change to the
	// method, path, host, signed headers, Region or signing name shows up here.
	capture := &captureHTTPClient{}
	client := newSignedDescribeClusterClient(aws.Config{
		Region:      "us-west-2",
		Credentials: credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "session-token"),
		HTTPClient:  capture,
	}).(*signedDescribeClusterClient)
	client.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	_, _ = client.DescribeCluster(t.Context(), "orders")
	require.NotNil(t, capture.req)
	assert.Equal(t, "https://eks.us-west-2.amazonaws.com/clusters/orders", capture.req.URL.String())
	assert.Equal(t, "20260102T030405Z", capture.req.Header.Get("X-Amz-Date"))
	assert.Equal(t, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260102/us-west-2/eks/aws4_request, "+
		"SignedHeaders=accept;host;x-amz-date;x-amz-security-token, "+
		"Signature=eef049706f1272cea1549402abec75276cbb9c109a204f70fa83e0e2501e72a6",
		capture.req.Header.Get("Authorization"))
}

func TestDescribeClusterRejectsUnsafeNames(t *testing.T) {
	capture := &captureHTTPClient{}
	cfg := testAWSConfig("https://eks.example")
	cfg.HTTPClient = capture
	for _, name := range []string{"../admin", "orders/../x", "orders?x=1", ""} {
		_, err := newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), name)
		assert.ErrorIs(t, err, ErrInvalidClusterName, name)
	}
	assert.Nil(t, capture.req, "no request is sent")
}

func stubThrottleDelay(t *testing.T, d time.Duration) {
	t.Helper()
	orig := throttleRetryDelay
	t.Cleanup(func() { throttleRetryDelay = orig })
	throttleRetryDelay = func() time.Duration { return d }
}

func TestDescribeClusterRetriesThrottlingCodesOn400(t *testing.T) {
	// EKS answers some throttling with a 400 and a throttling code, as the SDK expects.
	stubThrottleDelay(t, 0)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("X-Amzn-ErrorType", "ThrottlingException")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestDescribeClusterWaitsBeforeThrottleRetry(t *testing.T) {
	stubThrottleDelay(t, 200*time.Millisecond)
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		if len(times) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
	defer srv.Close()

	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	require.Len(t, times, 2)
	assert.GreaterOrEqual(t, times[1].Sub(times[0]), 200*time.Millisecond)

	// The time left is measured after the wait: 600ms minus 200ms is not enough for a retry.
	times = nil
	ctx, cancel := context.WithTimeout(t.Context(), 600*time.Millisecond)
	defer cancel()
	throttled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		times = append(times, time.Now())
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer throttled.Close()
	_, err = newSignedDescribeClusterClient(testAWSConfig(throttled.URL)).DescribeCluster(ctx, "orders")
	require.Error(t, err)
	assert.Len(t, times, 1)
}

func TestThrottleRetryDelayIsJittered(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 50 {
		d := throttleRetryDelay()
		assert.GreaterOrEqual(t, d, 100*time.Millisecond)
		assert.Less(t, d, 300*time.Millisecond)
		seen[d] = true
	}
	assert.Greater(t, len(seen), 1)
}

func TestDescribeClusterRetriesOnlyTransient5xx(t *testing.T) {
	for status, wantAttempts := range map[int]int{500: 2, 502: 2, 503: 2, 504: 2, 501: 1, 505: 1} {
		attempts := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			w.WriteHeader(status)
		}))
		_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
		srv.Close()
		require.Error(t, err)
		assert.Equal(t, wantAttempts, attempts, "status %d", status)
	}
}

func TestFailureTTLTruncatedBodyIsTransient(t *testing.T) {
	for _, cause := range []error{io.ErrUnexpectedEOF, io.EOF} {
		err := fmt.Errorf("describe: %w", &transportError{fmt.Errorf("read DescribeCluster response: %w", cause)})
		assert.Equal(t, transientFailureCacheTTL, failureTTL(err))
	}
}

func TestFailureTTLSignatureErrorsNeedSkew(t *testing.T) {
	for _, code := range []string{"InvalidSignatureException", "SignatureDoesNotMatch"} {
		assert.Equal(t, failureCacheTTL, failureTTL(&apiError{StatusCode: 403, Code: code}), code)
		assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 403, Code: code, skewChanged: true}), code)
	}
	// A clock error says so whatever the observed skew.
	assert.Equal(t, transientFailureCacheTTL, failureTTL(&apiError{StatusCode: 403, Code: "RequestTimeTooSkewed"}))
}

func TestDescribeClusterIgnoresDateOfACachedAnswer(t *testing.T) {
	// A response with an Age header may come from a cache, so its Date says nothing about the
	// server clock (SDK computeClockSkew).
	clientNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Date", clientNow.Add(20*time.Minute).Format(http.TimeFormat))
		w.Header().Set("Age", "1200")
		w.Header().Set("X-Amzn-ErrorType", "InvalidSignatureException")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	client := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).(*signedDescribeClusterClient)
	client.now = func() time.Time { return clientNow }

	_, err := client.DescribeCluster(t.Context(), "orders")
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "no re-signed attempt")
	assert.Zero(t, client.clockSkew.Load())
}

func TestDescribeClusterChecksTheTimeLeftBeforeWaiting(t *testing.T) {
	// 600ms left: a 200ms wait plus the 500ms an attempt needs does not fit, so the client
	// returns at once instead of sleeping for nothing.
	stubThrottleDelay(t, 200*time.Millisecond)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 600*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(ctx, "orders")
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
	assert.Less(t, time.Since(start), 150*time.Millisecond)
}

func TestDescribeClusterWaitsBefore5xxRetry(t *testing.T) {
	stubThrottleDelay(t, 200*time.Millisecond)
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		if len(times) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okClusterHandler("orders", nil)(w, r)
	}))
	defer srv.Close()
	_, err := newSignedDescribeClusterClient(testAWSConfig(srv.URL)).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	require.Len(t, times, 2)
	assert.GreaterOrEqual(t, times[1].Sub(times[0]), 200*time.Millisecond)
}

func TestParseAPIErrorPrefersTypeOverCode(t *testing.T) {
	// The SDK reads __type before code when the body has both.
	apiErr := parseAPIError(http.StatusBadRequest, http.Header{}, []byte(`{"code":"A","__type":"B"}`))
	assert.Equal(t, "B", apiErr.Code)
	apiErr = parseAPIError(http.StatusBadRequest, http.Header{}, []byte(`{"code":"A"}`))
	assert.Equal(t, "A", apiErr.Code)
}

func TestDescribeClusterRejectsAnEndpointWithoutSchemeOrHost(t *testing.T) {
	// AWS_ENDPOINT_URL_EKS=svc.example.com is a configuration error. It must not be retried
	// every 30s as a transport error.
	for _, endpoint := range []string{"svc.example.com", "https://", "/path/only"} {
		_, err := newSignedDescribeClusterClient(testAWSConfig(endpoint)).DescribeCluster(t.Context(), "orders")
		require.Error(t, err, endpoint)
		assert.Equal(t, failureCacheTTL, failureTTL(err), endpoint)
	}
}

func TestDescribeClusterSendsTheAppID(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(okClusterHandler("orders", func(r *http.Request) { got = r }))
	defer srv.Close()
	cfg := testAWSConfig(srv.URL)
	cfg.AppID = "orders-platform"
	_, err := newSignedDescribeClusterClient(cfg).DescribeCluster(t.Context(), "orders")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(got.Header.Get("User-Agent"), " app/orders-platform"), got.Header.Get("User-Agent"))
}
