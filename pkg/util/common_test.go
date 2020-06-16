package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"gopkg.in/yaml.v2"
)

func TestJSONConverter(t *testing.T) {

	checks := []string{
		"cassandra",
		"kafka",
		"jmx",
		"jmx_alt",
	}

	cache := map[string]integration.RawMap{}
	for _, c := range checks {
		var cf integration.RawMap

		// Read file contents
		yamlFile, err := ioutil.ReadFile(fmt.Sprintf("../collector/corechecks/embed/jmx/fixtures/%s.yaml", c))
		assert.Nil(t, err)

		// Parse configuration
		err = yaml.Unmarshal(yamlFile, &cf)
		assert.Nil(t, err)

		cache[c] = cf
	}

	//convert
	j := map[string]interface{}{}
	c := map[string]interface{}{}
	for name, config := range cache {
		c[name] = GetJSONSerializableMap(config)
	}
	j["configurations"] = c

	//json encode
	_, err := json.Marshal(GetJSONSerializableMap(j))
	assert.Nil(t, err)
}

func TestCopyDir(t *testing.T) {
	assert := assert.New(t)
	src, err := ioutil.TempDir("", "copydir-test-src-")
	assert.NoError(err)
	defer os.RemoveAll(src)

	dst, err := ioutil.TempDir("", "copydir-test-dst-")
	assert.NoError(err)
	defer os.RemoveAll(dst)

	files := map[string]string{
		"a/b/c/d.txt": "d.txt",
		"e/f/g/h.txt": "h.txt",
		"i/j/k.txt":   "k.txt",
	}

	for file, content := range files {
		p := filepath.Join(src, file)
		err = os.MkdirAll(filepath.Dir(p), os.ModePerm)
		assert.NoError(err)
		err = ioutil.WriteFile(p, []byte(content), os.ModePerm)
		assert.NoError(err)
	}
	err = CopyDir(src, dst)
	assert.NoError(err)

	for file, content := range files {
		p := filepath.Join(dst, file)
		actual, err := ioutil.ReadFile(p)
		assert.NoError(err)
		assert.Equal(string(actual), content)
	}
}
