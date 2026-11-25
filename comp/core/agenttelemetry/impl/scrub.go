// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const shieldedTagsKey = "tags_shielded_from_key_scrub"

var (
	tagsKeyRe         = regexp.MustCompile(`"tags"(\s*):`)
	shieldedTagsKeyRe = regexp.MustCompile(`"` + shieldedTagsKey + `"(\s*):`)
)

// scrubKeepingTags scrubs a JSON payload like scrubber.ScrubJSON, but never
// treats the JSON key "tags" as a sensitive key.
//
// The secrets component registers every configuration key that held a secret
// handle as a process-wide stripped key. The key-based JSON scrubber then masks
// any key with the same name in any payload and replaces its value, even an
// object or array, with a string. If one of those registered keys is named
// "tags", every "tags" field in telemetry payloads (metric tags, log tags)
// would be destroyed. Value-based scrubbing still applies to everything under
// "tags".
//
// The key is shielded by renaming it before scrubbing and restoring it after.
// Quotes inside JSON string values are always escaped, so the sequence
// `"tags"` followed by optional whitespace and a colon can only be an object
// key, never part of a value.
//
// This is a temporary workaround. Remove it once the scrubber supports
// excluding keys, or the secrets component registers full key paths instead
// of bare key names.
func scrubKeepingTags(payloadJSON []byte) ([]byte, error) {
	shielded := tagsKeyRe.ReplaceAll(payloadJSON, []byte(`"`+shieldedTagsKey+`"$1:`))
	scrubbed, err := scrubber.ScrubJSON(shielded)
	if err != nil {
		return nil, err
	}
	return shieldedTagsKeyRe.ReplaceAll(scrubbed, []byte(`"tags"$1:`)), nil
}
