package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
