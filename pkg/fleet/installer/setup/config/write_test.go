// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"
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
	}
}

func TestWriteConfig(t *testing.T) {
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
			name: "merge with commented keys",
			initialYAML: `
# site: oldvalue
# api_key: oldkey
`,
			config: simpleConfig{Site: "newsite", APIKey: "newkey"},
			merge:  true,
			expectedYAML: `site: newsite
api_key: newkey
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
`,
			config: simpleConfig{Site: "newsite", APIKey: "newkey"},
			merge:  true,
			expectedYAML: `# This is a config file
site: newsite
api_key: newkey
`,
		},
		{
			name: "preserves inline comments",
			initialYAML: `
# site: oldvalue # site comment
# api_key: oldkey # api comment
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
api_key: mergedkey
other: value
# end comment
`,
		},
		{
			name: "merge with commented-out key and unrelated commented lines",
			initialYAML: `
# unrelated comment
# site: oldvalue
# another comment
`,
			config: simpleConfig{Site: "merged", APIKey: ""},
			merge:  true,
			expectedYAML: `# unrelated comment
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runWriteConfigTestCase(t, tc)
		})
	}
}
