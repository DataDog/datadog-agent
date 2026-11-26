// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package tags

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	awsquery "github.com/DataDog/datadog-agent/pkg/util/aws/query"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

func TestGetIAMRole(t *testing.T) {
	ctx := context.Background()
	const expected = "test-role"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, expected)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	val, err := getIAMRole(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
}

func TestGetSecurityCreds(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "test-role")
		} else if r.URL.Path == "/iam/security-credentials/test-role" {
			w.Header().Set("Content-Type", "text/plain")
			content, err := os.ReadFile("testdata/payloads/security_cred.json")
			require.NoError(t, err, fmt.Sprintf("failed to load json in testdata/payloads/security_cred.json: %v", err))
			w.Write(content)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	assert.EventuallyWithT(
		t, func(_ *assert.CollectT) {
			cred, err := getSecurityCreds(ctx)
			require.NoError(t, err)
			assert.Equal(t, "123456", cred.AccessKeyID)
			assert.Equal(t, "secret access key", cred.SecretAccessKey)
			assert.Equal(t, "secret token", cred.Token)
		},
		10*time.Second, 1*time.Second)
}

func TestFetchEc2TagsFromIMDS(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/tags/instance":
			io.WriteString(w, "Name\nPurpose\nExcludedTag") // no trailing newline
		case "/tags/instance/Name":
			io.WriteString(w, "some-vm")
		case "/tags/instance/Purpose":
			io.WriteString(w, "mining")
		case "/tags/instance/ExcludedTag":
			io.WriteString(w, "testing")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)
	conf.SetInTest("exclude_ec2_tags", []string{"ExcludedTag", "OtherExcludedTag2"})

	tags, err := fetchEc2TagsFromIMDS(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Name:some-vm",
		"Purpose:mining",
	}, tags)
}

func TestFetchEc2TagsFromIMDSError(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	_, err := fetchEc2TagsFromIMDS(ctx)
	require.Error(t, err)
}

func mockFetchTagsSuccess(_ context.Context) ([]string, error) {
	return []string{"tag1", "tag2"}, nil
}

func mockFetchTagsFailure(_ context.Context) ([]string, error) {
	return nil, errors.New("could not fetch tags")
}

func TestGetTags(t *testing.T) {
	ctx := context.Background()
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	fetchTags = mockFetchTagsSuccess

	tags, err := GetTags(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)
}

func TestGetTagsErrorEmptyCache(t *testing.T) {
	ctx := context.Background()
	defer func() { fetchTags = fetchEc2Tags }()
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags(ctx)
	assert.Nil(t, tags)
	assert.Equal(t, errors.New("unable to get tags from aws and cache is empty: could not fetch tags"), err)
}

func TestGetTagsErrorFullCache(t *testing.T) {
	ctx := context.Background()
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	cache.Cache.Set(tagsCacheKey, []string{"cachedTag"}, cache.NoExpiration)
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"cachedTag"}, tags)
}

func TestGetTagsFullWorkflow(t *testing.T) {
	ctx := context.Background()
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	cache.Cache.Set(tagsCacheKey, []string{"oldTag"}, cache.NoExpiration)
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"oldTag"}, tags)

	fetchTags = mockFetchTagsSuccess
	tags, err = GetTags(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)

	fetchTags = mockFetchTagsFailure
	tags, err = GetTags(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)
}

func TestCollectEC2InstanceInfo(t *testing.T) {
	conf := configmock.New(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/latest/api/token":
			io.WriteString(w, "some-secret-token") // no trailing newline
		case "/latest/dynamic/instance-identity/document":
			if conf.GetBool("ec2_prefer_imdsv2") {
				assert.Equal(t, r.Header["X-Aws-Ec2-Metadata-Token"], []string{"some-secret-token"})
			}
			io.WriteString(w, `{
  "accountId" : "123456abcdef",
  "architecture" : "x86_64",
  "availabilityZone" : "eu-west-3a",
  "billingProducts" : null,
  "devpayProductCodes" : null,
  "marketplaceProductCodes" : null,
  "imageId" : "ami-aaaaaaaaaaaaaaaaa",
  "instanceId" : "i-aaaaaaaaaaaaaaaaa",
  "instanceType" : "t2.medium",
  "kernelId" : null,
  "pendingTime" : "2025-05-06T10:04:40Z",
  "privateIp" : "123.12.1.123",
  "ramdiskId" : null,
  "region" : "eu-west-3",
  "version" : "2017-09-30"
}`) // no trailing newline
		default:
			fmt.Printf("%s\n", r.RequestURI)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	defer func(url string) { ec2internal.InstanceIdentityURL = url }(ec2internal.InstanceIdentityURL)
	defer func(url string) { ec2internal.TokenURL = url }(ec2internal.TokenURL)

	ec2internal.InstanceIdentityURL = ts.URL + "/latest/dynamic/instance-identity/document"
	ec2internal.TokenURL = ts.URL + "/latest/api/token"

	conf.SetInTest("collect_ec2_instance_info", true)

	// Enable ECS EC2 feature and mock ARN fetch
	env.SetFeatures(t, env.ECSEC2)
	oldFetchARN := fetchContainerInstanceARN
	fetchContainerInstanceARN = func(_ context.Context) (string, error) {
		return "arn:aws:ecs:region:account:container-instance/ci-123", nil
	}
	t.Cleanup(func() { fetchContainerInstanceARN = oldFetchARN })

	oldIsSpot := isSpotInstance
	isSpotInstance = func(_ context.Context) (bool, error) { return true, nil }
	t.Cleanup(func() { isSpotInstance = oldIsSpot })

	tags, err := GetInstanceInfo(context.Background())
	require.NoError(t, err)

	expected := []string{
		"region:eu-west-3",
		"instance-type:t2.medium",
		"aws_account:123456abcdef",
		"image:ami-aaaaaaaaaaaaaaaaa",
		"availability-zone:eu-west-3a",
		"container_instance_arn:arn:aws:ecs:region:account:container-instance/ci-123",
		"capacity-type:spot",
	}
	assert.Equal(t, expected, tags)

	ec2Info, found := cache.Cache.Get(infoCacheKey)
	assert.True(t, found)
	assert.Equal(t, expected, ec2Info.([]string))

	conf.SetInTest("collect_ec2_instance_info", false)
	tags, err = GetInstanceInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string(nil), tags)
}

func TestCreateEC2Client(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		region      string
		withCreds   bool
		expectError bool
	}{
		{
			name:        "default credential chain (IRSA-compatible)",
			region:      "us-east-1",
			withCreds:   false,
			expectError: false,
		},
		{
			name:        "explicit credentials (fallback path)",
			region:      "us-west-2",
			withCreds:   true,
			expectError: false,
		},
		{
			name:        "different region with default chain",
			region:      "eu-west-1",
			withCreds:   false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var creds aws.CredentialsProvider
			if tt.withCreds {
				creds = credentials.NewStaticCredentialsProvider("key", "secret", "token")
			}
			client, err := createEC2Client(ctx, tt.region, creds)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func setupTestIMDS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/latest/api/token":
			io.WriteString(w, "test-token")
		case "/latest/dynamic/instance-identity/document":
			io.WriteString(w, `{"instanceId": "i-test", "region": "us-east-1"}`)
		case "/iam/security-credentials/":
			io.WriteString(w, "test-role")
		case "/iam/security-credentials/test-role":
			content, _ := os.ReadFile("testdata/payloads/security_cred.json")
			w.Write(content)
		}
	}))

	ec2internal.InstanceIdentityURL = ts.URL + "/latest/dynamic/instance-identity/document"
	ec2internal.TokenURL = ts.URL + "/latest/api/token"
	ec2internal.MetadataURL = ts.URL

	t.Cleanup(func() { ts.Close() })
}

func TestFetchEc2TagsFromAPIFallback(t *testing.T) {
	ctx := context.Background()
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	defer func() {
		getTagsWithClientFunc = getTagsWithClient
		createEC2ClientFunc = createEC2Client
	}()

	t.Run("tries default credentials first (IRSA), no fallback needed", func(t *testing.T) {
		setupTestIMDS(t)

		callCount := 0
		createEC2ClientFunc = func(ctx context.Context, region string, creds aws.CredentialsProvider) (*awsquery.Client, error) {
			callCount++
			return createEC2Client(ctx, region, creds)
		}

		getTagsWithClientFunc = func(_ context.Context, _ *awsquery.Client, _ *ec2internal.EC2Identity) ([]string, error) {
			return []string{"Name:test"}, nil
		}

		tags, err := fetchEc2TagsFromAPI(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"Name:test"}, tags)
		assert.Equal(t, 1, callCount, "should only create client once")
	})

	t.Run("falls back to instance credentials when default fails", func(t *testing.T) {
		setupTestIMDS(t)

		callCount := 0
		createEC2ClientFunc = func(ctx context.Context, region string, creds aws.CredentialsProvider) (*awsquery.Client, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("mock no credentials available")
			}
			return createEC2Client(ctx, region, creds)
		}

		getTagsWithClientFunc = func(_ context.Context, _ *awsquery.Client, _ *ec2internal.EC2Identity) ([]string, error) {
			return []string{"Name:test"}, nil
		}

		tags, err := fetchEc2TagsFromAPI(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"Name:test"}, tags)
		assert.Equal(t, 2, callCount, "should try twice: default credentials fail, then fallback succeeds")
	})
}

func TestGetTagsWithClient(t *testing.T) {
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 2000)
	conf.SetInTest("exclude_ec2_tags", []string{"ExcludedTag"})

	t.Run("single page with SigV4 signing and tag exclusion", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 "))
			assert.NotEmpty(t, r.Header.Get("X-Amz-Date"))
			assert.Equal(t, "test-token", r.Header.Get("X-Amz-Security-Token"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			q, err := url.ParseQuery(string(body))
			require.NoError(t, err)
			assert.Equal(t, "DescribeTags", q.Get("Action"))
			assert.Equal(t, "2016-11-15", q.Get("Version"))
			assert.Equal(t, "resource-id", q.Get("Filter.1.Name"))
			assert.Equal(t, "i-0123456789abcdef0", q.Get("Filter.1.Value.1"))

			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<DescribeTagsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-1</requestId>
  <tagSet>
    <item>
      <resourceId>i-0123456789abcdef0</resourceId>
      <resourceType>instance</resourceType>
      <key>Name</key>
      <value>web-01</value>
    </item>
    <item>
      <resourceId>i-0123456789abcdef0</resourceId>
      <resourceType>instance</resourceType>
      <key>ExcludedTag</key>
      <value>secret</value>
    </item>
    <item>
      <resourceId>i-0123456789abcdef0</resourceId>
      <resourceType>instance</resourceType>
      <key>EmptyVal</key>
      <value></value>
    </item>
  </tagSet>
</DescribeTagsResponse>`)
		}))
		defer ts.Close()

		client, err := awsquery.NewClient(aws.Config{
			Region:       "us-east-1",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", "test-token"),
			HTTPClient:   ts.Client(),
		}, ec2ServiceName, ec2SDKID, ec2APIVersion)
		require.NoError(t, err)

		tags, err := getTagsWithClient(context.Background(), client, &ec2internal.EC2Identity{
			Region:     "us-east-1",
			InstanceID: "i-0123456789abcdef0",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"Name:web-01", "EmptyVal:"}, tags)
	})

	t.Run("paginates across NextToken pages", func(t *testing.T) {
		pageCalls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pageCalls++
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			q, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			w.Header().Set("Content-Type", "text/xml")
			switch q.Get("NextToken") {
			case "":
				_, _ = io.WriteString(w, `<DescribeTagsResponse>
  <tagSet>
    <item><key>env</key><value>prod</value></item>
  </tagSet>
  <nextToken>page-2-token</nextToken>
</DescribeTagsResponse>`)
			case "page-2-token":
				_, _ = io.WriteString(w, `<DescribeTagsResponse>
  <tagSet>
    <item><key>service</key><value>api</value></item>
  </tagSet>
</DescribeTagsResponse>`)
			default:
				t.Fatalf("unexpected NextToken %q", q.Get("NextToken"))
			}
		}))
		defer ts.Close()

		client, err := awsquery.NewClient(aws.Config{
			Region:       "us-west-2",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
			HTTPClient:   ts.Client(),
		}, ec2ServiceName, ec2SDKID, ec2APIVersion)
		require.NoError(t, err)

		tags, err := getTagsWithClient(context.Background(), client, &ec2internal.EC2Identity{
			Region:     "us-west-2",
			InstanceID: "i-page-test",
		})
		require.NoError(t, err)
		assert.Equal(t, 2, pageCalls)
		assert.Equal(t, []string{"env:prod", "service:api"}, tags)
	})

	t.Run("parses EC2 XML API error response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Errors>
    <Error>
      <Code>UnauthorizedOperation</Code>
      <Message>You are not authorized to perform this operation.</Message>
    </Error>
  </Errors>
  <RequestID>req-denied</RequestID>
</Response>`)
		}))
		defer ts.Close()

		client, err := awsquery.NewClient(aws.Config{
			Region:       "us-east-1",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
			HTTPClient:   ts.Client(),
		}, ec2ServiceName, ec2SDKID, ec2APIVersion)
		require.NoError(t, err)

		_, err = getTagsWithClient(context.Background(), client, &ec2internal.EC2Identity{
			Region:     "us-east-1",
			InstanceID: "i-denied",
		})
		require.Error(t, err)
		var apiErr *awsquery.APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, "UnauthorizedOperation", apiErr.Code)
		assert.Equal(t, "You are not authorized to perform this operation.", apiErr.Message)
		assert.Equal(t, "req-denied", apiErr.RequestID)
	})

	t.Run("end-to-end fetchEc2TagsFromAPI via AWS_ENDPOINT_URL_EC2", func(t *testing.T) {
		setupTestIMDS(t)

		ec2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.Header.Get("Authorization"), "Credential=AKIAENVTEST/")
			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w, `<DescribeTagsResponse><tagSet><item><key>role</key><value>worker</value></item></tagSet></DescribeTagsResponse>`)
		}))
		defer ec2Server.Close()

		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAENVTEST")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRETENVTEST")
		t.Setenv("AWS_ENDPOINT_URL_EC2", ec2Server.URL)
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "")
		t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "")

		tags, err := fetchEc2TagsFromAPI(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"role:worker"}, tags)
	})

	t.Run("honors ec2_metadata_timeout", func(t *testing.T) {
		conf.SetInTest("ec2_metadata_timeout", 50)
		defer conf.SetInTest("ec2_metadata_timeout", 2000)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer ts.Close()

		client, err := awsquery.NewClient(aws.Config{
			Region:       "us-east-1",
			BaseEndpoint: aws.String(ts.URL),
			Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
			HTTPClient:   ts.Client(),
		}, ec2ServiceName, ec2SDKID, ec2APIVersion)
		require.NoError(t, err)

		_, err = getTagsWithClient(context.Background(), client, &ec2internal.EC2Identity{
			Region:     "us-east-1",
			InstanceID: "i-slow",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
