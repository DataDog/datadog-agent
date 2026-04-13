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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

func TestGetPublicIPv4(t *testing.T) {
	ctx := context.Background()
	ip := "10.0.0.2"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			switch r.RequestURI {
			case "/public-ipv4":
				io.WriteString(w, ip)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

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
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			switch r.RequestURI {
			case "/network/interfaces/macs":
				io.WriteString(w, mac+"/")
			case "/network/interfaces/macs/00:00:00:00:00/vpc-id":
				io.WriteString(w, vpc)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

	val, err := GetNetworkID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, vpc, val)
}

func TestGetInstanceIDNoMac(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			io.WriteString(w, "")
		}
	}))

	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EC2: GetNetworkID no mac addresses returned")
}

func TestGetInstanceIDMultipleVPC(t *testing.T) {
	ctx := context.Background()
	mac := "00:00:00:00:00"
	vpc := "vpc-12345"
	mac2 := "00:00:00:00:01"
	vpc2 := "vpc-6789"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
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
		}
	}))

	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many mac addresses returned")
}

func TestGetVPCSubnets(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			switch r.RequestURI {
			case "/network/interfaces/macs":
				io.WriteString(w, "00:00:00:12:34/\n")
				io.WriteString(w, "00:00:00:56:78/")
			case "/network/interfaces/macs/00:00:00:12:34/vpc-ipv4-cidr-blocks":
				io.WriteString(w, "10.1.56.0/8\n")
				io.WriteString(w, "10.1.56.1/8")
			case "/network/interfaces/macs/00:00:00:12:34/vpc-ipv6-cidr-blocks":
				io.WriteString(w, "2600::/64\n")
				io.WriteString(w, "2601::/64")
			case "/network/interfaces/macs/00:00:00:56:78/vpc-ipv4-cidr-blocks":
				io.WriteString(w, "10.1.56.2/8\n")
				io.WriteString(w, "10.1.56.3/8")
			case "/network/interfaces/macs/00:00:00:56:78/vpc-ipv6-cidr-blocks":
				io.WriteString(w, "2602::/64\n")
				io.WriteString(w, "2603::/64")
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer ts.Close()

	defer resetPackageVars()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	subnets, err := GetVPCSubnetsForHost(ctx)
	require.NoError(t, err)

	expected := []string{
		"10.1.56.0/8",
		"10.1.56.1/8",
		"2600::/64",
		"2601::/64",
		"10.1.56.2/8",
		"10.1.56.3/8",
		"2602::/64",
		"2603::/64",
	}

	// elements may come back in any order
	require.ElementsMatch(t, expected, subnets)
}

func TestGetVPCSubnets404(t *testing.T) {
	ctx := context.Background()

	var found404Subnets bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			switch r.RequestURI {
			case "/network/interfaces/macs":
				io.WriteString(w, "00:00:00:12:34/\n")
				io.WriteString(w, "00:00:00:56:78/")
			case "/network/interfaces/macs/00:00:00:12:34/vpc-ipv4-cidr-blocks":
				io.WriteString(w, "10.1.56.0/8\n")
				io.WriteString(w, "10.1.56.1/8")
			case "/network/interfaces/macs/00:00:00:12:34/vpc-ipv6-cidr-blocks":
				io.WriteString(w, "2600::/64\n")
				io.WriteString(w, "2601::/64")
			case "/network/interfaces/macs/00:00:00:56:78/vpc-ipv4-cidr-blocks":
				io.WriteString(w, "10.1.56.2/8\n")
				io.WriteString(w, "10.1.56.3/8")
			case "/network/interfaces/macs/00:00:00:56:78/vpc-ipv6-cidr-blocks":
				found404Subnets = true
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer ts.Close()

	defer resetPackageVars()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	subnets, err := GetVPCSubnetsForHost(ctx)
	require.NoError(t, err)
	// it should have checked the interface that has no ipv6 CIDRs
	require.True(t, found404Subnets)

	expected := []string{
		"10.1.56.0/8",
		"10.1.56.1/8",
		"2600::/64",
		"2601::/64",
		"10.1.56.2/8",
		"10.1.56.3/8",
	}

	// elements may come back in any order
	require.ElementsMatch(t, expected, subnets)
}
