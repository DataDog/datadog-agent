// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

var (
	initialTimeout     = time.Duration(config.Datadog.GetInt("ec2_metadata_timeout")) * time.Millisecond
	initialMetadataURL = metadataURL
	initialTokenURL    = tokenURL
)

func resetPackageVars() {
	config.Datadog.Set("ec2_metadata_timeout", initialTimeout)
	metadataURL = initialMetadataURL
	tokenURL = initialTokenURL
	token = httputils.NewAPIToken(getToken)

	instanceIDFetcher.Reset()
	localIPv4Fetcher.Reset()
	publicIPv4Fetcher.Reset()
	hostnameFetcher.Reset()
	networkIDFetcher.Reset()
}

func TestIsDefaultHostname(t *testing.T) {
	const key = "ec2_use_windows_prefix_detection"
	prefixDetection := config.Datadog.GetBool(key)
	defer config.Datadog.SetDefault(key, prefixDetection)

	for _, prefix := range []bool{true, false} {
		config.Datadog.SetDefault(key, prefix)

		assert.True(t, IsDefaultHostname("IP-FOO"))
		assert.True(t, IsDefaultHostname("domuarigato"))
		assert.Equal(t, prefix, IsDefaultHostname("EC2AMAZ-FOO"))
		assert.False(t, IsDefaultHostname(""))
	}
}

func TestIsDefaultHostnameForIntake(t *testing.T) {
	const key = "ec2_use_windows_prefix_detection"
	prefixDetection := config.Datadog.GetBool(key)
	config.Datadog.SetDefault(key, true)
	defer config.Datadog.SetDefault(key, prefixDetection)

	assert.True(t, IsDefaultHostnameForIntake("IP-FOO"))
	assert.True(t, IsDefaultHostnameForIntake("domuarigato"))
	assert.False(t, IsDefaultHostnameForIntake("EC2AMAZ-FOO"))
	assert.True(t, IsDefaultHostname("EC2AMAZ-FOO"))
}

func TestGetInstanceID(t *testing.T) {
	ctx := context.Background()
	expected := "i-0123456789abcdef0"
	var responseCode int
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(responseCode)
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	// API errors out, should return error
	responseCode = http.StatusInternalServerError
	val, err := GetInstanceID(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// API successful, should return API result
	responseCode = http.StatusOK
	val, err = GetInstanceID(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated now, should return the cached value even if API errors out
	responseCode = http.StatusInternalServerError
	val, err = GetInstanceID(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated, should refresh result if API call succeeds
	responseCode = http.StatusOK
	expected = "i-aaaaaaaaaaaaaaaaa"
	val, err = GetInstanceID(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")
}

func TestGetHostAliases(t *testing.T) {
	tests := []struct {
		name          string
		instanceID    string
		expectedHosts []string
	}{
		{
			name:          "Instance ID found",
			instanceID:    "i-0b22a22eec53b9321",
			expectedHosts: []string{"i-0b22a22eec53b9321"},
		},
		{
			name:          "No Instance ID found",
			expectedHosts: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				var responseCode int
				if tc.instanceID != "" {
					responseCode = http.StatusOK
				} else {
					responseCode = http.StatusInternalServerError
				}
				w.WriteHeader(responseCode)
				_, _ = io.WriteString(w, tc.instanceID)
			}))
			defer ts.Close()

			metadataURL = ts.URL
			config.Datadog.Set("ec2_metadata_timeout", 1000)
			defer resetPackageVars()

			ctx := context.Background()
			aliases, err := GetHostAliases(ctx)
			assert.Equal(t, tc.expectedHosts, aliases)
			assert.NoError(t, err)
		})
	}
}

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	expected := "ip-10-10-10-10.ec2.internal"
	var responseCode int
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(responseCode)
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	// API errors out, should return error
	responseCode = http.StatusInternalServerError
	val, err := GetHostname(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// API successful, should return hostname
	responseCode = http.StatusOK
	val, err = GetHostname(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// the internal cache is populated now, should return the cached hostname even if API errors out
	responseCode = http.StatusInternalServerError
	val, err = GetHostname(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// the internal cache is populated, should refresh result if API call succeeds
	responseCode = http.StatusOK
	expected = "ip-20-20-20-20.ec2.internal"
	val, err = GetHostname(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// clear internal cache
	hostnameFetcher.Reset()

	// ensure we get an empty string along with the error when not on EC2
	metadataURL = "foo"
	val, err = GetHostname(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")
}

func TestExtractClusterName(t *testing.T) {
	testCases := []struct {
		name string
		in   []string
		out  string
		err  error
	}{
		{
			name: "cluster name found",
			in: []string{
				"Name:myclustername-eksnodes-Node",
				"aws:autoscaling:groupName:myclustername-eks-nodes-NodeGroup-11111111",
				"aws:cloudformation:logical-id:NodeGroup",
				"aws:cloudformation:stack-id:arn:aws:cloudformation:zone:1111111111:stack/myclustername-eks-nodes/1111111111",
				"aws:cloudformation:stack-name:myclustername-eks-nodes",
				"kubernetes.io/role/master:1",
				"kubernetes.io/cluster/myclustername:owned",
			},
			out: "myclustername",
			err: nil,
		},
		{
			name: "cluster name not found",
			in: []string{
				"Name:myclustername-eksnodes-Node",
				"aws:autoscaling:groupName:myclustername-eks-nodes-NodeGroup-11111111",
				"aws:cloudformation:logical-id:NodeGroup",
				"aws:cloudformation:stack-id:arn:aws:cloudformation:zone:1111111111:stack/myclustername-eks-nodes/1111111111",
				"aws:cloudformation:stack-name:myclustername-eks-nodes",
				"kubernetes.io/role/master:1",
			},
			out: "",
			err: errors.New("unable to parse cluster name from EC2 tags"),
		},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, test.name), func(t *testing.T) {
			result, err := extractClusterName(test.in)
			assert.Equal(t, test.out, result)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestGetNetworkID(t *testing.T) {
	ctx := context.Background()
	mac := "00:00:00:00:00"
	vpc := "vpc-12345"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/network/interfaces/macs":
			io.WriteString(w, mac+"/")
		case "/network/interfaces/macs/00:00:00:00:00/vpc-id":
			io.WriteString(w, vpc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	val, err := GetNetworkID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, vpc, val)
}

func TestGetInstanceIDNoMac(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "")
	}))

	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mac addresses returned")
}

func TestGetInstanceIDMultipleVPC(t *testing.T) {
	ctx := context.Background()
	mac := "00:00:00:00:00"
	vpc := "vpc-12345"
	mac2 := "00:00:00:00:01"
	vpc2 := "vpc-6789"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/network/interfaces/macs":
			io.WriteString(w, mac+"/\n")
			io.WriteString(w, mac2+"/\n")
		case "/network/interfaces/macs/00:00:00:00:00/vpc-id":
			io.WriteString(w, vpc)
		case "/network/interfaces/macs/00:00:00:00:01/vpc-id":
			io.WriteString(w, vpc2)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many mac addresses returned")
}

func TestGetPublicIPv4(t *testing.T) {
	ctx := context.Background()
	ip := "10.0.0.2"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/public-ipv4":
			io.WriteString(w, ip)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer ts.Close()
	metadataURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	val, err := GetPublicIPv4(ctx)
	require.NoError(t, err)
	assert.Equal(t, ip, val)
}

func TestGetToken(t *testing.T) {
	ctx := context.Background()
	originalToken := "AQAAAFKw7LyqwVmmBMkqXHpDBuDWw2GnfGswTHi2yiIOGvzD7OMaWw=="
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		h := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
		if h != "" && r.Method == http.MethodPut {
			io.WriteString(w, originalToken)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer ts.Close()
	tokenURL = ts.URL
	config.Datadog.Set("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	token, err := token.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalToken, token)
}

func TestGetLocalIPv4(t *testing.T) {
	const ip = "10.0.0.2"
	const tok = "AQAAAFKw7LyqwVmmBMkqXHpDBuDWw2GnfGswTHi2yiIOGvzD7OMaWw=="
	type result struct {
		requestMethod string
		statusCode    int
		body          []byte
	}
	tests := []struct {
		name              string
		handlerFunc       http.HandlerFunc
		ec2_prefer_imdsv2 bool
		expectResults     []result
		runGetLocalIPv4   int
		expectError       bool
	}{
		{
			name:              "get a local IPv4 with a token four times, token should be cached",
			runGetLocalIPv4:   4,
			ec2_prefer_imdsv2: true,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					h := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
					if h == "" {
						w.WriteHeader(http.StatusUnauthorized)
						t.Fatal("X-aws-ec2-metadata-token-ttl-seconds is expected in a http header in this test case")
						return
					}
					w.Write([]byte(tok))
				case http.MethodGet:
					switch r.RequestURI {
					case "/local-ipv4":
						h := r.Header.Get("X-aws-ec2-metadata-token")
						if h == "" || h != tok {
							w.WriteHeader(http.StatusUnauthorized)
							t.Fatalf("X-aws-ec2-metadata-token should be %s in this test case", tok)
							return
						}
						w.Write([]byte(ip))
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				default:
					t.Fatalf("%s is not expected in this test case", r.Method)
				}
			},
			expectResults: []result{
				{http.MethodPut, http.StatusOK, []byte(tok)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
			},
		},
		{
			name:              "failed to get a token and a local IPv4",
			runGetLocalIPv4:   1,
			ec2_prefer_imdsv2: true,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					w.WriteHeader(http.StatusUnauthorized)
				case http.MethodGet:
					w.WriteHeader(http.StatusInternalServerError)
				default:
					t.Fatalf("%s is not expected in this test case", r.Method)
				}
			},
			expectResults: []result{
				{http.MethodPut, http.StatusUnauthorized, nil},
				{http.MethodGet, http.StatusInternalServerError, nil},
			},
			expectError: true,
		},
		{
			name:              "get a local IPv4 without a token four times",
			runGetLocalIPv4:   4,
			ec2_prefer_imdsv2: false,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					w.WriteHeader(http.StatusUnauthorized)
				case http.MethodGet:
					switch r.RequestURI {
					case "/local-ipv4":
						w.Write([]byte(ip))
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				default:
					t.Fatalf("%s is not expected in this test case", r.Method)
				}
			},
			expectResults: []result{
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
				{http.MethodGet, http.StatusOK, []byte(ip)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("ec2_prefer_imdsv2=%t, %s", tt.ec2_prefer_imdsv2, tt.name), func(t *testing.T) {
			config.Datadog.Set("ec2_prefer_imdsv2", tt.ec2_prefer_imdsv2)
			defer resetPackageVars()

			var actualResults []result
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rec := httptest.NewRecorder()
				tt.handlerFunc(rec, r)
				actualResults = append(actualResults, result{
					requestMethod: r.Method,
					statusCode:    rec.Code,
					body:          rec.Body.Bytes(),
				})
				w.WriteHeader(rec.Code)
				w.Write(rec.Body.Bytes())
			}))
			defer ts.Close()
			metadataURL = ts.URL
			tokenURL = ts.URL
			for i := 0; i < tt.runGetLocalIPv4; i++ {
				_, err := GetLocalIPv4()
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.Nil(t, err)
				}
			}
			assert.Equal(t, tt.expectResults, actualResults)
		})
	}
}

func TestGetNTPHosts(t *testing.T) {
	ctx := context.Background()
	expectedHosts := []string{"169.254.169.123"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	config.Datadog.Set("cloud_provider_metadata", []string{"aws"})
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}
