// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upcloud

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const fixture = `
{
  "cloud_name": "upcloud",
  "instance_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "hostname": "test-hostname",
  "platform": "servers",
  "subplatform": "metadata (http://169.254.169.254)",
  "public_keys": [
    "ssh-ed25519 AAAA foo"
  ],
  "region": "fi-hel2",
  "network": {
    "interfaces": [
      {
        "index": 1,
        "ip_addresses": [
          {
            "address": "127.42.42.42",
            "dhcp": true,
            "dns": [
              "94.237.127.9",
              "94.237.40.9"
            ],
            "family": "IPv4",
            "floating": false,
            "gateway": "127.42.42.1",
            "network": "127.42.42.0/24"
          }
        ],
        "mac": "mm:mm:mm:mm:mm:m1",
        "network_id": "nnnnnnnn-nnnn-nnnn-nnnn-nnnnnnnnnnnn",
        "type": "public"
      },
      {
        "index": 2,
        "ip_addresses": [
          {
            "address": "127.42.42.43",
            "dhcp": true,
            "dns": [
              "94.237.127.9",
              "94.237.40.9"
            ],
            "family": "IPv4",
            "floating": false,
            "gateway": "127.42.42.1",
            "network": "127.42.42.0/24"
          }
        ],
        "mac": "mm:mm:mm:mm:mm:m2",
        "network_id": "nnnnnnnn-nnnn-nnnn-nnnn-nnnnnnnnnnnn",
        "type": "public"
      }
    ],
    "dns": [
      "94.237.127.9",
      "94.237.40.9"
    ]
  },
  "storage": {
    "disks": [
      {
        "id": "ssssssss-ssss-ssss-ssss-ssssssssssss",
        "serial": "00000000000000000000",
        "size": 25600,
        "type": "disk",
        "tier": "maxiops"
      }
    ]
  },
  "tags": [],
  "user_data": "",
  "vendor_data": ""
}
`

func TestGetHostname(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.Set("cloud_provider_metadata", holdValue)
	config.Datadog.Set("cloud_provider_metadata", []string{"upcloud"})

	expected := "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/metadata/v1/instance_id", lastRequest.URL.Path)
}

func TestGetNetworkID(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.Set("cloud_provider_metadata", holdValue)
	config.Datadog.Set("cloud_provider_metadata", []string{"upcloud"})

	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json#")
		io.WriteString(w, fixture)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetNetworkID()
	assert.Nil(t, err)
	assert.Equal(t, "nnnnnnnn-nnnn-nnnn-nnnn-nnnnnnnnnnnn", val)
	assert.Equal(t, "/metadata/v1.json", lastRequest.URL.Path)
}

func TestGetPublicIPv4(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.Set("cloud_provider_metadata", holdValue)
	config.Datadog.Set("cloud_provider_metadata", []string{"upcloud"})

	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, fixture)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetPublicIPv4()
	assert.Nil(t, err)
	assert.Equal(t, "127.42.42.42", val)
	assert.Equal(t, "/metadata/v1.json", lastRequest.URL.Path)
}

func TestGetNTPHosts(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.Set("cloud_provider_metadata", holdValue)
	config.Datadog.Set("cloud_provider_metadata", []string{"upcloud"})

	var expectedHosts []string

	actualHosts := GetNTPHosts()

	assert.Equal(t, expectedHosts, actualHosts)
}
