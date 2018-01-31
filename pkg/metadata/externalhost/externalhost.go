// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package externalhost

// externalHostCache maps hostname -> externalTags
var externalHostCache = make(map[string]ExternalTags)

// AddExternalTags adds external tags for a specific host to the cache
func AddExternalTags(hostname string, tags ExternalTags) {
	externalHostCache[hostname] = tags
}

// GetPayload fills and return the external host tags metadata payload
func GetPayload() *Payload {
	payload := Payload{}
	for hostname, tags := range externalHostCache {
		ht := hostTags{hostname, tags}
		payload = append(payload, ht)
	}

	// clear the cache
	externalHostCache = make(map[string]ExternalTags)
	return &payload
}
