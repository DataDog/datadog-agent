// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package ec2

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDefaultHostname(t *testing.T) {
	assert.True(t, IsDefaultHostname("IP-FOO"))
	assert.True(t, IsDefaultHostname("domuarigato"))
	assert.False(t, IsDefaultHostname(""))
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

	val, err := GetNetworkID()
	assert.NoError(t, err)
	assert.Equal(t, vpc, val)
}

func TestGetInstanceIDNoMac(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
	}))

	defer ts.Close()
	metadataURL = ts.URL

	_, err := GetNetworkID()
	assert.Error(t, err)
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

	_, err := GetNetworkID()
	assert.Error(t, err)
}
