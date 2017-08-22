// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package configproviders

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/test/integration/utils"

	etcd_client "github.com/coreos/etcd/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	etcdImg  string = "quay.io/coreos/etcd:latest"
	etcdURL  string = "http://127.0.0.1:2379/"
	etcdUser string = "root"
	etcdPass string = "root"
)

type EtcdTestSuite struct {
	suite.Suite
	templates     map[string]string
	clientCfg     etcd_client.Config
	containerName string
}

func (suite *EtcdTestSuite) SetupSuite() {
	suite.templates = map[string]string{
		"/foo/nginx/check_names":  `["http_check", "nginx"]`,
		"/foo/nginx/init_configs": `[{}, {}]`,
		"/foo/nginx/instances":    `[{"name": "test", "url": "http://%25%25host%25%25/", "timeout": 5}, {"foo": "bar"}]`,
	}
	suite.clientCfg = etcd_client.Config{
		Endpoints:               []string{etcdURL},
		Transport:               etcd_client.DefaultTransport,
		HeaderTimeoutPerRequest: 1 * time.Second,
		Username:                etcdUser,
		Password:                etcdPass,
	}

	// pull the latest etcd image, create a standalone etcd container
	suite.containerName = "datadog-agent-etcd0"
	utils.StartEtcdContainer(etcdImg, suite.containerName)

	// wait for etcd to start
	time.Sleep(1 * time.Second)

	suite.setEtcdPassword()
}

func (suite *EtcdTestSuite) TearDownSuite() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	cli.ContainerRemove(ctx, suite.containerName, types.ContainerRemoveOptions{Force: true})
}

// put configuration back in a known state before each test
func (suite *EtcdTestSuite) SetupTest() {
	config.Datadog.Set("autoconf_template_url", etcdURL)
	config.Datadog.Set("autoconf_template_dir", "/foo/")
	config.Datadog.Set("autoconf_template_username", "")
	config.Datadog.Set("autoconf_template_password", "")

	suite.populateEtcd()
	suite.toggleEtcdAuth(false)
}

func (suite *EtcdTestSuite) populateEtcd() {
	cl, err := etcd_client.New(suite.clientCfg)
	if err != nil {
		panic(err)
	}

	c := etcd_client.NewKeysAPI(cl)
	ctx := context.Background()

	for k, v := range suite.templates {
		_, err := c.Set(ctx, k, v, nil)
		if err != nil {
			panic(err)
		}
	}
}

func (suite *EtcdTestSuite) setEtcdPassword() {
	cl, err := etcd_client.New(suite.clientCfg)
	if err != nil {
		panic(err)
	}

	auth := etcd_client.NewAuthUserAPI(cl)
	ctx := context.Background()

	_, err = auth.ChangePassword(ctx, etcdUser, etcdPass)
	if err != nil && len(err.Error()) > 0 { // Flaky error with empty string ignored
		panic(err)
	}
}

func (suite *EtcdTestSuite) toggleEtcdAuth(enable bool) {
	cl, err := etcd_client.New(suite.clientCfg)
	if err != nil {
		panic(err)
	}

	c := etcd_client.NewAuthAPI(cl)
	ctx := context.Background()

	if enable {
		err = c.Enable(ctx)
	} else {
		err = c.Disable(ctx)
	}
	if err != nil && !strings.Contains(err.Error(), "auth: already") {
		panic(err)
	}
}

func (suite *EtcdTestSuite) TestWorkingConnectionAnon() {
	p, err := providers.NewEtcdConfigProvider()
	if err != nil {
		panic(err)
	}

	checks, err := p.Collect()
	if err != nil {
		panic(err)
	}

	assert.Equal(suite.T(), 2, len(checks))
	assert.Equal(suite.T(), "http_check", checks[0].Name)
	assert.Equal(suite.T(), "nginx", checks[1].Name)
}

func (suite *EtcdTestSuite) TestBadConnection() {
	config.Datadog.Set("autoconf_template_url", "http://127.0.0.1:1337")

	p, err := providers.NewEtcdConfigProvider()
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Empty(suite.T(), checks)
}

func (suite *EtcdTestSuite) TestWorkingAuth() {
	suite.toggleEtcdAuth(true)
	config.Datadog.Set("autoconf_template_username", etcdUser)
	config.Datadog.Set("autoconf_template_password", etcdPass)

	p, err := providers.NewEtcdConfigProvider()
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 2, len(checks))
}

func (suite *EtcdTestSuite) TestBadAuth() {
	suite.toggleEtcdAuth(true)
	config.Datadog.Set("autoconf_template_username", etcdUser)
	config.Datadog.Set("autoconf_template_password", "invalid")

	p, err := providers.NewEtcdConfigProvider()
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Empty(suite.T(), checks)
}

func TestEtcdSuite(t *testing.T) {
	suite.Run(t, new(EtcdTestSuite))
}
