// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package main

import (
	"encoding/json"
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectedCollectors_String(t *testing.T) {
	sc := &SelectedCollectors{
		"foo": struct{}{},
		"bar": struct{}{},
	}
	assert.Equal(t, "[bar foo]", sc.String())
}

// gohaiPayload defines the format we expect the gohai information
// to be in.
// Any change to this datastructure should be notified to the backend
// team to ensure compatibility.
type gohaiPayload struct {
	Filesystem []struct {
		KbSize string `json:"kb_size"`
		// MountedOn can be empty on Windows
		MountedOn string `json:"mounted_on"`
		Name      string `json:"name"`
	} `json:"filesystem"`
	Network struct {
		Interfaces []struct {
			Ipv4        []string `json:"ipv4"`
			Ipv6        []string `json:"ipv6"`
			Ipv6Network string   `json:"ipv6-network"`
			Macaddress  string   `json:"macaddress"`
			Name        string   `json:"name"`
			Ipv4Network string   `json:"ipv4-network"`
		} `json:"interfaces"`
		Ipaddress   string `json:"ipaddress"`
		Ipaddressv6 string `json:"ipaddressv6"`
		Macaddress  string `json:"macaddress"`
	} `json:"network"`
}

func TestGohaiSerialization(t *testing.T) {
	gohai, err := Collect()

	assert.NoError(t, err)

	gohaiJSON, err := json.Marshal(gohai)
	assert.NoError(t, err)

	var payload gohaiPayload
	assert.NoError(t, json.Unmarshal(gohaiJSON, &payload))

	if assert.NotEmpty(t, payload.Filesystem) {
		if runtime.GOOS != "windows" {
			// On Windows, MountedOn can be empty
			assert.NotEmpty(t, payload.Filesystem[0].MountedOn, 0)
		}
		assert.NotEmpty(t, payload.Filesystem[0].KbSize, 0)
		assert.NotEmpty(t, payload.Filesystem[0].Name, 0)
	}

	if assert.NotEmpty(t, payload.Network.Interfaces) {
		for _, itf := range payload.Network.Interfaces {
			assert.NotEmpty(t, itf.Name)
			// Some interfaces don't have MacAddresses
			//assert.NotEmpty(t, itf.Macaddress)

			if len(itf.Ipv4) == 0 && len(itf.Ipv6) == 0 {
				// Disabled interfaces won't have any IP address
				continue
			}
			if len(itf.Ipv4) == 0 {
				assert.NotEmpty(t, itf.Ipv6)
				assert.NotEmpty(t, itf.Ipv6Network)
				for _, ip := range itf.Ipv6 {
					assert.NotNil(t, net.ParseIP(ip))
				}
			} else {
				assert.NotEmpty(t, itf.Ipv4)
				assert.NotEmpty(t, itf.Ipv4Network)
				for _, ip := range itf.Ipv4 {
					assert.NotNil(t, net.ParseIP(ip))
				}
			}
		}
	}
	assert.NotEmpty(t, payload.Network.Ipaddress)
	assert.NotNil(t, net.ParseIP(payload.Network.Ipaddress))
	// Ipaddressv6 *can* be empty
	// assert.NotEmpty(t, payload.Network.Ipaddressv6)
	assert.NotEmpty(t, payload.Network.Macaddress)
}
