// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"encoding/json"
	"encoding/xml"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

// obfuscateTestFile contains all the tests for JSON obfuscation
const obfuscateTestFile = "./testdata/json_tests.xml"

type xmlObfuscateTests struct {
	XMLName xml.Name            `xml:"ObfuscateTests,-"`
	Tests   []*xmlObfuscateTest `xml:"TestSuite>Test"`
}

type xmlObfuscateTest struct {
	Tag           string
	DontNormalize bool // this test contains invalid JSON
	In            string
	Out           string
	KeepValues    []string `xml:"KeepValues>key"`
}

// loadTests loads all XML tests from ./testdata/obfuscate.xml
func loadTests() ([]*xmlObfuscateTest, error) {
	path, err := filepath.Abs(obfuscateTestFile)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	var suite xmlObfuscateTests
	if err := xml.NewDecoder(f).Decode(&suite); err != nil {
		return nil, err
	}
	for _, test := range suite.Tests {
		// normalize JSON output
		if !test.DontNormalize {
			test.Out = normalize(test.Out)
			test.In = normalize(test.In)
		}
	}
	return suite.Tests, err
}

// normalize normalizes JSON input. This allows us to write "pretty" JSON
// inside the test file using \t, \r, \n, etc.
func normalize(in string) string {
	var tmp map[string]interface{}
	if err := json.Unmarshal([]byte(in), &tmp); err != nil {
		log.Fatal(err)
	}
	out, err := json.Marshal(tmp)
	if err != nil {
		log.Fatal(err)
	}
	return string(out)
}

// jsonSuite holds the JSON test suite. It is loaded in TestMain.
var jsonSuite []*xmlObfuscateTest

func TestObfuscateJSON(t *testing.T) {
	runTest := func(s *xmlObfuscateTest) func(*testing.T) {
		return func(t *testing.T) {
			assert := assert.New(t)
			cfg := &config.JSONObfuscationConfig{KeepValues: s.KeepValues}
			out, err := newJSONObfuscator(cfg).obfuscate([]byte(s.In))
			if !s.DontNormalize {
				assert.NoError(err)
			}
			assert.Equal(s.Out, out)
		}
	}
	for i, s := range jsonSuite {
		var name string
		if s.DontNormalize {
			name += "invalid/"
		}
		name += strconv.Itoa(i + 1)
		t.Run(name, runTest(s))
	}
}

func BenchmarkObfuscateJSON(b *testing.B) {
	cfg := &config.JSONObfuscationConfig{KeepValues: []string{"highlight"}}
	if len(jsonSuite) == 0 {
		b.Fatal("no test suite loaded")
	}
	var ran int
	for i := len(jsonSuite) - 1; i >= 0; i-- {
		ran++
		if ran > 3 {
			// run max 3 benchmarks
			break
		}
		test := jsonSuite[i]
		b.Run(strconv.Itoa(len(test.In)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := newJSONObfuscator(cfg).obfuscate([]byte(test.In))
				if !test.DontNormalize && err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
