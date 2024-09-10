package uptane

import (
	"github.com/DataDog/go-tuf/client"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"
)

const (
	host = "test-host"
	site = "test-site"
	k    = "test"
)

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
			req.URL.Path == "/"+path.Join(site, storeType, p) &&
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
		site:           site,
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
		site:           site,
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
		site:           site,
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
		site:           site,
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
		site:           site,
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
