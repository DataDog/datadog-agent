// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/go-tuf/client"
	"github.com/stretchr/testify/assert"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const (
	host = "test-host"
	k    = "test"
)

func generateUpdate(baseVersion uint64) *pbgo.LatestConfigsResponse {
	baseVersion *= 10000
	return &pbgo.LatestConfigsResponse{
		ConfigMetas: &pbgo.ConfigMetas{
			Roots: []*pbgo.TopMeta{
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+1)),
					Version: baseVersion + 1,
				},
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+2)),
					Version: baseVersion + 2,
				},
			},
			Timestamp: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("timestamp_content_%d", baseVersion+3)),
				Version: baseVersion + 3,
			},
			Snapshot: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("snapshot_content_%d", baseVersion+4)),
				Version: baseVersion + 4,
			},
			TopTargets: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("targets_content_%d", baseVersion+5)),
				Version: baseVersion + 5,
			},
			DelegatedTargets: []*pbgo.DelegatedMeta{
				{
					Role:    "PRODUCT1",
					Raw:     []byte(fmt.Sprintf("product1_content_%d", baseVersion+6)),
					Version: baseVersion + 6,
				},
				{
					Role:    "PRODUCT2",
					Raw:     []byte(fmt.Sprintf("product1_content_%d", baseVersion+7)),
					Version: baseVersion + 7,
				},
			},
		},
		DirectorMetas: &pbgo.DirectorMetas{
			Roots: []*pbgo.TopMeta{
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+8)),
					Version: baseVersion + 8,
				},
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+9)),
					Version: baseVersion + 9,
				},
			},
			Timestamp: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("timestamp_content_%d", baseVersion+10)),
				Version: baseVersion + 10,
			},
			Snapshot: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("snapshot_content_%d", baseVersion+11)),
				Version: baseVersion + 11,
			},
			Targets: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("targets_content_%d", baseVersion+12)),
				Version: baseVersion + 12,
			},
		},
		TargetFiles: []*pbgo.File{
			{
				Raw:  []byte(fmt.Sprintf("config_content_%d", baseVersion)),
				Path: fmt.Sprintf("2/PRODUCT1/6fd7a9e2-3893-4c41-b995-21d41836bc91/config/%d", baseVersion),
			},
			{
				Raw:  []byte(fmt.Sprintf("config_content_%d", baseVersion+1)),
				Path: fmt.Sprintf("2/PRODUCT2/ff7ae782-e418-44e4-95af-47ba3e6bfbf9/config/%d", baseVersion+1),
			},
		},
	}
}

func TestRemoteStoreConfig(t *testing.T) {
	db := newTransactionalStore(getTestDB(t))
	defer db.commit()

	targetStore := newTargetStore(db)
	store := newRemoteStoreConfig(targetStore)

	testUpdate1 := generateUpdate(1)
	targetStore.storeTargetFiles(testUpdate1.TargetFiles)
	store.update(testUpdate1)

	// Checking that timestamp is the only role allowed to perform version-less retrivals
	assertGetMeta(t, &store.remoteStore, "timestamp.json", testUpdate1.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, "root.json", nil, client.ErrNotFound{File: "root.json"})
	assertGetMeta(t, &store.remoteStore, "targets.json", nil, client.ErrNotFound{File: "targets.json"})
	assertGetMeta(t, &store.remoteStore, "snapshot.json", nil, client.ErrNotFound{File: "snapshot.json"})

	// Checking state matches update1
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version), testUpdate1.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version), testUpdate1.ConfigMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version), testUpdate1.ConfigMetas.TopTargets.Raw, nil)
	for _, root := range testUpdate1.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, delegatedTarget := range testUpdate1.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), delegatedTarget.Raw, nil)
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	testUpdate2 := generateUpdate(2)
	targetStore.storeTargetFiles(testUpdate2.TargetFiles)
	store.update(testUpdate2)

	// Checking that update1 metas got properly evicted
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version)})
	for _, delegatedTarget := range testUpdate1.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), nil, client.ErrNotFound{File: fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role)})
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	// Checking that update1 roots got retained
	for _, root := range testUpdate1.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}

	// Checking state matches update2
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate2.ConfigMetas.Timestamp.Version), testUpdate2.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate2.ConfigMetas.Snapshot.Version), testUpdate2.ConfigMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate2.ConfigMetas.TopTargets.Version), testUpdate2.ConfigMetas.TopTargets.Raw, nil)
	for _, root := range testUpdate2.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, delegatedTarget := range testUpdate2.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), delegatedTarget.Raw, nil)
	}
	for _, target := range testUpdate2.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}
}

func TestRemoteStoreDirector(t *testing.T) {
	db := newTransactionalStore(getTestDB(t))
	defer db.commit()
	targetStore := newTargetStore(db)
	store := newRemoteStoreDirector(targetStore)

	testUpdate1 := generateUpdate(1)
	targetStore.storeTargetFiles(testUpdate1.TargetFiles)
	store.update(testUpdate1)

	// Checking that timestamp is the only role allowed to perform version-less retrivals
	assertGetMeta(t, &store.remoteStore, "timestamp.json", testUpdate1.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, "root.json", nil, client.ErrNotFound{File: "root.json"})
	assertGetMeta(t, &store.remoteStore, "targets.json", nil, client.ErrNotFound{File: "targets.json"})
	assertGetMeta(t, &store.remoteStore, "snapshot.json", nil, client.ErrNotFound{File: "snapshot.json"})

	// Checking state matches update1
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version), testUpdate1.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version), testUpdate1.DirectorMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version), testUpdate1.DirectorMetas.Targets.Raw, nil)
	for _, root := range testUpdate1.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	testUpdate2 := generateUpdate(2)
	targetStore.storeTargetFiles(testUpdate2.TargetFiles)
	store.update(testUpdate2)

	// Checking that update1 metas got properly evicted
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version)})
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	// Checking that update1 roots got retained
	for _, root := range testUpdate1.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}

	// Checking state matches update2
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate2.DirectorMetas.Timestamp.Version), testUpdate2.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate2.DirectorMetas.Snapshot.Version), testUpdate2.DirectorMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate2.DirectorMetas.Targets.Version), testUpdate2.DirectorMetas.Targets.Raw, nil)
	for _, root := range testUpdate2.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, target := range testUpdate2.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}
}

func assertGetMeta(t *testing.T, store *remoteStore, path string, expectedContent []byte, expectedError error) {
	stream, size, err := store.GetMeta(path)
	if expectedError != nil {
		assert.Equal(t, expectedError, err)
		return
	}
	assert.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), size)
	content, err := io.ReadAll(stream)
	assert.NoError(t, err)
	assert.Equal(t, expectedContent, content)
}

func assertGetTarget(t *testing.T, store *remoteStore, path string, expectedContent []byte, expectedError error) {
	stream, size, err := store.GetTarget(path)
	if expectedError != nil {
		assert.Equal(t, expectedError, err)
		return
	}
	assert.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), size)
	content, err := io.ReadAll(stream)
	assert.NoError(t, err)
	assert.Equal(t, expectedContent, content)
}

type mockHTTPClient struct {
	mock.Mock
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func getRequestMatcher(storeType, p, apiKey, token string) interface{} {
	return mock.MatchedBy(func(arg interface{}) bool {
		req := arg.(*http.Request)
		return req.Method == "GET" &&
			req.URL.Scheme == "https" &&
			req.URL.Host == host &&
			req.URL.Path == "/"+path.Join("test-site", storeType, p) &&
			req.Host == host &&
			req.Header.Get("X-Dd-Api-Key") == apiKey &&
			req.Header.Get("Authorization") == token
	})
}

// TestCDNRemoteStore tests that a series of GetMeta and GetTarget invocations will make the
// correct HTTP requests and handle authz tokens correctly
func TestCDNRemoteStore(t *testing.T) {
	storeType := "director"
	root2 := "path/to/2.root.json"
	body2 := "body2"
	length := len(body2)

	// First GetMeta request should pass the api key but no token, since the remote store is freshly initialized
	apiKeyMatcher := getRequestMatcher(storeType, root2, k, "")
	httpClient := &mockHTTPClient{}

	// Response with no authz token in the response headers
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(body2)),
		ContentLength: int64(length),
	}
	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)

	cdnStore := &cdnRemoteStore{
		httpClient:     httpClient,
		host:           host,
		pathPrefix:     "test-site",
		apiKey:         k,
		repositoryType: storeType,
	}

	readCloser, contentLength, err := cdnStore.GetMeta(root2)
	require.NoError(t, err)
	require.NotNil(t, readCloser)
	require.Equal(t, int64(length), contentLength)
	content := make([]byte, length)
	n, err := readCloser.Read(content)
	require.NoError(t, err)
	require.Equal(t, length, n)
	require.Equal(t, body2, string(content))
	httpClient.AssertExpectations(t)
	require.NoError(t, readCloser.Close())

	root3 := "path/to/3.root.json"
	body3 := "body3"
	length = len(body3)
	// For the second GetMeta request, we still expect to only pass the api key, since the first request's response did not contain a token
	apiKeyMatcher = getRequestMatcher(storeType, root3, k, "")

	// Second response will include an authz token in the headers
	token := "Bearer test-token"
	resp = &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(body3)),
		ContentLength: int64(length),
		Header: http.Header{
			"X-Dd-Refreshed-Authorization": []string{token},
		},
	}

	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)
	readCloser, contentLength, err = cdnStore.GetMeta(root3)
	require.NoError(t, err)
	require.NotNil(t, readCloser)
	require.Equal(t, int64(length), contentLength)
	content = make([]byte, length)
	n, err = readCloser.Read(content)
	require.NoError(t, err)
	require.Equal(t, length, n)
	require.Equal(t, body3, string(content))
	httpClient.AssertExpectations(t)
	require.NoError(t, readCloser.Close())

	root4 := "path/to/4.root.json"
	body4 := "body4"
	resp = &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(body4)),
		ContentLength: int64(length),
	}

	// For the third and final GetMeta request, we still expect to pass both the api key and the authz token that was returned in the second response
	apiKeyAndAuthzMatcher := getRequestMatcher(storeType, root4, k, token)
	httpClient.On("Do", apiKeyAndAuthzMatcher).Return(resp, nil)

	readCloser, contentLength, err = cdnStore.GetMeta(root4)
	require.NoError(t, err)
	require.NotNil(t, readCloser)
	require.Equal(t, int64(length), contentLength)
	content = make([]byte, length)
	n, err = readCloser.Read(content)
	require.NoError(t, err)
	require.Equal(t, length, n)
	require.Equal(t, body4, string(content))
	httpClient.AssertExpectations(t)
	require.NoError(t, readCloser.Close())

	// Lastly, perform a GetTarget request to ensure that the authz token is passed along correctly, and
	// the path is correctly constructed (does not have the repository type prefix)
	target := "path/to/target/abc"
	body := "targetBody"
	length = len(body)
	targetMatcher := getRequestMatcher("", target, k, token)
	resp = &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(length),
	}
	httpClient.On("Do", targetMatcher).Return(resp, nil)
	readCloser, contentLength, err = cdnStore.GetTarget(target)
	require.NoError(t, err)
	require.NotNil(t, readCloser)
	require.Equal(t, int64(length), contentLength)
	content = make([]byte, length)
	n, err = readCloser.Read(content)
	require.NoError(t, err)
	require.Equal(t, length, n)
	require.Equal(t, body, string(content))
	httpClient.AssertExpectations(t)
	require.NoError(t, readCloser.Close())
}

func TestGetMetaNotFound(t *testing.T) {
	storeType := "director"
	root2 := "path/to/2.root.json"

	apiKeyMatcher := getRequestMatcher(storeType, root2, k, "")
	httpClient := &mockHTTPClient{}

	resp := &http.Response{
		StatusCode: http.StatusNotFound,
	}
	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)

	cdnStore := &cdnRemoteStore{
		httpClient:     httpClient,
		host:           host,
		pathPrefix:     "test-site",
		apiKey:         k,
		repositoryType: storeType,
	}

	readCloser, contentLength, err := cdnStore.GetMeta(root2)
	require.Error(t, err)
	require.ErrorIs(t, err, client.ErrNotFound{File: path.Join(storeType, root2)})
	require.Nil(t, readCloser)
	require.Equal(t, int64(0), contentLength)
	httpClient.AssertExpectations(t)
}

func TestGetMetaError(t *testing.T) {
	storeType := "director"
	root2 := "path/to/2.root.json"

	apiKeyMatcher := getRequestMatcher(storeType, root2, k, "")
	httpClient := &mockHTTPClient{}

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
	}
	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)

	cdnStore := &cdnRemoteStore{
		httpClient:     httpClient,
		host:           host,
		pathPrefix:     "test-site",
		apiKey:         k,
		repositoryType: storeType,
	}

	readCloser, contentLength, err := cdnStore.GetMeta(root2)
	require.Error(t, err)
	require.Equal(t, err.Error(), "unexpected status code "+strconv.Itoa(http.StatusInternalServerError))
	require.Nil(t, readCloser)
	require.Equal(t, int64(0), contentLength)
	httpClient.AssertExpectations(t)
}

func TestGetTargetNotFound(t *testing.T) {
	targetFile := "path/to/target/abc"

	apiKeyMatcher := getRequestMatcher("", targetFile, k, "")
	httpClient := &mockHTTPClient{}

	resp := &http.Response{
		StatusCode: http.StatusNotFound,
	}
	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)

	cdnStore := &cdnRemoteStore{
		httpClient:     httpClient,
		host:           host,
		pathPrefix:     "test-site",
		apiKey:         k,
		repositoryType: "director",
	}

	readCloser, contentLength, err := cdnStore.GetTarget(targetFile)
	require.Error(t, err)
	require.ErrorIs(t, err, client.ErrNotFound{File: targetFile})
	require.Nil(t, readCloser)
	require.Equal(t, int64(0), contentLength)
	httpClient.AssertExpectations(t)
}

func TestGetTargetError(t *testing.T) {
	targetFile := "path/to/target/abc"

	apiKeyMatcher := getRequestMatcher("", targetFile, k, "")
	httpClient := &mockHTTPClient{}

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
	}
	httpClient.On("Do", apiKeyMatcher).Return(resp, nil)

	cdnStore := &cdnRemoteStore{
		httpClient:     httpClient,
		host:           host,
		pathPrefix:     "test-site",
		apiKey:         k,
		repositoryType: "director",
	}

	readCloser, contentLength, err := cdnStore.GetTarget(targetFile)
	require.Error(t, err)
	require.Equal(t, err.Error(), "unexpected status code "+strconv.Itoa(http.StatusInternalServerError))
	require.Nil(t, readCloser)
	require.Equal(t, int64(0), contentLength)
	httpClient.AssertExpectations(t)
}
