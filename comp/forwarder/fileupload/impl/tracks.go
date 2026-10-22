// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fileuploadimpl

import (
	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
)

// trackDesc describes where uploads for a track are sent.
type trackDesc struct {
	// trackType is the intake track; uploads go to /api/v2/<trackType>.
	trackType fileupload.TrackType
	// hostnameEndpointPrefix is prepended to the site to build the default
	// intake host, e.g. "cnm-intake." for "cnm-intake.datadoghq.com".
	hostnameEndpointPrefix string
	// endpointsConfigPrefix is the prefix of the logs-style endpoint settings
	// of the track (logs_dd_url, dd_url, additional_endpoints, ...).
	endpointsConfigPrefix string
}

func getTracks() []trackDesc {
	getters := []func() []trackDesc{
		getNetworkPcapTracks,
	}
	var descs []trackDesc
	for _, get := range getters {
		descs = append(descs, get()...)
	}
	return descs
}
