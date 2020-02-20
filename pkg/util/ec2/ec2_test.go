// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ec2

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	initialTimeout     = timeout
	initialMetadataURL = metadataURL
	initialTokenURL    = tokenURL
)

func resetPackageVars() {
	timeout = initialTimeout
	metadataURL = initialMetadataURL
	tokenURL = initialTokenURL
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
	expected := "i-0123456789abcdef0"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	val, err := GetInstanceID()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")
}

func TestGetHostname(t *testing.T) {
	expected := "ip-10-10-10-10.ec2.internal"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	val, err := GetHostname()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// ensure we get an empty string along with the error when not on EC2
	metadataURL = "foo"
	val, err = GetHostname()
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
	timeout = time.Second
	defer resetPackageVars()

	val, err := GetNetworkID()
	assert.NoError(t, err)
	assert.Equal(t, vpc, val)
}

func TestGetInstanceIDNoMac(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "")
	}))

	defer ts.Close()
	metadataURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	_, err := GetNetworkID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mac addresses returned")
}

func TestGetInstanceIDMultipleVPC(t *testing.T) {
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
	timeout = time.Second
	defer resetPackageVars()

	_, err := GetNetworkID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many mac addresses returned")
}

func TestGetLocalIPv4(t *testing.T) {
	ip := "10.0.0.2"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/local-ipv4":
			io.WriteString(w, ip)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer ts.Close()
	metadataURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	ips, err := GetLocalIPv4()
	require.NoError(t, err)
	assert.Equal(t, []string{ip}, ips)
}

func TestGetToken(t *testing.T) {
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
	timeout = time.Second
	defer resetPackageVars()

	token, err := getToken()
	require.NoError(t, err)
	assert.Equal(t, originalToken, token)
}

func TestMetedataRequestWithToken(t *testing.T) {
	var requestWithoutToken *http.Request
	var requestForToken *http.Request
	var requestWithToken *http.Request
	var seq int

	ipv4 := "198.51.100.1"
	token := "AQAAAFKw7LyqwVmmBMkqXHpDBuDWw2GnfGswTHi2yiIOGvzD7OMaWw=="

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut:
			// Should be a token request
			h := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
			if h == "" {
				w.WriteHeader(http.StatusUnauthorized)
			}
			r.Header.Add("X-sequence", fmt.Sprintf("%v", seq))
			seq++
			requestForToken = r
			io.WriteString(w, token)
		case http.MethodGet:
			// Should be a metadata request
			t := r.Header.Get("X-aws-ec2-metadata-token")
			if t != token {
				r.Header.Add("X-sequence", fmt.Sprintf("%v", seq))
				seq++
				requestWithoutToken = r
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch r.RequestURI {
			case "/local-ipv4":
				r.Header.Add("X-sequence", fmt.Sprintf("%v", seq))
				seq++
				requestWithToken = r
				io.WriteString(w, ipv4)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			fmt.Println("q")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL
	tokenURL = ts.URL
	timeout = time.Second
	defer resetPackageVars()

	ips, err := GetLocalIPv4()
	require.NoError(t, err)
	assert.Equal(t, []string{ipv4}, ips)

	assert.Equal(t, "0", requestWithoutToken.Header.Get("X-sequence"))
	assert.Equal(t, "1", requestForToken.Header.Get("X-sequence"))
	assert.Equal(t, "2", requestWithToken.Header.Get("X-sequence"))
	assert.Equal(t, "", requestWithoutToken.Header.Get("X-aws-ec2-metadata-token"))
	assert.Equal(t, "/local-ipv4", requestWithoutToken.RequestURI)
	assert.Equal(t, http.MethodGet, requestWithoutToken.Method)
	assert.Equal(t, "60", requestForToken.Header.Get("X-aws-ec2-metadata-token-ttl-seconds"))
	assert.Equal(t, http.MethodPut, requestForToken.Method)
	assert.Equal(t, "/", requestForToken.RequestURI)
	assert.Equal(t, token, requestWithToken.Header.Get("X-aws-ec2-metadata-token"))
	assert.Equal(t, "/local-ipv4", requestWithToken.RequestURI)
	assert.Equal(t, http.MethodGet, requestWithToken.Method)
}
