// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		service      string
		region       string
		baseEndpoint string
		useFIPS      bool
		useDualStack bool
		expected     string
		wantErr      string
	}{
		{
			name:     "standard commercial partition EC2",
			service:  "ec2",
			region:   "us-east-1",
			expected: "https://ec2.us-east-1.amazonaws.com",
		},
		{
			name:     "standard commercial partition RDS",
			service:  "rds",
			region:   "eu-west-3",
			expected: "https://rds.eu-west-3.amazonaws.com",
		},
		{
			name:     "commercial partition FIPS",
			service:  "ec2",
			region:   "us-west-2",
			useFIPS:  true,
			expected: "https://ec2-fips.us-west-2.amazonaws.com",
		},
		{
			name:         "commercial partition DualStack",
			service:      "ec2",
			region:       "us-east-1",
			useDualStack: true,
			expected:     "https://ec2.us-east-1.api.aws",
		},
		{
			name:         "commercial partition FIPS + DualStack",
			service:      "rds",
			region:       "us-east-2",
			useFIPS:      true,
			useDualStack: true,
			expected:     "https://rds-fips.us-east-2.api.aws",
		},
		{
			name:     "China partition",
			service:  "ec2",
			region:   "cn-north-1",
			expected: "https://ec2.cn-north-1.amazonaws.com.cn",
		},
		{
			name:         "China partition DualStack",
			service:      "rds",
			region:       "cn-northwest-1",
			useDualStack: true,
			expected:     "https://rds.cn-northwest-1.api.amazonwebservices.com.cn",
		},
		{
			name:     "GovCloud standard",
			service:  "ec2",
			region:   "us-gov-west-1",
			expected: "https://ec2.us-gov-west-1.amazonaws.com",
		},
		{
			name:     "GovCloud FIPS uses standard endpoint",
			service:  "ec2",
			region:   "us-gov-east-1",
			useFIPS:  true,
			expected: "https://ec2.us-gov-east-1.amazonaws.com",
		},
		{
			name:         "GovCloud FIPS + DualStack",
			service:      "rds",
			region:       "us-gov-west-1",
			useFIPS:      true,
			useDualStack: true,
			expected:     "https://rds-fips.us-gov-west-1.api.aws",
		},
		{
			name:     "European Sovereign Cloud partition",
			service:  "ec2",
			region:   "eusc-de-east-1",
			expected: "https://ec2.eusc-de-east-1.amazonaws.eu",
		},
		{
			name:     "ISO partition",
			service:  "ec2",
			region:   "us-iso-east-1",
			expected: "https://ec2.us-iso-east-1.c2s.ic.gov",
		},
		{
			name:     "ISO-B partition",
			service:  "ec2",
			region:   "us-isob-east-1",
			expected: "https://ec2.us-isob-east-1.sc2s.sgov.gov",
		},
		{
			name:     "ISO-E partition",
			service:  "ec2",
			region:   "eu-isoe-west-1",
			expected: "https://ec2.eu-isoe-west-1.cloud.adc-e.uk",
		},
		{
			name:     "ISO-F partition",
			service:  "ec2",
			region:   "us-isof-south-1",
			expected: "https://ec2.us-isof-south-1.csp.hci.ic.gov",
		},
		{
			name:         "custom base endpoint",
			service:      "ec2",
			region:       "us-east-1",
			baseEndpoint: "https://vpce-12345.ec2.us-east-1.vpce.amazonaws.com",
			expected:     "https://vpce-12345.ec2.us-east-1.vpce.amazonaws.com",
		},
		{
			name:         "custom base endpoint with FIPS rejected",
			service:      "ec2",
			region:       "us-east-1",
			baseEndpoint: "https://custom.local",
			useFIPS:      true,
			wantErr:      "FIPS and custom endpoint are not supported",
		},
		{
			name:         "custom base endpoint with DualStack rejected",
			service:      "ec2",
			region:       "us-east-1",
			baseEndpoint: "https://custom.local",
			useDualStack: true,
			wantErr:      "dual-stack and custom endpoint are not supported",
		},
		{
			name:         "invalid custom base endpoint",
			service:      "ec2",
			region:       "us-east-1",
			baseEndpoint: "not-a-url",
			wantErr:      "invalid custom AWS endpoint URL",
		},
		{
			name:    "missing region",
			service: "ec2",
			region:  "",
			wantErr: "missing AWS region",
		},
		{
			name:         "custom base endpoint strips query string and fragment",
			service:      "ec2",
			region:       "us-east-1",
			baseEndpoint: "https://proxy.corp.local/ec2?tenant=abc#frag",
			expected:     "https://proxy.corp.local/ec2",
		},
		{
			name:     "dotted region label allowed (matching SDK IsValidHostLabel allowSubDomains)",
			service:  "ec2",
			region:   "us.east.1",
			expected: "https://ec2.us.east.1.amazonaws.com",
		},
		{
			name:    "whitespace in region rejected",
			service: "ec2",
			region:  " us-east-1 ",
			wantErr: "invalid AWS region",
		},
		{
			name:    "invalid region label",
			service: "ec2",
			region:  "us-east-1/evil",
			wantErr: "invalid AWS region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEndpoint(tt.service, tt.region, tt.baseEndpoint, tt.useFIPS, tt.useDualStack)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNewClientEnvOverrides(t *testing.T) {
	ctx := context.Background()

	t.Run("AWS_ENDPOINT_URL_EC2 overrides AWS_ENDPOINT_URL", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "https://global.example.com")
		t.Setenv("AWS_ENDPOINT_URL_EC2", "https://ec2-specific.example.com")
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "")
		t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "")

		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
		require.NoError(t, err)

		c, err := NewClient(cfg, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)
		assert.Equal(t, "https://ec2-specific.example.com", c.Endpoint())

		// RDS should still get the global AWS_ENDPOINT_URL
		rdsClient, err := NewClient(cfg, "rds", "RDS", "2014-10-31")
		require.NoError(t, err)
		assert.Equal(t, "https://global.example.com", rdsClient.Endpoint())
	})

	t.Run("AWS_USE_FIPS_ENDPOINT and AWS_USE_DUALSTACK_ENDPOINT", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "")
		t.Setenv("AWS_ENDPOINT_URL_EC2", "")
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "true")
		t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "true")

		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
		require.NoError(t, err)

		c, err := NewClient(cfg, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)
		assert.Equal(t, "https://ec2-fips.us-east-1.api.aws", c.Endpoint())
	})

	t.Run("pseudo-FIPS region normalizes signing region", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "")
		t.Setenv("AWS_ENDPOINT_URL_EC2", "")
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "")
		t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "")

		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1-fips"))
		require.NoError(t, err)

		c, err := NewClient(cfg, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", c.Region())
		assert.Equal(t, "https://ec2-fips.us-east-1.amazonaws.com", c.Endpoint())
	})

	t.Run("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS disables custom endpoints", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "https://global.example.com")
		t.Setenv("AWS_ENDPOINT_URL_EC2", "https://ec2-specific.example.com")
		t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "true")

		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
		require.NoError(t, err)

		c, err := NewClient(cfg, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)
		assert.Equal(t, "https://ec2.us-east-1.amazonaws.com", c.Endpoint())
	})
}

func TestClientDoSigV4SigningAndXMLParsing(t *testing.T) {
	var capturedAuth, capturedDate, capturedToken, capturedContentType, capturedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/", r.URL.Path)
		capturedAuth = r.Header.Get("Authorization")
		capturedDate = r.Header.Get("X-Amz-Date")
		capturedToken = r.Header.Get("X-Amz-Security-Token")
		capturedContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = string(body)

		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<DescribeTagsResponse><tagSet><item><key>env</key><value>prod</value></item></tagSet></DescribeTagsResponse>`)
	}))
	defer ts.Close()

	cfg := aws.Config{
		Region:       "us-west-2",
		BaseEndpoint: aws.String(ts.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "session-tok-123"),
		HTTPClient:   ts.Client(),
	}

	client, err := NewClient(cfg, "ec2", "EC2", "2016-11-15")
	require.NoError(t, err)
	fixedTime := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	client.nowFunc = func() time.Time { return fixedTime }

	params := url.Values{}
	params.Set("Filter.1.Name", "resource-id")
	params.Set("Filter.1.Value.1", "i-1234567890abcdef0")

	var resp struct {
		Tags []struct {
			Key   string `xml:"key"`
			Value string `xml:"value"`
		} `xml:"tagSet>item"`
	}

	err = client.Do(context.Background(), "DescribeTags", params, &resp)
	require.NoError(t, err)

	// Verify SigV4 headers
	assert.Equal(t, "application/x-www-form-urlencoded", capturedContentType)
	assert.Equal(t, "20260315T120000Z", capturedDate)
	assert.Equal(t, "session-tok-123", capturedToken)
	assert.True(t, strings.HasPrefix(capturedAuth, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20260315/us-west-2/ec2/aws4_request, SignedHeaders="))
	assert.Contains(t, capturedAuth, "Signature=")

	// Verify form body and deterministic SHA-256 payload digest
	parsedBody, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, "DescribeTags", parsedBody.Get("Action"))
	assert.Equal(t, "2016-11-15", parsedBody.Get("Version"))
	assert.Equal(t, "resource-id", parsedBody.Get("Filter.1.Name"))
	assert.Equal(t, "i-1234567890abcdef0", parsedBody.Get("Filter.1.Value.1"))

	// Ensure SHA-256 hash of body is non-empty and deterministic
	sum := sha256.Sum256([]byte(capturedBody))
	assert.Len(t, hex.EncodeToString(sum[:]), 64)

	// Verify XML response unmarshaling
	require.Len(t, resp.Tags, 1)
	assert.Equal(t, "env", resp.Tags[0].Key)
	assert.Equal(t, "prod", resp.Tags[0].Value)
}

func TestClientDoErrorParsing(t *testing.T) {
	tests := []struct {
		name           string
		service        string
		action         string
		statusCode     int
		responseBody   string
		expectedCode   string
		expectedMsg    string
		expectedReqID  string
		expectedSubstr string
	}{
		{
			name:       "EC2 Query XML error response",
			service:    "ec2",
			action:     "DescribeTags",
			statusCode: http.StatusForbidden,
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Errors>
    <Error>
      <Code>UnauthorizedOperation</Code>
      <Message>You are not authorized to perform this operation.</Message>
    </Error>
  </Errors>
  <RequestID>ec2-req-123</RequestID>
</Response>`,
			expectedCode:   "UnauthorizedOperation",
			expectedMsg:    "You are not authorized to perform this operation.",
			expectedReqID:  "ec2-req-123",
			expectedSubstr: "UnauthorizedOperation",
		},
		{
			name:       "AWS Query (RDS) XML error response",
			service:    "rds",
			action:     "DescribeDBInstances",
			statusCode: http.StatusBadRequest,
			responseBody: `<ErrorResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>User is not authorized to perform rds:DescribeDBInstances</Message>
  </Error>
  <RequestId>rds-req-456</RequestId>
</ErrorResponse>`,
			expectedCode:   "AccessDenied",
			expectedMsg:    "User is not authorized to perform rds:DescribeDBInstances",
			expectedReqID:  "rds-req-456",
			expectedSubstr: "AccessDenied",
		},
		{
			name:           "non-XML gateway error response",
			service:        "ec2",
			action:         "DescribeTags",
			statusCode:     http.StatusBadGateway,
			responseBody:   "<html>502 Bad Gateway</html>",
			expectedMsg:    "<html>502 Bad Gateway</html>",
			expectedSubstr: "HTTP 502",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = io.WriteString(w, tt.responseBody)
			}))
			defer ts.Close()

			client, err := NewClient(aws.Config{
				Region:       "us-east-1",
				BaseEndpoint: aws.String(ts.URL),
				Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
				HTTPClient:   ts.Client(),
			}, tt.service, strings.ToUpper(tt.service), "2016-11-15")
			require.NoError(t, err)

			err = client.Do(context.Background(), tt.action, nil, nil)
			require.Error(t, err)

			var apiErr *APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, tt.statusCode, apiErr.StatusCode)
			assert.Equal(t, tt.expectedCode, apiErr.Code)
			assert.Equal(t, tt.expectedMsg, apiErr.Message)
			assert.Equal(t, tt.expectedReqID, apiErr.RequestID)
			assert.Contains(t, err.Error(), tt.expectedSubstr)
		})
	}
}

func TestClientDoFailures(t *testing.T) {
	t.Run("missing credentials provider", func(t *testing.T) {
		client, err := NewClient(aws.Config{Region: "us-east-1"}, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)

		err = client.Do(context.Background(), "DescribeTags", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing credentials provider")
	})

	t.Run("response exceeds max body size", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", 1024))
		}))
		defer ts.Close()

		client, err := NewClient(aws.Config{
			Region:       "us-east-1",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			HTTPClient:   ts.Client(),
		}, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)
		client.maxBodyBytes = 64

		err = client.Do(context.Background(), "DescribeTags", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded maximum size")
	})

	t.Run("invalid XML in 200 response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "<unclosed>")
		}))
		defer ts.Close()

		client, err := NewClient(aws.Config{
			Region:       "us-east-1",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			HTTPClient:   ts.Client(),
		}, "ec2", "EC2", "2016-11-15")
		require.NoError(t, err)

		var out struct{}
		err = client.Do(context.Background(), "DescribeTags", nil, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse XML response")
	})
}
