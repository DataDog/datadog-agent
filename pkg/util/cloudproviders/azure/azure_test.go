// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestGetAlias(t *testing.T) {
	ctx := context.Background()
	expected := "5d33a910-a7a0-4443-9f01-6a807801b29b"

	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	aliases, err := GetHostAliases(ctx)
	assert.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, expected, aliases[0])
	assert.Equal(t, lastRequest.URL.Path, "/metadata/instance/compute/vmId")
	assert.Equal(t, lastRequest.URL.RawQuery, "api-version=2017-04-02&format=text")
}

func TestGetClusterName(t *testing.T) {
	tests := []struct {
		name    string
		rgName  string
		want    string
		wantErr bool
	}{
		{
			name:    "uppercase prefix",
			rgName:  "MC_aks-kenafeh_aks-kenafeh-eu_westeurope",
			want:    "aks-kenafeh-eu",
			wantErr: false,
		},
		{
			name:    "lowercase prefix",
			rgName:  "mc_foo-bar-aks-k8s-rg_foo-bar-aks-k8s_westeurope",
			want:    "foo-bar-aks-k8s",
			wantErr: false,
		},
		{
			name:    "invalid",
			rgName:  "unexpected-resource-group-name-format",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var lastRequest *http.Request
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				io.WriteString(w, tt.rgName)
				lastRequest = r
			}))
			defer ts.Close()
			metadataURL = ts.URL
			got, err := GetClusterName(ctx)
			assert.Equal(t, tt.wantErr, (err != nil))
			assert.Equal(t, tt.want, got)
			assert.Equal(t, lastRequest.URL.Path, "/metadata/instance/compute/resourceGroupName")
			assert.Equal(t, lastRequest.URL.RawQuery, "api-version=2017-08-01&format=text")
		})
	}
}

func TestGetNTPHosts(t *testing.T) {
	ctx := context.Background()
	expectedHosts := []string{"time.windows.com"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	pkgconfigsetup.Datadog().SetWithoutSource("cloud_provider_metadata", []string{"azure"})
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"name": "vm-name",
			"resourceGroupName": "my-resource-group",
			"subscriptionId": "2370ac56-5683-45f8-a2d4-d1054292facb",
			"vmId": "b33fa46-6aff-4dfa-be0a-9e922ca3ac6d"
		}`)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	cases := []struct {
		style, value string
		err          bool
	}{
		{"os", "", true},
		{"vmid", "b33fa46-6aff-4dfa-be0a-9e922ca3ac6d", false},
		{"name", "vm-name", false},
		{"name_and_resource_group", "vm-name.my-resource-group", false},
		{"full", "vm-name.my-resource-group.2370ac56-5683-45f8-a2d4-d1054292facb", false},
		{"invalid", "", true},
	}

	mockConfig := configmock.New(t)

	for _, tt := range cases {
		mockConfig.SetWithoutSource(hostnameStyleSetting, tt.style)
		hostname, err := getHostnameWithConfig(ctx, mockConfig)
		assert.Equal(t, tt.value, hostname)
		assert.Equal(t, tt.err, (err != nil))
	}
}

func TestGetHostnameWithInvalidMetadata(t *testing.T) {
	ctx := context.Background()
	mockConfig := configmock.New(t)

	styles := []string{"vmid", "name", "name_and_resource_group", "full"}

	for _, response := range []string{"", "!"} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, fmt.Sprintf(`{
				"name": "%s",
				"resourceGroupName": "%s",
				"subscriptionId": "%s",
				"vmId": "%s"
			}`, response, response, response, response))
		}))
		metadataURL = ts.URL

		t.Run(fmt.Sprintf("with response '%s'", response), func(t *testing.T) {
			for _, style := range styles {
				mockConfig.SetWithoutSource(hostnameStyleSetting, style)
				hostname, err := getHostnameWithConfig(ctx, mockConfig)
				assert.Empty(t, hostname)
				assert.NotNil(t, err)
			}
		})

		ts.Close()
	}
}

func TestGetPublicIPv4(t *testing.T) {
	var lastRequest *http.Request

	ctx := context.Background()
	pathPrefix := "/metadata/instance/network/interface/0/ipv4/ipAddress/0/publicIpAddress"
	expected := "8.8.8.8"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(pathPrefix, r.URL.Path) {
			io.WriteString(w, expected)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

		lastRequest = r
	}))

	defer ts.Close()

	metadataURL = ts.URL
	val, err := GetPublicIPv4(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.True(t, strings.HasPrefix(lastRequest.URL.Path, pathPrefix))
}
