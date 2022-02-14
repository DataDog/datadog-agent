package obfuscate

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObfuscateAppSec(t *testing.T) {
	for _, tc := range []struct {
		name           string
		keyRE, valueRE *regexp.Regexp
		value          string
		expected       string
	}{
		{
			// The key regexp should take precedence over the value regexp and obfuscate the entire values
			name:     "sensitive-key",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["?","?","?"],"value":"?"}]}]}]}`,
		},
		{
			// The key regexp should take precedence over the value regexp and obfuscate the entire values
			name:     "sensitive-key",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`^$`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["?","?","?"],"value":"?"}]}]}]}`,
		},
		{
			// The key regexp doesn't match and the value regexp does and obfuscates accordingly.
			name:     "sensitive-value",
			keyRE:    regexp.MustCompile(`^$`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE value 1","highlighted value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted ? value 1","highlighted value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "disabled",
			keyRE:    regexp.MustCompile(`^$`),
			valueRE:  regexp.MustCompile(`^$`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE value 1","highlighted value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE value 1","highlighted value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-empty-string",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    ``,
			expected: ``,
		},
		{
			name:     "unexpected-json-empty-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    ``,
			expected: ``,
		},
		{
			name:     "unexpected-json-null-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `null`,
			expected: `null`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `""`,
			expected: `""`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{}`,
			expected: `{}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":"not an array"}`,
			expected: `{"triggers":"not an array"}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":["not a struct"]}`,
			expected: `{"triggers":["not a struct"]}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches": "not an array"}]}`,
			expected: `{"triggers":[{"rule_matches": "not an array"}]}`,
		}, {

			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches": ["not a struct"]}]}`,
			expected: `{"triggers":[{"rule_matches": ["not a struct"]}]}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches": [{"parameters":{}}]}]}`,
			expected: `{"triggers":[{"rule_matches": [{"parameters":{}}]}]}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches": [{"parameters":"not an array"}]}]}`,
			expected: `{"triggers":[{"rule_matches": [{"parameters":"not an array"}]}]}`,
		},
		{
			name:     "unexpected-json-value",
			keyRE:    regexp.MustCompile(`SENSITIVE`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches": [{"parameters":["not a struct"]}]}]}`,
			expected: `{"triggers":[{"rule_matches": [{"parameters":["not a struct"]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values with a bad key_path
		{
			name:     "unexpected-json-value-key-path-missing",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-key-path-bad-type",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":"bad type","highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":"bad type","highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-key-path-null-array",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":null,"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":null,"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-key-path-empty-array",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values in case of bad parameter value
		{
			name:     "unexpected-json-value-parameter-highlight-missing",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-highlight-bad-type",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":"bad type","value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":"bad type","value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-highlight-null-array",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":null,"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":null,"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-highlight-empty-array",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-highlight-empty-array",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[1,"the highlighted SENSITIVE value",[1,2,3]],"value":"the entire SENSITIVE value"}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[1,"the highlighted ? value",[1,2,3]],"value":"the entire ? value"}]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values with a bad parameter value
		{
			name:     "unexpected-json-value-parameter-value-missing",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"]}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"]}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-value-bad-type",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":33}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":33}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-value-null",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":null}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":null}]}]}]}`,
		},
		{
			name:     "unexpected-json-value-parameter-value-empty-string",
			keyRE:    regexp.MustCompile(`k3`),
			valueRE:  regexp.MustCompile(`SENSITIVE`),
			value:    `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE value 1","highlighted SENSITIVE value 2","highlighted SENSITIVE value 3"],"value":""}]}]}]}`,
			expected: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":""}]}]}]}`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				AppSec: AppSecConfig{
					ParameterKeyRegexp:   tc.keyRE,
					ParameterValueRegexp: tc.valueRE,
				},
			}
			result := NewObfuscator(cfg).ObfuscateAppSec(tc.value)
			if tc.value == "" {
				require.Equal(t, result, tc.expected)
			} else {
				// Compare the two parsed json values
				var actual interface{}
				err := json.Unmarshal([]byte(result), &actual)
				require.NoError(t, err)
				var expected interface{}
				err = json.Unmarshal([]byte(tc.expected), &expected)
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			}
		})
	}
}
