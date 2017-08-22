// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package configproviders

import (
	"context"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/test/integration/utils"
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
)

const (
	zookeeperURL = "127.0.0.1"
)

type ZkTestSuite struct {
	suite.Suite
	templates     map[string]string
	client        *zk.Conn
	containerName string
}

func (suite *ZkTestSuite) SetupSuite() {
	var err error
	suite.client, _, err = zk.Connect([]string{zookeeperURL}, 2*time.Second)
	if err != nil {
		panic(err)
	}

	// pull the latest image, create a standalone zk container
	suite.containerName = "datadog-agent-test-zk"
	utils.StartZkContainer("zookeeper:latest", suite.containerName)

	// wait for zk to start
	time.Sleep(1 * time.Second)
}

func (suite *ZkTestSuite) TearDownSuite() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	cli.ContainerRemove(ctx, suite.containerName, types.ContainerRemoveOptions{Force: true})
}

// put configuration back in a known state before each test
func (suite *ZkTestSuite) SetupTest() {
	config.Datadog.Set("autoconf_template_url", zookeeperURL)
	config.Datadog.Set("autoconf_template_dir", "/datadog/check_configs")
	config.Datadog.Set("autoconf_template_username", "")
	config.Datadog.Set("autoconf_template_password", "")

	suite.populate()
}

func (suite *ZkTestSuite) populate() error {
	// create test data
	for _, node := range zkDataTree {
		_, err := suite.client.Create(node[0], []byte(node[1]), 0, zk.WorldACL(zk.PermAll))
		if err != nil && err != zk.ErrNodeExists {
			log.Errorf("Could not create path %s with value '%s': %s", node[0], node[1], err)
			return err
		}
	}

	return nil
}

func (suite *ZkTestSuite) TestCollect() {
	zk, err := providers.NewZookeeperConfigProvider()
	require.Nil(suite.T(), err)

	templates, err := zk.Collect()

	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), templates, 3)

	// FIXME: assert.Equal(suite.T(), "/datadog/check_configs/nginx", templates[0].Digest())
	assert.Equal(suite.T(), "nginx_a", templates[0].Name)
	assert.Equal(suite.T(), "{}", string(templates[0].InitConfig))
	require.Len(suite.T(), templates[0].Instances, 1)
	assert.Equal(suite.T(), "{\"key\":2}", string(templates[0].Instances[0]))

	// FIXME: assert.Equal(suite.T(), check.ID("/datadog/check_configs/nginx"), templates[1].ID)
	assert.Equal(suite.T(), "nginx_b", templates[1].Name)
	assert.Equal(suite.T(), "{\"key\":3}", string(templates[1].InitConfig))
	require.Len(suite.T(), templates[1].Instances, 1)
	assert.Equal(suite.T(), "{}", string(templates[1].Instances[0]))

	// FIXME: assert.Equal(suite.T(), check.ID("/datadog/check_configs/redis"), templates[2].ID)
	assert.Equal(suite.T(), "redis_a", templates[2].Name)
	assert.Equal(suite.T(), "{}", string(templates[2].InitConfig))
	require.Len(suite.T(), templates[2].Instances, 1)
	assert.Equal(suite.T(), "{}", string(templates[2].Instances[0]))
}

func TestZkSuite(t *testing.T) {
	suite.Run(t, new(ZkTestSuite))
}
