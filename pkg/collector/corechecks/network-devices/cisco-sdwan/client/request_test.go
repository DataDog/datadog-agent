// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/fixtures"
)

func TestNewRequest(t *testing.T) {
	tests := []struct {
		name          string
		host          string
		method        string
		uri           string
		expectedError string
	}{
		{
			name:   "get request",
			host:   "testserver",
			method: "GET",
			uri:    "/test-endpoint",
		},
		{
			name:   "post request",
			host:   "testserver2",
			method: "POST",
			uri:    "/post-endpoint",
		},
		{
			name:          "invalid method",
			host:          "testserver2",
			method:        "HELLO?",
			uri:           "/endpoint",
			expectedError: "invalid method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.host, "testuser", "testpassword", false)
			require.NoError(t, err)

			req, err := client.newRequest(tt.method, tt.uri, nil)

			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.method, req.Method)
			require.Equal(t, tt.host, req.URL.Host)
			require.Equal(t, tt.uri, req.URL.RequestURI())
		})
	}
}

func TestDoRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	req, err := http.NewRequest("GET", "http://"+serverURL(server)+"/test", nil)
	require.NoError(t, err)

	body, statusCode, err := client.do(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, []byte(""), body)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestDoRequestBadRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("an error occurred"))
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	req, err := http.NewRequest("GET", "http://"+serverURL(server)+"/test", nil)
	require.NoError(t, err)

	body, statusCode, err := client.do(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, []byte("an error occurred"), body)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestDoRequestError(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	// Create request with invalid URL
	req, err := http.NewRequest("GET", "", nil)
	require.NoError(t, err)

	body, statusCode, err := client.do(req)

	require.ErrorContains(t, err, "unsupported protocol scheme")
	require.Equal(t, 0, statusCode)
	require.Equal(t, []byte(nil), body)
	require.Equal(t, 0, handler.numberOfCalls())
}

func TestGetRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		test := query.Get("test")
		test2 := query.Get("test2")

		// URL params are correctly set
		require.Equal(t, "param", test)
		require.Equal(t, "param2", test2)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	params := map[string]string{
		"test":  "param",
		"test2": "param2",
	}

	resp, err := client.get("/test", params)
	require.NoError(t, err)
	require.Equal(t, []byte{}, resp)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetRequestRetries(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("an error occurred"))
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set max retries to 10 for testing
	client.maxAttempts = 10

	resp, err := client.get("/test", nil)
	require.ErrorContains(t, err, "http responded with 400 code")
	require.Equal(t, []byte(nil), resp)
	require.Equal(t, 10, handler.numberOfCalls())
}

func TestGetRequestUnmarshalling(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/test", fixtures.FakePayload(fixtures.GetDevices))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	resp, err := get[Device](client, "/test", nil)
	require.NoError(t, err)
	require.Equal(t, "10.10.1.1", resp.Data[0].DeviceID)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetRequestUnmarshallingError(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/test", fixtures.FakePayload(`
[
	{
		"lastupdated": "1.0"
	}
]
`))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	resp, err := get[Device](client, "/test", nil)
	require.ErrorContains(t, err, "cannot unmarshal string into Go struct field Device.data.lastupdated of type float64")

	var empty *Response[Device]
	require.Equal(t, empty, resp)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetMoreEntriesMaxPages(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		// Always respond saying that more entries are available
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"startId": "31",
        			"endId": "35",
        			"moreEntries": true,
        			"count": 4
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set max pages to 20 for testing
	client.maxPages = 20

	params := make(map[string]string)
	pageInfo := PageInfo{
		StartID:     "1",
		EndID:       "15",
		MoreEntries: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", params, pageInfo)
	require.ErrorContains(t, err, "max number of page reached")

	// Ensure endpoint has been called 20 times
	require.Equal(t, 20, handler.numberOfCalls())
}

func TestGetMoreEntriesIndexPagination(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		startID := r.URL.Query().Get("startId")
		if calls == 1 {
			// First call, expect startId to be 16

			require.Equal(t, "16", startID, "startId should be correct")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				{
  					"pageInfo": {
        				"startId": "16",
						"endId": "30",
        				"moreEntries": true,
        				"count": 14
					}
				}
			`))
			return
		}

		// Second call, expect startId to be 31
		require.Equal(t, "31", startID, "startId should be correct")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"startId": "31",
        			"endId": "35",
        			"moreEntries": false,
        			"count": 4
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	params := make(map[string]string)
	pageInfo := PageInfo{
		StartID:     "1",
		EndID:       "15",
		MoreEntries: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", params, pageInfo)
	require.NoError(t, err)

	// Ensure endpoint has been called 2 times
	require.Equal(t, 2, handler.numberOfCalls())
}

func TestGetMoreEntriesScrollPagination(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		scrollID := r.URL.Query().Get("scrollId")
		if calls == 1 {
			// First call, expect scrollId to be "test"

			require.Equal(t, "test", scrollID, "scrollId should be correct")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				{
  					"pageInfo": {
        				"scrollId": "test2",
        				"hasMoreData": true
					}
				}
			`))
			return
		}

		// Second call, expect scrollId to be "test2"
		require.Equal(t, "test2", scrollID, "scrollId should be correct")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"scrollId": "test2",
					"hasMoreData": false
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	params := make(map[string]string)
	pageInfo := PageInfo{
		ScrollID:    "test",
		HasMoreData: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", params, pageInfo)
	require.NoError(t, err)

	// Ensure endpoint has been called 2 times
	require.Equal(t, 2, handler.numberOfCalls())
}

func TestUpdatePaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		pageInfo       PageInfo
		expectedParams map[string]string
		expectedError  string
	}{
		{
			name: "index based pagination",
			pageInfo: PageInfo{
				StartID:     "0",
				EndID:       "10",
				MoreEntries: true,
				Count:       10,
			},
			expectedParams: map[string]string{"startId": "11"},
		},
		{
			name: "scroll based pagination",
			pageInfo: PageInfo{
				HasMoreData: true,
				ScrollID:    "test",
				Count:       10,
			},
			expectedParams: map[string]string{"scrollId": "test"},
		},
		{
			name: "index based pagination with invalid end id",
			pageInfo: PageInfo{
				StartID:     "0",
				EndID:       "invalid id",
				MoreEntries: true,
				Count:       10,
			},
			expectedError:  "invalid syntax",
			expectedParams: make(map[string]string),
		},
		{
			name:           "invalid page info",
			pageInfo:       PageInfo{},
			expectedParams: make(map[string]string),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := make(map[string]string)
			err := updatePaginationParams(tt.pageInfo, params)
			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
			}
			require.Equal(t, tt.expectedParams, params)
		})
	}
}

func TestIsValidStatusCode(t *testing.T) {
	tests := []struct {
		code    int
		isValid bool
	}{
		{
			code:    200,
			isValid: true,
		},
		{
			code:    201,
			isValid: true,
		},
		{
			code:    299,
			isValid: true,
		},
		{
			code:    399,
			isValid: true,
		},
		{
			code:    400,
			isValid: false,
		},
		{
			code:    401,
			isValid: false,
		},
		{
			code:    1839849230,
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run("is valid status code", func(t *testing.T) {
			require.Equal(t, tt.isValid, isValidStatusCode(tt.code))
		})
	}
}
