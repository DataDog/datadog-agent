// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package zookeeper

import (
	"context"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/samuel/go-zookeeper/zk"
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

type ZkTestSuite struct {
	suite.Suite
	templates     map[string]string
	client        *zk.Conn
	containerName string
	zkVersion     string
	zkURL         string
}

// use a constructor to make the suite parametric
func NewZkTestSuite(zkVersion, containerName string) *ZkTestSuite {
	return &ZkTestSuite{
		containerName: containerName,
		zkVersion:     zkVersion,
	}
}

func (suite *ZkTestSuite) SetupSuite() {
	var err error

	// pull the image, create a standalone zk container
	imageName := "zookeeper:" + suite.zkVersion
	containerID, err := utils.StartZkContainer(imageName, suite.containerName)
	if err != nil {
		// failing in SetupSuite won't call TearDownSuite, do it manually
		suite.TearDownSuite()
		suite.FailNow(err.Error())
	}

	suite.zkURL, err = utils.GetContainerIP(containerID)
	if err != nil {
		suite.TearDownSuite()
		suite.FailNow(err.Error())
	}

	suite.client, _, err = zk.Connect([]string{suite.zkURL}, 2*time.Second)
	if err != nil {
		suite.TearDownSuite()
		suite.FailNow(err.Error())
	}
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
	config.Datadog.Set("autoconf_template_url", suite.zkURL)
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

	require.Nil(suite.T(), err)
	require.Len(suite.T(), templates, 3)

	// FIXME: require.Equal(suite.T(), "/datadog/check_configs/nginx", templates[0].Digest())
	require.Equal(suite.T(), "nginx_a", templates[0].Name)
	require.Equal(suite.T(), "{}", string(templates[0].InitConfig))
	require.Len(suite.T(), templates[0].Instances, 1)
	require.Equal(suite.T(), "{\"key\":2}", string(templates[0].Instances[0]))

	// FIXME: require.Equal(suite.T(), check.ID("/datadog/check_configs/nginx"), templates[1].ID)
	require.Equal(suite.T(), "nginx_b", templates[1].Name)
	require.Equal(suite.T(), "{\"key\":3}", string(templates[1].InitConfig))
	require.Len(suite.T(), templates[1].Instances, 1)
	require.Equal(suite.T(), "{}", string(templates[1].Instances[0]))

	// FIXME: require.Equal(suite.T(), check.ID("/datadog/check_configs/redis"), templates[2].ID)
	require.Equal(suite.T(), "redis_a", templates[2].Name)
	require.Equal(suite.T(), "{}", string(templates[2].InitConfig))
	require.Len(suite.T(), templates[2].Instances, 1)
	require.Equal(suite.T(), "{}", string(templates[2].Instances[0]))
}

func TestZkSuite(t *testing.T) {
	suite.Run(t, NewZkTestSuite("3.3.6", "datadog-agent-test-zk"))
	suite.Run(t, NewZkTestSuite("3.4.10", "datadog-agent-test-zk"))
}
