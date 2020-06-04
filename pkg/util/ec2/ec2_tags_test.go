// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build ec2

package ec2

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIAMRole(t *testing.T) {
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
	timeout = time.Second
	defer resetPackageVars()

	val, err := getIAMRole()
	require.Nil(t, err)
	assert.Equal(t, expected, val)
}

func TestGetSecurityCreds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "test-role")
		} else if r.URL.Path == "/iam/security-credentials/test-role" {
			w.Header().Set("Content-Type", "text/plain")
			content, err := ioutil.ReadFile("payloads/security_cred.json")
			require.Nil(t, err, fmt.Sprintf("failed to load json in payloads/security_cred.json: %v", err))
			io.WriteString(w, string(content))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	cred, err := getSecurityCreds()
	require.Nil(t, err)
	assert.Equal(t, "123456", cred.AccessKeyID)
	assert.Equal(t, "secret access key", cred.SecretAccessKey)
	assert.Equal(t, "secret token", cred.Token)
}

func TestGetInstanceIdentity(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		content, err := ioutil.ReadFile("payloads/instance_indentity.json")
		require.Nil(t, err, fmt.Sprintf("failed to load json in payloads/instance_indentity.json: %v", err))
		io.WriteString(w, string(content))
	}))
	defer ts.Close()
	instanceIdentityURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	val, err := getInstanceIdentity()
	require.Nil(t, err)
	assert.Equal(t, "us-east-1", val.Region)
	assert.Equal(t, "i-aaaaaaaaaaaaaaaaa", val.InstanceID)
}

func mockFetchTagsSuccess() ([]string, error) {
	fmt.Printf("mockFetchTagsSuccess !!!!!!!!\n")
	return []string{"tag1", "tag2"}, nil
}

func mockFetchTagsFailure() ([]string, error) {
	fmt.Printf("mockFetchTagsFailure !!!!!!!!\n")
	return nil, fmt.Errorf("could not fetch tags")
}

func TestGetTags(t *testing.T) {
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	fetchTags = mockFetchTagsSuccess

	tags, err := GetTags()
	assert.Nil(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)
}

func TestGetTagsErrorEmptyCache(t *testing.T) {
	defer func() { fetchTags = fetchEc2Tags }()
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags()
	assert.Nil(t, tags)
	assert.Equal(t, fmt.Errorf("unable to get tags from aws and cache is empty: could not fetch tags"), err)
}

func TestGetTagsErrorFullCache(t *testing.T) {
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	cache.Cache.Set(tagsCacheKey, []string{"cachedTag"}, cache.NoExpiration)
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags()
	assert.Nil(t, err)
	assert.Equal(t, []string{"cachedTag"}, tags)
}

func TestGetTagsFullWorkflow(t *testing.T) {
	defer func() {
		fetchTags = fetchEc2Tags
		cache.Cache.Delete(tagsCacheKey)
	}()
	cache.Cache.Set(tagsCacheKey, []string{"oldTag"}, cache.NoExpiration)
	fetchTags = mockFetchTagsFailure

	tags, err := GetTags()
	assert.Nil(t, err)
	assert.Equal(t, []string{"oldTag"}, tags)

	fetchTags = mockFetchTagsSuccess
	tags, err = GetTags()
	assert.Nil(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)

	fetchTags = mockFetchTagsFailure
	tags, err = GetTags()
	assert.Nil(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, tags)
}
