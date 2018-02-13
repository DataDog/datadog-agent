// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package externalhost

// hostname -> ExternalTags
type externalHost map[string]ExternalTags

// externalHostCache maps source_type -> externalHost
var externalHostCache = make(map[string]externalHost)

// SetExternalTags adds external tags for a specific host and source type
// to the cache.
func SetExternalTags(hostname, sourceType string, tags []string) {
	_, found := externalHostCache[sourceType]
	if !found {
		externalHostCache[sourceType] = make(externalHost)
	}

	externalHostCache[sourceType][hostname] = ExternalTags{sourceType: tags}
}

// GetPayload fills and return the external host tags metadata payload
func GetPayload() *Payload {
	payload := Payload{}
	for _, extHost := range externalHostCache {
		for hostname, tags := range extHost {
			ht := hostTags{hostname, tags}
			payload = append(payload, ht)
		}
	}

	// clear the cache
	externalHostCache = make(map[string]externalHost)
	return &payload
}
