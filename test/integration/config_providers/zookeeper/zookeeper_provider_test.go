package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	zkDataTree = [][]string{
		// create required path (we need to create every node one by one)
		[]string{"/datadog", ""},
		[]string{"/datadog/check_configs", ""},

		//// create 3 valid configuration
		[]string{"/datadog/check_configs/nginx", ""},
		[]string{"/datadog/check_configs/nginx/check_names", "[\"nginx_a\", \"nginx_b\"]"},
		[]string{"/datadog/check_configs/nginx/instances", "[{\"key\":2}, {}]"},
		[]string{"/datadog/check_configs/nginx/init_configs", "[{}, {\"key\":3}]"},

		[]string{"/datadog/check_configs/redis", ""},
		[]string{"/datadog/check_configs/redis/check_names", "[\"redis_a\"]"},
		[]string{"/datadog/check_configs/redis/instances", "[{}]"},
		[]string{"/datadog/check_configs/redis/init_configs", "[{}]"},

		//// create non config folder folder
		[]string{"/datadog/check_configs/other", ""},
		[]string{"/datadog/check_configs/other/data", "some data"},

		//// create config with missing parameter
		[]string{"/datadog/check_configs/incomplete", ""},
		[]string{"/datadog/check_configs/incomplete/instances", "[{\"key\":2}, {}]"},
		[]string{"/datadog/check_configs/incomplete/init_configs", "[{}, {\"key\":3}]"},

		//// create config with json error
		[]string{"/datadog/check_configs/json_error1", ""},
		[]string{"/datadog/check_configs/json_error1/check_names", "[\"nginx_a\", \"nginx_b\"]"},
		[]string{"/datadog/check_configs/json_error1/instances", "[{\"key\":2}]"},
		[]string{"/datadog/check_configs/json_error1/init_configs", "[{}, {\"key\":3}]"},

		[]string{"/datadog/check_configs/json_error2", ""},
		[]string{"/datadog/check_configs/json_error2/check_names", "[\"nginx_a\"]"},
		[]string{"/datadog/check_configs/json_error2/instances", "[{]"},
		[]string{"/datadog/check_configs/json_error2/init_configs", "[{}]"},

		[]string{"/datadog/check_configs/json_error3", ""},
		[]string{"/datadog/check_configs/json_error3/check_names", "\"nginx_a\""},
		[]string{"/datadog/check_configs/json_error3/instances", "[{}]"},
		[]string{"/datadog/check_configs/json_error3/init_configs", "[{}]"},
	}

	zookeeperURL = os.Getenv("ZK_URL")
)

func zkSetup() error {
	c, _, err := zk.Connect([]string{zookeeperURL}, 2*time.Second)
	if err != nil {
		return fmt.Errorf("ZookeeperConfigProvider: couldn't connect zookeeper on port 2181: %s", err)
	}
	defer c.Close()

	// create test data
	for _, node := range zkDataTree {
		_, err := c.Create(node[0], []byte(node[1]), 0, zk.WorldACL(zk.PermAll))
		log.Errorf("creating node: %s", node[0])
		if err != nil && err != zk.ErrNodeExists {
			log.Errorf("Could not create path %s with value '%s': %s", node[0], node[1], err)
			return err
		}
	}

	return nil
}

func TestCollect(t *testing.T) {
	err := zkSetup()
	require.Nil(t, err)

	config.Datadog.Set("autoconf_template_url", zookeeperURL)
	zk, err := providers.NewZookeeperConfigProvider()
	require.Nil(t, err)

	templates, err := zk.Collect()

	assert.Nil(t, err)
	assert.Len(t, templates, 3)

	assert.Equal(t, check.ID("/datadog/check_configs/nginx"), templates[0].ID)
	assert.Equal(t, "nginx_a", templates[0].Name)
	assert.Equal(t, "{}", string(templates[0].InitConfig))
	require.Len(t, templates[0].Instances, 1)
	assert.Equal(t, "{\"key\":2}", string(templates[0].Instances[0]))

	assert.Equal(t, check.ID("/datadog/check_configs/nginx"), templates[1].ID)
	assert.Equal(t, "nginx_b", templates[1].Name)
	assert.Equal(t, "{\"key\":3}", string(templates[1].InitConfig))
	require.Len(t, templates[1].Instances, 1)
	assert.Equal(t, "{}", string(templates[1].Instances[0]))

	assert.Equal(t, check.ID("/datadog/check_configs/redis"), templates[2].ID)
	assert.Equal(t, "redis_a", templates[2].Name)
	assert.Equal(t, "{}", string(templates[2].InitConfig))
	require.Len(t, templates[2].Instances, 1)
	assert.Equal(t, "{}", string(templates[2].Instances[0]))
}
