// +build jmx

package embed

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJmxCfg(t *testing.T) {
	jmxConf := new(jmxCfg)

	// Test instance with no listed JMX conf files
	instance := []byte("files:")
	err := jmxConf.Parse(instance)
	assert.EqualError(t, err, "Error parsing configuration: no config files")

	// Test valid instance
	instance = []byte("files:\n  - kafka.yml")
	err = jmxConf.Parse(instance)
	assert.Nil(t, err)
	if assert.Equal(t, 1, len(jmxConf.instance.Files)) {
		assert.Equal(t, "kafka.yml", jmxConf.instance.Files[0])
	}
}

func TestReadJMXConf(t *testing.T) {
	checkConf := new(checkCfg)

	// Test for no instances in jmx check conf file
	checkConfYaml := []byte("" +
		"init_config:\n" +
		"instances:\n")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err := readJMXConf(checkConf, "")
	assert.EqualError(t, err, "You need to have at least one instance "+
		"defined in the YAML file for this check")

	// Test for no name with jmx_url
	checkConfYaml = []byte("" +
		"init_config:\n" +
		"instances:\n" +
		" - jmx_url: foo\n")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.EqualError(t, err, "A name must be specified when using a jmx_url")

	// Test for no host
	checkConfYaml = []byte("" +
		"init_config:\n" +
		"instances:\n" +
		" - host:")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.EqualError(t, err, "A host must be specified")

	// Test for no port
	checkConfYaml = []byte("" +
		"init_config:\n" +
		"instances:\n" +
		" - host: foo\n")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.EqualError(t, err, "A numeric port must be specified")

	// Test for no include in conf
	checkConfYaml = []byte("" +
		"init_config:\n" +
		" conf:\n" +
		"  - include:\n" +
		"instances:\n" +
		" - host: foo\n" +
		"   port: 1234\n")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.EqualError(t, err, fmt.Sprintf("Each configuration must have an"+
		" 'include' section. %s", linkToDoc))

	// Test for no tools.jar if isAttachAPI
	checkConfYaml = []byte("" +
		"init_config:\n" +
		"instances:\n" +
		" - host: foo\n" +
		"   port: 1234\n" +
		"   process_name_regex: regex")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.EqualError(t, err, fmt.Sprintf("You must specify the path to tools.jar. %s", linkToDoc))

	// Test valid conf
	checkConfYaml = []byte("" +
		"init_config:\n" +
		"instances:\n" +
		" - host: foo\n" +
		"   port: 1234\n")
	checkConf.Parse(checkConfYaml)
	_, _, _, _, err = readJMXConf(checkConf, "")
	assert.Nil(t, err)
}
