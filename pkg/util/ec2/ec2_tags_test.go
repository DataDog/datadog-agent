// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package ec2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	metadataURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

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
			content, err := os.ReadFile("payloads/security_cred.json")
			require.NoError(t, err, fmt.Sprintf("failed to load json in payloads/security_cred.json: %v", err))
			w.Write(content)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	cred, err := getSecurityCreds(ctx)
	require.NoError(t, err)
	assert.Equal(t, "123456", cred.AccessKeyID)
	assert.Equal(t, "secret access key", cred.SecretAccessKey)
	assert.Equal(t, "secret token", cred.Token)
}

func TestGetInstanceIdentity(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		content, err := os.ReadFile("payloads/instance_indentity.json")
		require.NoError(t, err, fmt.Sprintf("failed to load json in payloads/instance_indentity.json: %v", err))
		w.Write(content)
	}))
	defer ts.Close()
	instanceIdentityURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	val, err := GetInstanceIdentity(ctx)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", val.Region)
	assert.Equal(t, "i-aaaaaaaaaaaaaaaaa", val.InstanceID)
	assert.Equal(t, "REMOVED", val.AccountID)
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
	metadataURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	confMock := configmock.New(t)
	confMock.SetWithoutSource("exclude_ec2_tags", []string{"ExcludedTag", "OtherExcludedTag2"})

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
	metadataURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	_, err := fetchEc2TagsFromIMDS(ctx)
	require.Error(t, err)
}

func mockFetchTagsSuccess(_ context.Context) ([]string, error) {
	return []string{"tag1", "tag2"}, nil
}

func mockFetchTagsFailure(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("could not fetch tags")
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
	assert.Equal(t, fmt.Errorf("unable to get tags from aws and cache is empty: could not fetch tags"), err)
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

// Mock implementation of ec2ClientInterface
type mockEC2Client struct {
	DescribeTagsFunc func(ctx context.Context, params *ec2.DescribeTagsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error)
}

func (m *mockEC2Client) DescribeTags(ctx context.Context, params *ec2.DescribeTagsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error) {
	return m.DescribeTagsFunc(ctx, params, optFns...)
}

// Helper function to compare slices of strings
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]struct{}, len(a))
	for _, v := range a {
		aMap[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := aMap[v]; !ok {
			return false
		}
	}
	return true
}

func TestGetTagsWithCreds(t *testing.T) {
	tests := []struct {
		name             string
		instanceIdentity *EC2Identity
		mockDescribeTags func(ctx context.Context, params *ec2.DescribeTagsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error)
		expectedTags     []string
		expectedError    assert.ErrorAssertionFunc
	}{
		{
			name: "Successful retrieval of tags",
			instanceIdentity: &EC2Identity{
				InstanceID: "i-1234567890abcdef0",
			},
			mockDescribeTags: func(_ context.Context, _ *ec2.DescribeTagsInput, _ ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error) {
				return &ec2.DescribeTagsOutput{
					Tags: []types.TagDescription{
						{Key: aws.String("Name"), Value: aws.String("TestInstance")},
						{Key: aws.String("Env"), Value: aws.String("Production")},
					},
				}, nil
			},
			expectedTags:  []string{"Name:TestInstance", "Env:Production"},
			expectedError: assert.NoError,
		},
		{
			name: "Excluded tags are filtered out",
			instanceIdentity: &EC2Identity{
				InstanceID: "i-1234567890abcdef0",
			},
			mockDescribeTags: func(_ context.Context, _ *ec2.DescribeTagsInput, _ ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error) {
				return &ec2.DescribeTagsOutput{
					Tags: []types.TagDescription{
						{Key: aws.String("Name"), Value: aws.String("TestInstance")},
						{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("MyStack")},
					},
				}, nil
			},
			expectedTags:  []string{"Name:TestInstance", "aws:cloudformation:stack-name:MyStack"},
			expectedError: assert.NoError,
		},
		{
			name: "DescribeTags returns an error",
			instanceIdentity: &EC2Identity{
				InstanceID: "i-1234567890abcdef0",
			},
			mockDescribeTags: func(_ context.Context, _ *ec2.DescribeTagsInput, _ ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error) {
				return nil, errors.New("DescribeTags error")
			},
			expectedTags:  nil,
			expectedError: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the EC2 client
			mockClient := &mockEC2Client{
				DescribeTagsFunc: tt.mockDescribeTags,
			}

			// Create a background context
			ctx := context.Background()

			// Call the function under test
			tags, err := getTagsWithCreds(ctx, tt.instanceIdentity, mockClient)

			// Validate the error
			tt.expectedError(t, err)
			// Validate the tags
			if !equalStringSlices(tags, tt.expectedTags) {
				t.Fatalf("Expected tags '%v', got '%v'", tt.expectedTags, tags)
			}
		})
	}
}
