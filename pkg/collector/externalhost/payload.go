// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalhost

/*
The payload looks like this when SOURCE_TYPE is `vsphere`:

external_host_metadata = [
	["hostname1", {"vsphere": ["foo:val1", "bar:val1"]}],
	["hostname2", {"vsphere": ["foo:val2", "bar:val2"]}]
]
*/

// ExternalTags maps SOURCE_TYPE -> list of tags, exported to ease testing
type ExternalTags map[string][]string

// hostname -> list of externalTags
type hostTags []interface{}

// Payload handles the JSON unmarshalling of the external host tags payload
type Payload []hostTags
