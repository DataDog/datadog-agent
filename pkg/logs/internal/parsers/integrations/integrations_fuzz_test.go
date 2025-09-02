// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func FuzzIntegrationsParser(f *testing.F) {
	// Valid JSON with ddtags
	f.Add([]byte(`{"log":"message","ddtags":"foo:bar,env:prod"}`))
	f.Add([]byte(`{"log":"message","ddtags":""}`))
	f.Add([]byte(`{"log":"message","ddtags":"  foo:bar , env:prod  "}`))

	// Invalid ddtags type
	f.Add([]byte(`{"log":"message","ddtags":12345}`))
	f.Add([]byte(`{"log":"message","ddtags":null}`))
	f.Add([]byte(`{"log":"message","ddtags":["tag1","tag2"]}`))

	// Invalid JSON
	f.Add([]byte(`not valid json`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	parser := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Parser should return same message object
		if result != msg {
			t.Fatalf("Parser returned different message object")
		}

		if err != nil {
			// On error: content unchanged
			if string(result.GetContent()) != string(data) {
				t.Errorf("Content changed on error")
			}
			return
		}

		// On success: verify ddtags handling
		var input, output map[string]interface{}
		if json.Unmarshal(data, &input) == nil {
			if json.Unmarshal(result.GetContent(), &output) != nil {
				t.Errorf("Parser produced invalid JSON")
			}

			// Check if ddtags was properly handled
			if ddtags, exists := input["ddtags"]; exists {
				if _, isString := ddtags.(string); isString {
					// String ddtags should be removed
					if _, exists := output["ddtags"]; exists {
						t.Errorf("String ddtags not removed")
					}
				} else {
					// Non-string ddtags should remain
					if _, exists := output["ddtags"]; !exists {
						t.Errorf("Non-string ddtags removed")
					}
				}
			}
		}

		// Extracted tags should be trimmed and non-empty
		for _, tag := range result.ParsingExtra.Tags {
			if tag == "" || tag != strings.TrimSpace(tag) {
				t.Errorf("Invalid tag: %q", tag)
			}
		}
	})
}

func FuzzNormalizeTags(f *testing.F) {
	// Seed corpus with various tag formats
	f.Add("tag1,tag2,tag3")
	f.Add("  tag1  ,  tag2  ,  tag3  ")
	f.Add("tag1,,tag2")
	f.Add("")
	f.Add(",,,")
	f.Add("tag:value,env:prod")
	f.Add("tag=value,key=val")
	f.Add("\ttag1\t,\ntag2\n")
	f.Add("tag:with:colons,tag=with=equals")
	f.Add("unicode:值,emoji:😀")

	f.Fuzz(func(t *testing.T, tagsStr string) {
		tags := strings.Split(tagsStr, ",")
		normalized := normalizeTags(tags)

		// Verify invariants
		for _, tag := range normalized {
			// No empty tags
			if tag == "" {
				t.Error("normalizeTags returned empty tag")
			}

			// All tags should be trimmed
			if tag != strings.TrimSpace(tag) {
				t.Errorf("normalizeTags returned untrimmed tag: %q", tag)
			}
		}

		// Result should not have more tags than input
		if len(normalized) > len(tags) {
			t.Errorf("normalizeTags increased tag count from %d to %d", len(tags), len(normalized))
		}
	})
}
