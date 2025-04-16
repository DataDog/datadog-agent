package expansion

import (
	"testing"

	api "k8s.io/api/core/v1"
)

func TestMappingFuncFor(t *testing.T) {
	context := map[string]string{
		"VAR_A": "A",
	}
	mapping := MappingFuncFor(context)

	cases := []struct {
		input             string
		expectedExpansion string
		expectedStatus    bool
	}{
		{
			input:             "VAR_A",
			expectedExpansion: "A",
			expectedStatus:    true,
		},
		{
			input:             "VAR_DNE",
			expectedExpansion: "$(VAR_DNE)",
			expectedStatus:    false,
		},
	}

	for _, tc := range cases {
		expanded, success := mapping(tc.input)
		if e, a := tc.expectedExpansion, expanded; e != a {
			t.Errorf("expected expansion=%q, got %q", e, a)
		}
		if e, a := tc.expectedStatus, success; e != a {
			t.Errorf("expected status=%v, got %v", e, a)
		}
	}
}

func TestMapReference(t *testing.T) {
	envs := []api.EnvVar{
		{
			Name:  "FOO",
			Value: "bar",
		},
		{
			Name:  "ZOO",
			Value: "$(FOO)-1",
		},
		{
			Name:  "BLU",
			Value: "$(ZOO)-2",
		},
		{
			Name:  "INCOMPLETE",
			Value: "$(ZOO)-2-$(DNE)",
		},
	}

	declaredEnv := map[string]string{
		"FOO":  "bar",
		"ZOO":  "$(FOO)-1",
		"BLU":  "$(ZOO)-2",
		"FAIL": "$(ZOO)-2-$(DNE)",
	}
	declaredStatus := map[string]bool{}

	serviceEnv := map[string]string{}

	mapping := MappingFuncFor(declaredEnv, serviceEnv)

	for _, env := range envs {
		declaredEnv[env.Name], declaredStatus[env.Name] = Expand(env.Value, mapping)
	}

	expectedEnv := map[string]string{
		"FOO":  "bar",
		"ZOO":  "bar-1",
		"BLU":  "bar-1-2",
		"FAIL": "bar-1-2-$(DNE)",
	}
	expectedStatus := map[string]bool{
		"FOO":  true,
		"ZOO":  true,
		"BLU":  true,
		"FAIL": false,
	}

	for k, v := range expectedEnv {
		if e, a := v, declaredEnv[k]; e != a {
			t.Errorf("Expected expansion %v, got %v", e, a)
		} else {
			delete(declaredEnv, k)
		}
	}

	for k, v := range expectedStatus {
		if e, a := v, declaredStatus[k]; e != a {
			t.Errorf("Expected status %v, got %v", e, a)
		} else {
			delete(declaredStatus, k)
		}
	}

	if len(declaredEnv) != 0 {
		t.Errorf("Unexpected keys in declared env: %v", declaredEnv)
	}
}

func TestMapping(t *testing.T) {
	context := map[string]string{
		"VAR_A":     "A",
		"VAR_B":     "B",
		"VAR_C":     "C",
		"VAR_REF":   "$(VAR_A)",
		"VAR_EMPTY": "",
	}
	mapping := MappingFuncFor(context)

	doExpansionTest(t, mapping)
}

func TestMappingDual(t *testing.T) {
	context := map[string]string{
		"VAR_A":     "A",
		"VAR_EMPTY": "",
	}
	context2 := map[string]string{
		"VAR_B":   "B",
		"VAR_C":   "C",
		"VAR_REF": "$(VAR_A)",
	}
	mapping := MappingFuncFor(context, context2)

	doExpansionTest(t, mapping)
}

func doExpansionTest(t *testing.T, mapping func(string) (string, bool)) {
	cases := []struct {
		name           string
		input          string
		expected       string
		expectedStatus bool
	}{
		{
			name:           "whole string",
			input:          "$(VAR_A)",
			expected:       "A",
			expectedStatus: true,
		},
		{
			name:           "repeat",
			input:          "$(VAR_A)-$(VAR_A)",
			expected:       "A-A",
			expectedStatus: true,
		},
		{
			name:           "beginning",
			input:          "$(VAR_A)-1",
			expected:       "A-1",
			expectedStatus: true,
		},
		{
			name:           "middle",
			input:          "___$(VAR_B)___",
			expected:       "___B___",
			expectedStatus: true,
		},
		{
			name:           "end",
			input:          "___$(VAR_C)",
			expected:       "___C",
			expectedStatus: true,
		},
		{
			name:           "compound",
			input:          "$(VAR_A)_$(VAR_B)_$(VAR_C)",
			expected:       "A_B_C",
			expectedStatus: true,
		},
		{
			name:           "escape & expand",
			input:          "$$(VAR_B)_$(VAR_A)",
			expected:       "$(VAR_B)_A",
			expectedStatus: true,
		},
		{
			name:           "compound escape",
			input:          "$$(VAR_A)_$$(VAR_B)",
			expected:       "$(VAR_A)_$(VAR_B)",
			expectedStatus: true,
		},
		{
			name:           "mixed in escapes",
			input:          "f000-$$VAR_A",
			expected:       "f000-$VAR_A",
			expectedStatus: true,
		},
		{
			name:           "backslash escape ignored",
			input:          "foo\\$(VAR_C)bar",
			expected:       "foo\\Cbar",
			expectedStatus: true,
		},
		{
			name:           "backslash escape ignored",
			input:          "foo\\\\$(VAR_C)bar",
			expected:       "foo\\\\Cbar",
			expectedStatus: true,
		},
		{
			name:           "lots of backslashes",
			input:          "foo\\\\\\\\$(VAR_A)bar",
			expected:       "foo\\\\\\\\Abar",
			expectedStatus: true,
		},
		{
			name:           "nested var references",
			input:          "$(VAR_A$(VAR_B))",
			expected:       "$(VAR_A$(VAR_B))",
			expectedStatus: false,
		},
		{
			name:           "nested var references second type",
			input:          "$(VAR_A$(VAR_B)",
			expected:       "$(VAR_A$(VAR_B)",
			expectedStatus: false,
		},
		{
			name:           "value is a reference",
			input:          "$(VAR_REF)",
			expected:       "$(VAR_A)",
			expectedStatus: true,
		},
		{
			name:           "value is a reference x 2",
			input:          "%%$(VAR_REF)--$(VAR_REF)%%",
			expected:       "%%$(VAR_A)--$(VAR_A)%%",
			expectedStatus: true,
		},
		{
			name:           "empty var",
			input:          "foo$(VAR_EMPTY)bar",
			expected:       "foobar",
			expectedStatus: true,
		},
		{
			name:           "unterminated expression",
			input:          "foo$(VAR_Awhoops!",
			expected:       "foo$(VAR_Awhoops!",
			expectedStatus: true,
		},
		{
			name:           "expression without operator",
			input:          "f00__(VAR_A)__",
			expected:       "f00__(VAR_A)__",
			expectedStatus: true,
		},
		{
			name:           "shell special vars pass through",
			input:          "$?_boo_$!",
			expected:       "$?_boo_$!",
			expectedStatus: true,
		},
		{
			name:           "bare operators are ignored",
			input:          "$VAR_A",
			expected:       "$VAR_A",
			expectedStatus: true,
		},
		{
			name:           "undefined vars are passed through",
			input:          "$(VAR_DNE)",
			expected:       "$(VAR_DNE)",
			expectedStatus: false,
		},
		{
			name:           "undefined vars are passed through",
			input:          "$(VAR_A)-$(VAR_DNE)",
			expected:       "A-$(VAR_DNE)",
			expectedStatus: false,
		},
		{
			name:           "multiple (even) operators, var undefined",
			input:          "$$$$$$(BIG_MONEY)",
			expected:       "$$$(BIG_MONEY)",
			expectedStatus: true,
		},
		{
			name:           "multiple (even) operators, var defined",
			input:          "$$$$$$(VAR_A)",
			expected:       "$$$(VAR_A)",
			expectedStatus: true,
		},
		{
			name:           "multiple (odd) operators, var undefined",
			input:          "$$$$$$$(GOOD_ODDS)",
			expected:       "$$$$(GOOD_ODDS)",
			expectedStatus: false,
		},
		{
			name:           "multiple (odd) operators, var defined",
			input:          "$$$$$$$(VAR_A)",
			expected:       "$$$A",
			expectedStatus: true,
		},
		{
			name:           "missing open expression",
			input:          "$VAR_A)",
			expected:       "$VAR_A)",
			expectedStatus: true,
		},
		{
			name:           "shell syntax ignored",
			input:          "${VAR_A}",
			expected:       "${VAR_A}",
			expectedStatus: true,
		},
		{
			name:           "trailing incomplete expression not consumed",
			input:          "$(VAR_B)_______$(A",
			expected:       "B_______$(A",
			expectedStatus: true,
		},
		{
			name:           "trailing incomplete expression, no content, is not consumed",
			input:          "$(VAR_C)_______$(",
			expected:       "C_______$(",
			expectedStatus: true,
		},
		{
			name:           "operator at end of input string is preserved",
			input:          "$(VAR_A)foobarzab$",
			expected:       "Afoobarzab$",
			expectedStatus: true,
		},
		{
			name:           "shell escaped incomplete expr",
			input:          "foo-\\$(VAR_A",
			expected:       "foo-\\$(VAR_A",
			expectedStatus: true,
		},
		{
			name:           "lots of $( in middle",
			input:          "--$($($($($--",
			expected:       "--$($($($($--",
			expectedStatus: true,
		},
		{
			name:           "lots of $( in beginning",
			input:          "$($($($($--foo$(",
			expected:       "$($($($($--foo$(",
			expectedStatus: true,
		},
		{
			name:           "lots of $( at end",
			input:          "foo0--$($($($(",
			expected:       "foo0--$($($($(",
			expectedStatus: true,
		},
		{
			name:           "escaped operators in variable names are not escaped",
			input:          "$(foo$$var)",
			expected:       "$(foo$$var)",
			expectedStatus: false,
		},
		{
			name:           "newline not expanded",
			input:          "\n",
			expected:       "\n",
			expectedStatus: true,
		},
	}

	for _, tc := range cases {
		expanded, success := Expand(tc.input, mapping)
		if e, a := tc.expected, expanded; e != a {
			t.Errorf("%v: expected %q, got %q", tc.name, e, a)
		}
		if e, a := tc.expectedStatus, success; e != a {
			t.Errorf("%v: expected success=%v, got %v", tc.name, e, a)
		}
	}
}
