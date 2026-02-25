// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
)

type writeConfigTestCase struct {
	name         string
	initialYAML  string
	config       any
	merge        bool
	expectedYAML string
}

type simpleConfig struct {
	Site   string `yaml:"site,omitempty"`
	APIKey string `yaml:"api_key,omitempty"`
}

func runWriteConfigTestCase(t *testing.T, tc writeConfigTestCase) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	if tc.initialYAML != "" {
		if err := os.WriteFile(configPath, []byte(tc.initialYAML), 0644); err != nil {
			t.Fatalf("failed to write initial yaml: %v", err)
		}
	}

	// Call writeConfig twice to ensure that the content does not change on re-run
	// e.g. disclaimer added once or not at all
	for i := 0; i < 2; i++ {
		err := writeConfig(configPath, tc.config, 0644, tc.merge)
		if err != nil {
			t.Fatalf("writeConfig failed: %v", err)
		}

		got, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read written yaml: %v", err)
		}

		if string(got) != tc.expectedYAML {
			t.Errorf("test %q failed:\nGot:\n%s\nExpected:\n%s", tc.name, got, tc.expectedYAML)
			break
		}
	}
}

// clearDisclaimer clears disclaimerGenerated to allow for simpler test cases
func clearDisclaimer(t *testing.T) {
	originalDisclaimer := disclaimerGenerated
	t.Cleanup(func() {
		disclaimerGenerated = originalDisclaimer
	})
	disclaimerGenerated = ""
}

func TestWriteConfig(t *testing.T) {
	clearDisclaimer(t)

	testCases := []writeConfigTestCase{
		{
			name:        "write new config file",
			initialYAML: "",
			config:      simpleConfig{Site: "datadoghq.com"},
			merge:       false,
			expectedYAML: `site: datadoghq.com
`,
		},
		{
			name: "preserves file with only comments",
			initialYAML: `# site: oldvalue
# api_key: oldkey
`,
			config: simpleConfig{Site: "datadoghq.com"},
			merge:  true,
			expectedYAML: `# site: oldvalue
# api_key: oldkey
site: datadoghq.com
`,
		},
		{
			// yaml.Node treats comment-only yaml files with the --- header differently
			// than without it. In this case, it's parsed as a null scalar node,
			// and the comment is a foot comment.
			// Note that this differs from the previous test without the --- header.
			// Not sure if this is a yaml.Node bug or expected behavior.
			// If a yaml.Node update changes to the output to match then we can just update the test.
			name: "document start with only comments",
			initialYAML: `---
# comment
`,
			config: simpleConfig{Site: "datadoghq.com"},
			merge:  true,
			expectedYAML: `site: datadoghq.com

# comment
`,
		},
		{
			name: "preserves block style comments",
			initialYAML: `api_key: oldkey
###########################
## Dogfood Configuration ##
###########################
`,
			config: simpleConfig{Site: "datadoghq.com"},
			merge:  true,
			expectedYAML: `api_key: oldkey
###########################
## Dogfood Configuration ##
###########################

site: datadoghq.com
`,
		},
		{
			name: "maintains structure with CRLF line endings",
			initialYAML: strings.ReplaceAll(`api_key: oldkey
###########################
## Dogfood Configuration ##
###########################
`, "\n", "\r\n"),
			config: simpleConfig{Site: "datadoghq.com"},
			merge:  true,
			expectedYAML: `api_key: oldkey
###########################
## Dogfood Configuration ##
###########################

site: datadoghq.com
`,
		},
		{
			// We don't want to overwrite commented keys.
			// They could be old/testing values that the customer wants to preserve.
			// If we ensure our config template has specific values then we could
			// target those.
			name: "preserves commented keys",
			initialYAML: `
# site: oldvalue
# api_key: oldkey
api_key: oldvalue
`,
			config: simpleConfig{Site: "newsite", APIKey: "newkey"},
			merge:  true,
			expectedYAML: `# site: oldvalue
# api_key: oldkey
api_key: newkey
site: newsite
`,
		},
		{
			name: "preserves existing keys",
			initialYAML: `
site: datadoghq.eu
# api_key: oldkey
`,
			config: simpleConfig{Site: "datadoghq.com", APIKey: "newkey"},
			merge:  true,
			expectedYAML: `site: datadoghq.com
# api_key: oldkey

api_key: newkey
`,
		},
		{
			name: "merge adds new keys",
			initialYAML: `
site: datadoghq.com
`,
			config: simpleConfig{Site: "datadoghq.com", APIKey: "added"},
			merge:  true,
			expectedYAML: `site: datadoghq.com
api_key: added
`,
		},
		{
			name: "merge with nested config",
			initialYAML: `
site: datadoghq.com
nested:
# nested comment (will be tabbed over)
  foo: bar
`,
			config: struct {
				Site   string `yaml:"site"`
				Nested struct {
					Foo string `yaml:"foo"`
					Baz string `yaml:"baz"`
				} `yaml:"nested"`
			}{
				Site: "datadoghq.com",
				Nested: struct {
					Foo string `yaml:"foo"`
					Baz string `yaml:"baz"`
				}{Foo: "baz", Baz: "qux"},
			},
			merge: true,
			expectedYAML: `site: datadoghq.com
nested:
  # nested comment (will be tabbed over)
  foo: baz
  baz: qux
`,
		},
		{
			name: "preserves unrelated comments",
			initialYAML: `
# This is a config file
# site: oldvalue
# api_key: oldkey
api_key: oldvalue
`,
			config: simpleConfig{Site: "newsite", APIKey: "newkey"},
			merge:  true,
			expectedYAML: `# This is a config file
# site: oldvalue
# api_key: oldkey
api_key: newkey
site: newsite
`,
		},
		{
			name: "preserves inline comments",
			initialYAML: `
site: oldvalue # site comment
api_key: oldkey # api comment
`,
			config: simpleConfig{Site: "withcomment", APIKey: "withapicomment"},
			merge:  true,
			expectedYAML: `site: withcomment # site comment
api_key: withapicomment # api comment
`,
		},
		{
			name: "preserves unrelated keys and comments",
			initialYAML: `
# global comment
site: datadoghq.com
# api_key: oldkey
other: value
# end comment
`,
			config: simpleConfig{Site: "datadoghq.eu", APIKey: "mergedkey"},
			merge:  true,
			expectedYAML: `# global comment
site: datadoghq.eu
# api_key: oldkey
other: value
# end comment

api_key: mergedkey
`,
		},
		{
			name: "merge with commented-out key and unrelated commented lines",
			initialYAML: `
# unrelated comment
# site: oldvalue
site: somevalue
# another comment
`,
			config: simpleConfig{Site: "merged", APIKey: ""},
			merge:  true,
			expectedYAML: `# unrelated comment
# site: oldvalue
site: merged
# another comment
`,
		},
		{
			name: "value starts with #",
			initialYAML: `
api_key: "#value"
`,
			config: simpleConfig{Site: "datadoghq.com", APIKey: "#newkey"},
			merge:  true,
			expectedYAML: `api_key: '#newkey'
site: datadoghq.com
`,
		},
		{
			name: "empty value",
			initialYAML: `
api_key:
`,
			config: simpleConfig{Site: "datadoghq.com", APIKey: "#newkey"},
			merge:  true,
			expectedYAML: `api_key: '#newkey'
site: datadoghq.com
`,
		},
		{
			name: "comments on maps are preserved",
			initialYAML: `
# map comment
installer:
  registry:
    url: registry.com
`,
			config: DatadogConfig{
				APIKey: "newkey",
				Installer: DatadogConfigInstaller{
					Registry: DatadogConfigInstallerRegistry{
						URL: "newregistry.com",
					},
				},
			},
			merge: true,
			expectedYAML: `# map comment
installer:
  registry:
    url: newregistry.com
api_key: newkey
`,
		},
		{
			name:        "initial yaml does not have trailing newline",
			initialYAML: `site: datadoghq.com`,
			config:      simpleConfig{Site: "datadoghq.com", APIKey: "newkey"},
			merge:       true,
			expectedYAML: `site: datadoghq.com
api_key: newkey
`,
		},
		{
			name: "replaces tags block",
			initialYAML: `tags:
  - 'env:prod'
  - 'team:sre'
`,
			config: DatadogConfig{APIKey: "newkey", Tags: []string{"env:qa", "team:platform"}},
			merge:  true,
			expectedYAML: `tags:
  - env:qa
  - team:platform
api_key: newkey
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runWriteConfigTestCase(t, tc)
		})
		// run tests again with UTF-16 input
		// The config file may be UTF-16 on Windows
		t.Run(tc.name+" (UTF-16)", func(t *testing.T) {
			encoded, err := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewEncoder().String(tc.initialYAML)
			require.NoError(t, err)
			tc.initialYAML = encoded
			runWriteConfigTestCase(t, tc)
		})
	}
}

func TestWriteConfigWithDisclaimer(t *testing.T) {
	testCases := []writeConfigTestCase{
		{
			name:        "write new config file with disclaimer",
			initialYAML: "",
			config:      simpleConfig{Site: "datadoghq.com"},
			merge:       false,
			expectedYAML: disclaimerGenerated + "\n\n" + `site: datadoghq.com
`,
		},
		{
			name:        "does not add disclaimer to existing config file",
			initialYAML: `site: datadoghq.com`,
			config:      simpleConfig{Site: "datadoghq.com"},
			merge:       true,
			expectedYAML: `site: datadoghq.com
`,
		},
		{
			name: "adds disclaimer to file with only comments",
			initialYAML: `# site: oldvalue
# api_key: oldkey
`,
			config: simpleConfig{Site: "datadoghq.com"},
			merge:  true,
			expectedYAML: disclaimerGenerated + "\n\n" + `# site: oldvalue
# api_key: oldkey
site: datadoghq.com
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runWriteConfigTestCase(t, tc)
		})
	}
}

// TestEnsureUTF8 tests that the ensureUTF8 function correctly converts between encodings
//
// It tests with UTF-16LE, UTF-16LE with BOM, UTF-16BE with BOM, and UTF-8.
func TestEnsureUTF8(t *testing.T) {
	encodings := []encoding.Encoding{
		unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM),
		unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM),
		unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM),
		encoding.Nop, // keeps UTF-8 as is
	}

	input := []byte("hello world")
	for _, e := range encodings {
		// encode input to new encoding
		encoded, err := e.NewEncoder().Bytes(input)
		require.NoError(t, err)
		// convert it back to UTF-8
		output, err := ensureUTF8(encoded)
		require.NoError(t, err)
		assert.Equal(t, input, output)
		// assert output does not contain BOM
		assert.False(t, bytes.HasPrefix(output, []byte{0xFF, 0xFE}), "output should not have UTF-16LE BOM")
		assert.False(t, bytes.HasPrefix(output, []byte{0xFE, 0xFF}), "output should not have UTF-16BE BOM")
		assert.False(t, bytes.HasPrefix(output, []byte{0xEF, 0xBB, 0xBF}), "output should not have UTF-8 BOM")
	}

	// returns an error on invalid encodings
	// Administrator in ru / cp1251
	input = []byte("A\xe4\xec\xe8\xed\xe8\xf1\xf2\xf0\xe0\xf2\xee\xf0")
	_, err := ensureUTF8(input)
	assert.ErrorContains(t, err, "not valid UTF-8", "should return error for unknown encodings")
}
