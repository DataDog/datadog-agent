// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	shieldedTagsKey = "tags_shielded_from_key_scrub"
	tagsFieldKey    = "tags"
)

// shieldTokenFor returns a key name that does not occur anywhere in the
// payload, so renaming "tags" to it and back cannot touch any other key.
func shieldTokenFor(payloadJSON []byte) string {
	token := shieldedTagsKey
	for i := 1; bytes.Contains(payloadJSON, []byte(token)); i++ {
		token = shieldedTagsKey + "_" + strconv.Itoa(i)
	}
	return token
}

// shieldTagsFields renames every outermost "tags" key to token. A "tags" key
// found inside the value of another "tags" key is a user label, not a payload
// field, so it keeps its name and stays subject to key-based scrubbing.
func shieldTagsFields(node any, token string, insideTags bool) any {
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			if k == tagsFieldKey && !insideTags {
				out[token] = shieldTagsFields(child, token, true)
				continue
			}
			out[k] = shieldTagsFields(child, token, insideTags || k == tagsFieldKey)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = shieldTagsFields(child, token, insideTags)
		}
		return out
	default:
		return node
	}
}

// scrubKeepingTags scrubs a JSON payload like scrubber.ScrubJSON, but never
// treats the payload's "tags" field as a sensitive key.
//
// The secrets component registers every configuration key that held a secret
// handle as a process-wide stripped key. The key-based JSON scrubber then masks
// any key with the same name in any payload and replaces its value, even an
// object or array, with a string. If one of those registered keys is named
// "tags", every "tags" field in telemetry payloads (metric tags, log tags)
// would be destroyed. Value-based scrubbing still applies to everything under
// "tags", and key-based scrubbing stays active for the labels inside it, so a
// label that is itself named "tags" is still masked.
//
// The field is shielded by renaming the outermost "tags" keys before scrubbing
// and restoring them after. The temporary name is chosen so that it does not
// appear anywhere in the payload, so a payload that already uses that name is
// left intact.
//
// This is a temporary workaround. Remove it once the scrubber supports
// excluding keys, or the secrets component registers full key paths instead
// of bare key names.
func scrubKeepingTags(payloadJSON []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(payloadJSON))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		// Not a JSON document: nothing to shield, scrub it as it is.
		return scrubber.ScrubJSON(payloadJSON)
	}

	token := shieldTokenFor(payloadJSON)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(shieldTagsFields(doc, token, false)); err != nil {
		return nil, err
	}

	scrubbed, err := scrubber.ScrubJSON(bytes.TrimRight(buf.Bytes(), "\n"))
	if err != nil {
		return nil, err
	}
	shieldedKeyRe := regexp.MustCompile(`"` + regexp.QuoteMeta(token) + `"(\s*):`)
	return shieldedKeyRe.ReplaceAll(scrubbed, []byte(`"`+tagsFieldKey+`"$1:`)), nil
}
