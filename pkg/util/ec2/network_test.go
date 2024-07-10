// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	config.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	val, err := GetPublicIPv4(ctx)
	require.NoError(t, err)
	assert.Equal(t, ip, val)
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
	config.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
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
	config.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
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
	config.Datadog().SetWithoutSource("ec2_metadata_timeout", 1000)
	defer resetPackageVars()

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many mac addresses returned")
}
