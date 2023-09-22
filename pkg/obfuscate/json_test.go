// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// obfuscateTestFile contains all the tests for JSON obfuscation
const obfuscateTestFile = "./testdata/json_tests.xml"

type xmlObfuscateTests struct {
	XMLName xml.Name            `xml:"ObfuscateTests"`
	Tests   []*xmlObfuscateTest `xml:"TestSuite>Test"`
}

type xmlObfuscateTest struct {
	Tag                string
	DontNormalize      bool // this test contains invalid JSON
	In                 string
	Out                string
	KeepValues         []string `xml:"KeepValues>key"`
	ObfuscateSQLValues []string `xml:"ObfuscateSQLValues>key"`
}

// loadTests loads all XML tests from ./testdata/obfuscate.xml
func loadTests() ([]*xmlObfuscateTest, error) {
	path, err := filepath.Abs(obfuscateTestFile)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var suite xmlObfuscateTests
	if err := xml.NewDecoder(f).Decode(&suite); err != nil {
		return nil, err
	}
	for i, test := range suite.Tests {
		// normalize JSON output
		if !test.DontNormalize {
			test.Out, err = normalize(test.Out)
			if err != nil {
				return nil, fmt.Errorf("failed to normalize test.Out. test_case_number=%d tag=%s error='%s'", i, test.Tag, err.Error())
			}
			test.In, err = normalize(test.In)
			if err != nil {
				return nil, fmt.Errorf("failed to normalize test.In. test_case_number=%d tag=%s error='%s'", i, test.Tag, err.Error())
			}
		}
	}
	return suite.Tests, err
}

// normalize normalizes JSON input. This allows us to write "pretty" JSON
// inside the test file using \t, \r, \n, etc.
func normalize(in string) (string, error) {
	var tmp map[string]interface{}
	if err := json.Unmarshal([]byte(in), &tmp); err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(tmp)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// jsonSuite holds the JSON test suite. It is loaded in TestMain.
var jsonSuite []*xmlObfuscateTest

func assertEqualJSON(t *testing.T, expected string, actual string) {
	var expectedParsed map[string]interface{}
	var actualParsed map[string]interface{}
	err := json.Unmarshal([]byte(expected), &expectedParsed)
	assert.NoError(t, err)
	err = json.Unmarshal([]byte(actual), &actualParsed)
	assert.NoError(t, err)
	assert.Equal(t, expectedParsed, actualParsed)
}

func TestObfuscateJSON(t *testing.T) {
	runTest := func(s *xmlObfuscateTest) func(*testing.T) {
		return func(t *testing.T) {
			assert := assert.New(t)
			cfg := &JSONConfig{
				KeepValues:         s.KeepValues,
				ObfuscateSQLValues: s.ObfuscateSQLValues,
			}
			out, err := newJSONObfuscator(cfg, NewObfuscator(Config{})).obfuscate([]byte(s.In))
			if !s.DontNormalize {
				assert.NoError(err)
				assertEqualJSON(t, s.Out, out)
			}
		}
	}
	for i, s := range jsonSuite {
		var name string
		if s.DontNormalize {
			name += "invalid/"
		}
		name += strconv.Itoa(i + 1)
		t.Run(fmt.Sprintf("%s/%s/", name, s.Tag), runTest(s))
	}
}

func BenchmarkObfuscateJSON(b *testing.B) {
	cfg := &JSONConfig{KeepValues: []string{"highlight"}}
	if len(jsonSuite) == 0 {
		b.Fatal("no test suite loaded")
	}
	for i := len(jsonSuite) - 1; i >= 0; i-- {
		test := jsonSuite[i]
		obf := newJSONObfuscator(cfg, NewObfuscator(Config{}))
		b.Run(test.Tag, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := obf.obfuscate([]byte(test.In))
				if !test.DontNormalize && err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func FuzzObfuscateJSON(f *testing.F) {
	for _, s := range jsonSuite {
		f.Add([]byte(s.In))
	}
	o := newJSONObfuscator(&JSONConfig{}, NewObfuscator(Config{}))
	f.Fuzz(func(t *testing.T, b []byte) {
		o.obfuscate(b)
	})
}
