package obfuscate

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObfuscateAppSec(t *testing.T) {
	for _, tc := range []struct {
		name                string
		input               string
		expectedOutput      string
		expectedSyntaxError bool
		expectedError       bool
	}{
		{
			name:           "object-empty",
			input:          `{}`,
			expectedOutput: `{}`,
		},
		{
			name:           "object-no-parameters",
			input:          `{ " key 1 " : " value 1 " }`,
			expectedOutput: `{ " key 1 " : " value 1 " }`,
		},
		{
			name:           "object-parameters-last",
			input:          `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" } ] }`,
			expectedOutput: `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a ? value with many ?" } ] }`,
		},
		{
			name:           "object-parameters-alone",
			input:          `{ "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" } ] }`,
			expectedOutput: `{ "parameters" : [ { "value": "i am a ? value with many ?" } ] }`,
		},
		{
			name:           "object-parameters-first",
			input:          `{ "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" } ] , " key 1 " : " value 1 " }`,
			expectedOutput: `{ "parameters" : [ { "value": "i am a ? value with many ?" } ] , " key 1 " : " value 1 " }`,
		},
		{
			name:           "object-parameters-middle",
			input:          `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" } ] , " key 2 " : " value 2 " }`,
			expectedOutput: `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a ? value with many ?" } ] , " key 2 " : " value 2 " }`,
		},
		{
			name:           "object-many-parameters",
			input:          `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, "bad-type", { "value": " i am the second SENSITIVE_VALUE ! " }, 33, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " }`,
			expectedOutput: `{ " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a ? value with many ?" }, "bad-type", { "value": " i am the second ? ! " }, 33, { "value": " i am the third value ! " }, { "value": " i am the forth ? ! " } ] , " key 2 " : " value 2 " }`,
		},
		{
			name:           "object-nested",
			input:          `{ "triggers" : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
			expectedOutput: `{ "triggers" : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a ? value with many ?" }, { "value": " i am the second ? ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth ? ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
		},
		{
			name:                "syntax-error-unexpected-end-of-json",
			input:               `{ "triggers" : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ]`,
			expectedOutput:      `{ "triggers" : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ]`,
			expectedSyntaxError: true,
		},
		{
			name:                "syntax-error-unexpected-string-escape",
			input:               `{ "triggers\ " : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
			expectedOutput:      `{ "triggers\ " : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
			expectedSyntaxError: true,
		},
		{
			name:                "syntax-error",
			input:               `{ "triggers : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
			expectedOutput:      `{ "triggers : [ { "rule_matches" : [ { " key 1 " : " value 1 " , "parameters" : [ { "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }, { "value": " i am the second SENSITIVE_VALUE ! " }, { "value": " i am the third value ! " }, { "value": " i am the forth SENSITIVE_VALUE ! " } ] , " key 2 " : " value 2 " } ] } ] }`,
			expectedSyntaxError: true,
		},

		{
			// The key regexp should take precedence over the value regexp and obfuscate the entire values
			name:           "sensitive-key",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"SENSITIVE_KEY"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"SENSITIVE_KEY"],"highlight":["?","?","?"],"value":"?"}]}]}]}`,
		},
		{
			// The key regexp should take precedence over the value regexp and obfuscate the entire values
			name:           "sensitive-key",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"SENSITIVE_KEY"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"SENSITIVE_KEY"],"highlight":["?","?","?"],"value":"?"}]}]}]}`,
		},
		{
			// The key regexp doesn't match and the value regexp does and obfuscates accordingly.
			name:           "sensitive-value",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,"k1",2,"k3"],"highlight":["highlighted ? value 1","highlighted value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			// The key regexp doesn't match and the value regexp does and obfuscates accordingly.
			name: "sensitive-value",
			input: `
{
  "triggers": [
    {
      "rule_matches": [
        {
          "parameters": [
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            }
          ]
        }
      ]
    }
  ]
}
`,
			expectedOutput: `
{
  "triggers": [
    {
      "rule_matches": [
        {
          "parameters": [
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted ? value 1",
                "highlighted value 2",
                "highlighted ? value 3"
              ],
              "value": "the entire ? value"
            }
          ]
        }
      ]
    }
  ]
}
`,
		},
		{
			name:           "unexpected-json-empty-value",
			input:          ``,
			expectedOutput: ``,
			expectedError:  true,
		},
		{
			name:           "unexpected-json-null-value",
			input:          `null`,
			expectedOutput: `null`,
		},
		{
			name:           "unexpected-json-value",
			input:          `""`,
			expectedOutput: `""`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{}`,
			expectedOutput: `{}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":"not an array"}`,
			expectedOutput: `{"triggers":"not an array"}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":["not a struct"]}`,
			expectedOutput: `{"triggers":["not a struct"]}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":[{"rule_matches": "not an array"}]}`,
			expectedOutput: `{"triggers":[{"rule_matches": "not an array"}]}`,
		}, {

			name:           "unexpected-json-value",
			input:          `{"triggers":[{"rule_matches": ["not a struct"]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches": ["not a struct"]}]}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":[{"rule_matches": [{"parameters":{}}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches": [{"parameters":{}}]}]}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":[{"rule_matches": [{"parameters":"not an array"}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches": [{"parameters":"not an array"}]}]}`,
		},
		{
			name:           "unexpected-json-value",
			input:          `{"triggers":[{"rule_matches": [{"parameters":["not a struct"]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches": [{"parameters":["not a struct"]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values with a bad key_path
		{
			name:           "unexpected-json-value-key-path-missing",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-key-path-bad-type",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":"bad type","highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":"bad type","highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-key-path-null-array",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":null,"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":null,"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-key-path-empty-array",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":"the entire ? value"}]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values in case of bad parameter value
		{
			name:           "unexpected-json-value-parameter-highlight-missing",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-highlight-bad-type",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":"bad type","value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":"bad type","value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-highlight-null-array",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":null,"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":null,"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-highlight-empty-array",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[],"value":"the entire ? value"}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-highlight-empty-array",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[1,"the highlighted SENSITIVE_VALUE value",[1,2,3]],"value":"the entire SENSITIVE_VALUE value"}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":[1,"the highlighted ? value",[1,2,3]],"value":"the entire ? value"}]}]}]}`,
		},
		// The obfuscator should be permissive enough to still obfuscate the values with a bad parameter value
		{
			name:           "unexpected-json-value-parameter-value-missing",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"]}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"]}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-value-bad-type",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":33}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":33}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-value-null",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":null}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":null}]}]}]}`,
		},
		{
			name:           "unexpected-json-value-parameter-value-empty-string",
			input:          `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted SENSITIVE_VALUE value 1","highlighted SENSITIVE_VALUE value 2","highlighted SENSITIVE_VALUE value 3"],"value":""}]}]}]}`,
			expectedOutput: `{"triggers":[{"rule_matches":[{"parameters":[{"key_path":[0,1,2,"3"],"highlight":["highlighted ? value 1","highlighted ? value 2","highlighted ? value 3"],"value":""}]}]}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("enabled", func(t *testing.T) {
				o := appsecEventsObfuscator{
					keyRE:   regexp.MustCompile("SENSITIVE_KEY"),
					valueRE: regexp.MustCompile("SENSITIVE_VALUE"),
				}
				output, err := o.obfuscate(tc.input)
				if err != nil {
					if tc.expectedSyntaxError {
						_, ok := err.(*SyntaxError)
						require.True(t, ok)
					} else if tc.expectedError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}
				require.Equal(t, tc.expectedOutput, output)
			})

			t.Run("disabled", func(t *testing.T) {
				o := appsecEventsObfuscator{
					// Disabled via nil regular expressions
				}
				output, err := o.obfuscate(tc.input)
				if err != nil {
					if tc.expectedSyntaxError {
						_, ok := err.(*SyntaxError)
						require.True(t, ok)
					} else if tc.expectedError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}
				require.Equal(t, tc.input, output)
			})
		})
	}
}

func TestObfuscateRuleMatchParameter(t *testing.T) {
	i := []struct {
		name                     string
		input                    string
		expectedOutput           string
		expectedSyntaxError      bool
		unexpectedScannerOpError int
	}{
		{
			name:           "value-alone",
			input:          `{ "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" }`,
			expectedOutput: `{ "value": "i am a ? value with many ?" }`,
		},
		{
			name:           "highlight-alone",
			input:          `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ] }`,
			expectedOutput: `{ "highlight": [ "i am a ? value", "i am not a a sensitive value", "i am another ? value" ] }`,
		},
		{
			name:           "sensitive-values-without-key-path",
			input:          `{ "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ] }`,
			expectedOutput: `{ "value": "i am a ? value with many ?", "highlight": [ "i am a ? value", "i am not a a sensitive value", "i am another ? value" ] }`,
		},
		{
			name:           "sensitive-values",
			input:          `{ "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key_path": ["key"] }`,
			expectedOutput: `{ "value": "i am a ? value with many ?", "highlight": [ "i am a ? value", "i am not a a sensitive value", "i am another ? value" ], "key_path": ["key"] }`,
		},
		{
			name:           "sensitive-key-last",
			input:          `{ "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"] }`,
			expectedOutput: `{ "value": "?", "highlight": [ "?", "?", "?" ], "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"] }`,
		},
		{
			name:           "sensitive-key-middle",
			input:          `{ "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"], "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ] }`,
			expectedOutput: `{ "value": "?", "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"], "highlight": [ "?", "?", "?" ] }`,
		},
		{
			name:           "sensitive-key-first",
			input:          `{ "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ] }`,
			expectedOutput: `{ "key_path": ["key", 0, 1, 2, "SENSITIVE_KEY"], "value": "?", "highlight": [ "?", "?", "?" ] }`,
		},
		{
			name:           "empty-object",
			input:          `{  }`,
			expectedOutput: `{  }`,
		},
		{
			name:           "empty-object",
			input:          `{}`,
			expectedOutput: `{}`,
		},
		{
			name:           "object-other-properties",
			input:          `{ "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
			expectedOutput: `{ "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
		},
		{
			name:           "object-mixed-properties",
			input:          `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["key"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
			expectedOutput: `{ "highlight": [ "i am a ? value", "i am not a a sensitive value", "i am another ? value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["key"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a ? value with many ?", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
		},
		{
			name:           "object-mixed-properties-sensitive-key",
			input:          `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
			expectedOutput: `{ "highlight": [ "?", "?", "?" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "?", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null }`,
		},
		{
			name:           "object-mixed-no-spaces",
			input:          `{"highlight":["i am a SENSITIVE_VALUE value","i am not a a sensitive value","i am another SENSITIVE_VALUE value"],"key 1":"SENSITIVE_VALUE","key_path":["SENSITIVE_KEY"],"key 2":["SENSITIVE_VALUE"],"value":"i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE","key 3":{"SENSITIVE_KEY":"SENSITIVE_VALUE"},"SENSITIVE_KEY":null}`,
			expectedOutput: `{"highlight":["?","?","?"],"key 1":"SENSITIVE_VALUE","key_path":["SENSITIVE_KEY"],"key 2":["SENSITIVE_VALUE"],"value":"?","key 3":{"SENSITIVE_KEY":"SENSITIVE_VALUE"},"SENSITIVE_KEY":null}`,
		},
		{
			name:           "object-mixed-properties-sensitive-key-with-bad-value-types",
			input:          `{ "highlight": "bad type - i am a SENSITIVE_VALUE value", "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "value": [ "bad type - i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" ], "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": ["SENSITIVE_VALUE"], "key_path": ["SENSITIVE_KEY"] }`,
			expectedOutput: `{ "highlight": "bad type - i am a SENSITIVE_VALUE value", "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "value": [ "bad type - i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE" ], "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": ["SENSITIVE_VALUE"], "key_path": ["SENSITIVE_KEY"] }`,
		},
		{
			name:           "object-mixed-properties-sensitive-key-having-bad-type",
			input:          `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null, "key_path": "bad type - SENSITIVE_KEY" }`,
			expectedOutput: `{ "highlight": [ "i am a ? value", "i am not a a sensitive value", "i am another ? value" ], "key 1": "SENSITIVE_VALUE", "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a ? value with many ?", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null, "key_path": "bad type - SENSITIVE_KEY" }`,
		},
		{
			name:                "unterminated-json",
			input:               `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null`,
			expectedOutput:      `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": null`,
			expectedSyntaxError: true,
		},
		{
			name:                "syntax-error",
			input:               `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": i`,
			expectedOutput:      `{ "highlight": [ "i am a SENSITIVE_VALUE value", "i am not a a sensitive value", "i am another SENSITIVE_VALUE value" ], "key 1": "SENSITIVE_VALUE", "key_path": ["SENSITIVE_KEY"], "key 2": [ "SENSITIVE_VALUE" ], "value": "i am a SENSITIVE_VALUE value with many SENSITIVE_VALUE", "key 3": { "SENSITIVE_KEY": "SENSITIVE_VALUE" }, "SENSITIVE_KEY": i`,
			expectedSyntaxError: true,
		},
	}
	for _, tc := range i {
		t.Run(tc.name, func(t *testing.T) {
			o := appsecEventsObfuscator{
				keyRE:   regexp.MustCompile("SENSITIVE_KEY"),
				valueRE: regexp.MustCompile("SENSITIVE_VALUE"),
			}
			var diff inputDiff
			scanner := &scanner{}
			scanner.reset()
			_, err := o.obfuscateRuleMatchParameter(scanner, tc.input, 0, &diff)
			output := diff.apply(tc.input)
			if err != nil {
				if tc.expectedSyntaxError {
					require.Equal(t, scanner.err, err)
				} else if tc.unexpectedScannerOpError != 0 {
					require.Equal(t, tc.unexpectedScannerOpError, err)
				} else {
					require.NoError(t, err)
				}
				require.Empty(t, diff)
				require.Equal(t, tc.expectedOutput, output)
			} else {
				output := diff.apply(tc.input)
				require.Equal(t, tc.expectedOutput, output)
			}
		})
	}
}

func TestObfuscateRuleMatchParameterValue(t *testing.T) {
	i := []struct {
		name            string
		input           string
		expectedOutput  string
		expectedIgnored bool
	}{
		{
			name:           "one-sensitive-value",
			input:          `"i am a SENSITIVE_VALUE value"`,
			expectedOutput: `"i am a ? value"`,
		},
		{
			name:           "many-sensitive-values",
			input:          `"SENSITIVE_VALUE i am a SENSITIVE_VALUE value SENSITIVE_VALUE"`,
			expectedOutput: `"? i am a ? value ?"`,
		},
		{
			name:           "many-sensitive-values",
			input:          `"      SENSITIVE_VALUE i am a      SENSITIVE_VALUE value      SENSITIVE_VALUE     "`,
			expectedOutput: `"      ? i am a      ? value      ?     "`,
		},
		{
			name:           "no-sensitive-values",
			input:          `"i am just a value"`,
			expectedOutput: `"i am just a value"`,
		},
		{
			name:           "empty-json-string",
			input:          `""`,
			expectedOutput: `""`,
		},
		{
			name:            "unterminated-json-string",
			input:           `"i am a SENSITIVE_VALUE value`,
			expectedIgnored: true,
		},
		{
			name:            "empty-string",
			input:           ``,
			expectedIgnored: true,
		},
		{
			name:            "null",
			input:           `null`,
			expectedIgnored: true,
		},
		{
			name:            "object",
			input:           `{"k":"v"}`,
			expectedIgnored: true,
		},
		{
			name:            "array",
			input:           `[1,2,"three"]`,
			expectedIgnored: true,
		},
		{
			name:            "float",
			input:           `1.5`,
			expectedIgnored: true,
		},
		{
			name:            "syntax-error",
			input:           `"i am a SENSITIVE_VALUE \ `,
			expectedIgnored: true,
		},
	}
	for _, tc := range i {
		t.Run(tc.name, func(t *testing.T) {
			for _, hasSensitiveKey := range []bool{true, false} {
				var name string
				if hasSensitiveKey {
					name = "with-sensitive-key"
				} else {
					name = "without-sensitive-key"
				}
				t.Run(name, func(t *testing.T) {
					o := appsecEventsObfuscator{
						keyRE:   regexp.MustCompile("SENSITIVE_KEY"),
						valueRE: regexp.MustCompile("SENSITIVE_VALUE"),
					}
					var diff inputDiff
					o.obfuscateRuleMatchParameterValue(tc.input, &diff, hasSensitiveKey)
					output := diff.apply(tc.input)
					if tc.expectedIgnored {
						require.Equal(t, tc.input, output)
					} else if hasSensitiveKey {
						require.Equal(t, `"?"`, output)
					} else {
						require.Equal(t, tc.expectedOutput, output)
					}
				})
			}
		})
	}
}

func TestObfuscateRuleMatchParameterHighlights(t *testing.T) {
	i := []struct {
		name                           string
		input                          string
		expectedOutput                 string
		expectedOutputWithSensitiveKey string
	}{
		{
			name:                           "one-sensitive-value",
			input:                          `["i am a SENSITIVE_VALUE value"]`,
			expectedOutput:                 `["i am a ? value"]`,
			expectedOutputWithSensitiveKey: `["?"]`,
		},
		{
			name:                           "many-sensitive-values",
			input:                          `["SENSITIVE_VALUE i am a SENSITIVE_VALUE value SENSITIVE_VALUE"]`,
			expectedOutput:                 `["? i am a ? value ?"]`,
			expectedOutputWithSensitiveKey: `["?"]`,
		},
		{
			name:                           "many-sensitive-values",
			input:                          `["      SENSITIVE_VALUE i am a      SENSITIVE_VALUE value      SENSITIVE_VALUE     "]`,
			expectedOutput:                 `["      ? i am a      ? value      ?     "]`,
			expectedOutputWithSensitiveKey: `["?"]`,
		},
		{
			name:                           "no-sensitive-values",
			input:                          `["i am just a value"]`,
			expectedOutput:                 `["i am just a value"]`,
			expectedOutputWithSensitiveKey: `["?"]`,
		},
		{
			name:                           "empty-array",
			input:                          `[]`,
			expectedOutput:                 `[]`,
			expectedOutputWithSensitiveKey: `[]`,
		},
		{
			name:                           "empty-json-string",
			input:                          `[""]`,
			expectedOutput:                 `[""]`,
			expectedOutputWithSensitiveKey: `["?"]`,
		},
		{
			name:                           "unterminated-json-string",
			input:                          `["i am a SENSITIVE_VALUE value`,
			expectedOutput:                 `["i am a SENSITIVE_VALUE value`,
			expectedOutputWithSensitiveKey: `["i am a SENSITIVE_VALUE value`,
		},
		{
			name:                           "empty-string",
			input:                          ``,
			expectedOutput:                 ``,
			expectedOutputWithSensitiveKey: ``,
		},
		{
			name:                           "null",
			input:                          `null`,
			expectedOutput:                 `null`,
			expectedOutputWithSensitiveKey: `null`,
		},
		{
			name:                           "object",
			input:                          `{}`,
			expectedOutput:                 `{}`,
			expectedOutputWithSensitiveKey: `{}`,
		},
		{
			name:                           "float",
			input:                          `1.5`,
			expectedOutput:                 `1.5`,
			expectedOutputWithSensitiveKey: `1.5`,
		},
		{
			name:                           "syntax-error",
			input:                          `["i am a SENSITIVE_VALUE \ `,
			expectedOutput:                 `["i am a SENSITIVE_VALUE \ `,
			expectedOutputWithSensitiveKey: `["i am a SENSITIVE_VALUE \ `,
		},
	}
	for _, tc := range i {
		t.Run(tc.name, func(t *testing.T) {
			for _, hasSensitiveKey := range []bool{true, false} {
				var name string
				if hasSensitiveKey {
					name = "with-sensitive-key"
				} else {
					name = "without-sensitive-key"
				}
				t.Run(name, func(t *testing.T) {
					o := appsecEventsObfuscator{
						keyRE:   regexp.MustCompile("SENSITIVE_KEY"),
						valueRE: regexp.MustCompile("SENSITIVE_VALUE"),
					}
					var diff inputDiff
					scanner := &scanner{}
					scanner.reset()
					o.obfuscateRuleMatchParameterHighlights(tc.input, &diff, hasSensitiveKey)
					output := diff.apply(tc.input)
					if hasSensitiveKey {
						require.Equal(t, tc.expectedOutputWithSensitiveKey, output)
					} else {
						require.Equal(t, tc.expectedOutput, output)
					}
				})
			}
		})
	}
}

func TestHasSentitiveKeyPath(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		input                string
		expectedSensitiveKey bool
	}{
		{
			name:                 "flat",
			input:                `[]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "flat",
			input:                `[1,2,3,"four","SENSITIVE_KEY",5]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "flat-first",
			input:                `[    "SENSITIVE_KEY"   , 1,2,3,"four" , 5]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "flat-middle",
			input:                `[    "SENSITIVE_KEY"   ]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "flat-last",
			input:                `[ 1,2,3,"four" , 5   ,      "SENSITIVE_KEY"   ]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "flat",
			input:                `[1,2,3,"four",5]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "sub-array",
			input:                `[1,2,3,"four","SENSITIVE_KEY",5, [ "SENSITIVE_KEY" ], 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "sub-array",
			input:                `[1,2,3,"four",5, [ "SENSITIVE_KEY" ], 6]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "sub-array",
			input:                `[1,2,3,"four",5, [[[]]], "SENSITIVE_KEY", 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "sub-object",
			input:                `[1,2,3,"four",5, { "a": "b" }, 6]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "mixed",
			input:                `[1,2,3,"four",5, { "key_path": [ "SENSITIVE_KEY" ] }, 6]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "mixed",
			input:                `[1,2.2,3,"four",5, [ { "key_path": [ "SENSITIVE_KEY" ] } ], [{},[{},[{},[{},[{},[{}]]]]]], "SENSITIVE_KEY", 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "mixed",
			input:                `["SENSITIVE_KEY", 1,2,3,"four",5, [ { "key_path": [ "SENSITIVE_KEY" ] } ], [{},[{},[{},[{},[{},[{}]]]]]], "SENSITIVE_KEY", 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "mixed",
			input:                `[ 1,2,3,"four",5, [ { "key_path": [ "SENSITIVE_KEY" ] } ], [{},[{},[{},[{},[{},[{}]]]]]], "SENSITIVE_KEY", 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "syntax-error",
			input:                `[ 1,2,3,"four",5, [ { "key_path": [ "SENSITIVE_KEY" ] } ], [{},[{},[{},[{},[{},[{}]]]]]], "SENSITIVE_KEY" 6]`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "syntax-error",
			input:                `[ 1,2,3,"four",5, [ { [ "SENSITIVE_KEY" ] } ], [{},[{},[{},[{},[{},[{}]]]]]], "SENSITIVE_KEY", 6]`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "null",
			input:                `null`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "object",
			input:                `{}`,
			expectedSensitiveKey: false,
		},
		{
			name:                 "unterminated",
			input:                `[ "SENSITIVE_KEY"`,
			expectedSensitiveKey: true,
		},
		{
			name:                 "syntax-error",
			input:                `[ "SENSITIVE_KEY"" ]`,
			expectedSensitiveKey: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := appsecEventsObfuscator{
				keyRE:   regexp.MustCompile("SENSITIVE_KEY"),
				valueRE: regexp.MustCompile("SENSITIVE_VALUE"),
			}
			hasSensitiveKey := o.hasSensitiveKeyPath(tc.input)
			require.Equal(t, tc.expectedSensitiveKey, hasSensitiveKey)
		})
	}
}

func TestWalkObject(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		input                    string
		expectedSeen             map[string]string
		expectedSyntaxError      bool
		unexpectedScannerOpError unexpectedScannerOpError
	}{
		{
			name:         "flat",
			input:        `{}`,
			expectedSeen: map[string]string{},
		},
		{
			name:         "flat",
			input:        `{   "key"      :    "  value  "   }`,
			expectedSeen: map[string]string{`"key"`: `    "  value  "   `},
		},
		{
			name:         "flat",
			input:        `{   "key 1"      :    "  value 1  "  ,    "key 2"      :    "  value 2  "  ,  "key 3"      :    "  value 3  "   }`,
			expectedSeen: map[string]string{`"key 1"`: `    "  value 1  "  `, `"key 2"`: `    "  value 2  "  `, `"key 3"`: `    "  value 3  "   `},
		},
		{
			name:         "flat",
			input:        `{"key":"  value  "}`,
			expectedSeen: map[string]string{`"key"`: `"  value  "`},
		},
		{
			name:         "nested-last-array",
			input:        `{"key":["  value  "]}`,
			expectedSeen: map[string]string{`"key"`: `["  value  "]`},
		},
		{
			name:         "nested-last-array",
			input:        `{"key":      [      "  value  "   ]      }`,
			expectedSeen: map[string]string{`"key"`: `      [      "  value  "   ]      `},
		},
		{
			name:         "nested-arrays",
			input:        `{"key 1":      [      "  value 1  "   ]      ,  "key 2":      [      "  value 2  "   ]      ,  "key 3":      [      "  value 3  "   ]       }`,
			expectedSeen: map[string]string{`"key 1"`: `      [      "  value 1  "   ]      `, `"key 2"`: `      [      "  value 2  "   ]      `, `"key 3"`: `      [      "  value 3  "   ]       `},
		},
		{
			name:         "nested-objects",
			input:        `{"key 1" :      {      "nested key 1": "nested  value 1  "   }      ,  "key 2":      {      "nested key 2"  : "nested  value 2  "   }      ,  "key 3":      {      "nested key 3"  : "nested  value 3  "   }       }`,
			expectedSeen: map[string]string{`"key 1"`: `      {      "nested key 1": "nested  value 1  "   }      `, `"key 2"`: `      {      "nested key 2"  : "nested  value 2  "   }      `, `"key 3"`: `      {      "nested key 3"  : "nested  value 3  "   }       `},
		},
		{
			name:         "nested-last-object",
			input:        `{"key":{ "nested key "  : "nested  value   " }}`,
			expectedSeen: map[string]string{`"key"`: `{ "nested key "  : "nested  value   " }`},
		},
		{
			name:         "nested-last-object",
			input:        `{"key":      {      "nested key "  : "nested  value   "   }      }`,
			expectedSeen: map[string]string{`"key"`: `      {      "nested key "  : "nested  value   "   }      `},
		},
		{
			name:                     "null",
			input:                    "null",
			unexpectedScannerOpError: unexpectedScannerOpError(scanBeginLiteral),
		},
		{
			name:                     "array",
			input:                    `[{"k":"v"}]`,
			unexpectedScannerOpError: unexpectedScannerOpError(scanBeginArray),
		},
		{
			name:                     "number",
			input:                    `1`,
			unexpectedScannerOpError: unexpectedScannerOpError(scanBeginLiteral),
		},
		{
			name:                     "float",
			input:                    `1.234`,
			unexpectedScannerOpError: unexpectedScannerOpError(scanBeginLiteral),
		},
		{
			name:                     "string",
			input:                    `"1234"`,
			unexpectedScannerOpError: unexpectedScannerOpError(scanBeginLiteral),
		},
		{
			name:                "unterminated-json",
			input:               `{"k":"v"`,
			expectedSyntaxError: true,
		},
		{
			name:                "unterminated-json",
			input:               `{"k":"v`,
			expectedSyntaxError: true,
		},
		{
			name:                "unterminated-json",
			input:               `{"k":`,
			expectedSyntaxError: true,
		},
		{
			name:                "unterminated-json",
			input:               `{"k"`,
			expectedSyntaxError: true,
		},
		{
			name:                "unterminated-json",
			input:               `{"k`,
			expectedSyntaxError: true,
		},
		{
			name:                "unterminated-json",
			input:               `{`,
			expectedSyntaxError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanner := &scanner{}
			scanner.reset()
			seen := map[string]string{}
			i, err := walkObject(scanner, tc.input, 0, func(keyFrom, keyTo, valueFrom, valueTo int) {
				key := tc.input[keyFrom:keyTo]
				value := tc.input[valueFrom:valueTo]
				assert.NotContains(t, seen, key)
				seen[key] = value
			})
			if tc.expectedSyntaxError {
				require.Equal(t, scanner.err, err)
			} else if tc.unexpectedScannerOpError != 0 {
				require.Equal(t, tc.unexpectedScannerOpError, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, len(tc.input), i)
				require.Equal(t, tc.expectedSeen, seen)
			}
		})
	}
}

func BenchmarkObfuscator(b *testing.B) {
	keyRE := regexp.MustCompile(`SENSITIVE_KEY`)
	valueRE := regexp.MustCompile(`SENSITIVE_VALUE`)
	matching := `
{
  "triggers": [
    {
      "id": "crs-x002-210",
      "name": "Fake AppSec rule",
      "tags": [ "attack-attempt", "crs-x002-210" ],
      "rule_matches": [
        {
          "parameters": [
            {
              "address": "some address",
              "key_path": [
                0,
                1,
                "k1",
                2,
                "SENSITIVE_KEY"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted value 1",
                "highlighted value 2",
                "highlighted value 3"
              ],
              "value": "the entire value"
            }
          ]
        }
      ]
    },
    {
      "id": "crs-x002-210",
      "name": "Fake AppSec rule",
      "tags": [ "attack-attempt", "crs-x002-210" ],
      "rule_matches": [
        {
          "parameters": [
            {
              "address": "some address",
              "key_path": [
                0,
                1,
                "k1",
                2,
                "SENSITIVE_KEY"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted value 1",
                "highlighted value 2",
                "highlighted value 3"
              ],
              "value": "the entire value"
            }
          ]
        }
      ]
    },
    {
      "id": "crs-x002-210",
      "name": "Fake AppSec rule",
      "tags": [
        "attack-attempt",
        "crs-x002-210"
      ],
      "rule_matches": [
        {
          "parameters": [
            {
              "address": "some address",
              "key_path": [
                0,
                1,
                "k1",
                2,
                "SENSITIVE_KEY"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted value 1",
                "highlighted value 2",
                "highlighted value 3"
              ],
              "value": "the entire value"
            }
          ]
        }
      ]
    },
    {
      "id": "crs-x002-210",
      "name": "Fake AppSec rule",
      "tags": [ "attack-attempt", "crs-x002-210" ],
      "rule_matches": [
        {
          "parameters": [
            {
              "address": "some address",
              "key_path": [
                0,
                1,
                "k1",
                2,
                "SENSITIVE_KEY"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted SENSITIVE_VALUE value 1",
                "highlighted SENSITIVE_VALUE value 2",
                "highlighted SENSITIVE_VALUE value 3"
              ],
              "value": "the entire SENSITIVE_VALUE value"
            },
            {
              "key_path": [
                0,
                1,
                "k1",
                2,
                "k3"
              ],
              "highlight": [
                "highlighted value 1",
                "highlighted value 2",
                "highlighted value 3"
              ],
              "value": "the entire value"
            }
          ]
        }
      ]
    }
  ]
}
`
	notMatching := strings.NewReplacer("SENSITIVE_VALUE", "", "SENSITIVE_KEY", "key").Replace(matching)

	for _, bc := range []struct {
		name, input string
	}{
		{
			name:  "matching",
			input: matching,
		},
		{
			name:  "not-matching",
			input: notMatching,
		},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.Run("scanner", func(b *testing.B) {
				o := appsecEventsObfuscator{
					keyRE:   keyRE,
					valueRE: valueRE,
				}
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					_, err := o.obfuscate(bc.input)
					if err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("unmarshalled", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					obfuscatorWithJSONParsing(keyRE, valueRE, bc.input)
				}
			})
		})
	}
}

func obfuscatorWithJSONParsing(keyRE, valueRE *regexp.Regexp, val string) string {
	if keyRE == nil && valueRE == nil {
		return val
	}

	var appsecMeta interface{}
	if err := json.Unmarshal([]byte(val), &appsecMeta); err != nil {
		return val
	}

	meta, ok := appsecMeta.(map[string]interface{})
	if !ok {
		return val
	}

	triggers, ok := meta["triggers"].([]interface{})
	if !ok {
		return val
	}

	var sensitiveDataFound bool
	for _, trigger := range triggers {
		trigger, ok := trigger.(map[string]interface{})
		if !ok {
			continue
		}
		ruleMatches, ok := trigger["rule_matches"].([]interface{})
		if !ok {
			continue
		}
		for _, ruleMatch := range ruleMatches {
			ruleMatch, ok := ruleMatch.(map[string]interface{})
			if !ok {
				continue
			}
			parameters, ok := ruleMatch["parameters"].([]interface{})
			if !ok {
				continue
			}
			for _, param := range parameters {
				param, ok := param.(map[string]interface{})
				if !ok {
					continue
				}

				paramValue, hasStrValue := param["value"].(string)
				highlight, _ := param["highlight"].([]interface{})
				keyPath, _ := param["key_path"].([]interface{})

				var sensitiveKeyFound bool
				for _, key := range keyPath {
					str, ok := key.(string)
					if !ok {
						continue
					}
					if !matchString(keyRE, str) {
						continue
					}
					sensitiveKeyFound = true
					for i, v := range highlight {
						if _, ok := v.(string); ok {
							highlight[i] = "?"
						}
					}
					if hasStrValue {
						param["value"] = "?"
					}
					break
				}

				if sensitiveKeyFound {
					sensitiveDataFound = true
					continue
				}

				// Obfuscate the parameter value
				if hasStrValue && matchString(valueRE, paramValue) {
					sensitiveDataFound = true
					param["value"] = valueRE.ReplaceAllString(paramValue, "?")
				}

				// Obfuscate the parameter highlights
				for i, h := range highlight {
					h, ok := h.(string)
					if !ok {
						continue
					}
					if matchString(valueRE, h) {
						sensitiveDataFound = true
						highlight[i] = valueRE.ReplaceAllString(h, "?")
					}
				}
			}
		}
	}

	if !sensitiveDataFound {
		return val
	}

	newVal, err := json.Marshal(appsecMeta)
	if err != nil {
		return val
	}
	return string(newVal)
}

// matchString is a helper function returning false when the regexp is nil and
// otherwise calling the regular expression to match the string.
func matchString(re *regexp.Regexp, s string) bool {
	if re == nil {
		return false
	}
	return re.MatchString(s)
}
