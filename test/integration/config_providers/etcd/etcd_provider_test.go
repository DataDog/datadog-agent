// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package etcd

import (
	"context"
	"strings"
	"testing"
	"time"

	etcd_client "github.com/coreos/etcd/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	etcdUser string = "root"
	etcdPass string = "root"
)

type EtcdTestSuite struct {
	suite.Suite
	templates     map[string]string
	clientCfg     etcd_client.Config
	containerName string
	etcdVersion   string
	etcdURL       string
	compose       *utils.ComposeConf
}

// use a constructor to make the suite parametric
func NewEtcdTestSuite(etcdVersion, containerName string) *EtcdTestSuite {
	return &EtcdTestSuite{
		containerName: containerName,
		etcdVersion:   etcdVersion,
	}
}

func (suite *EtcdTestSuite) SetupSuite() {
	suite.compose = &utils.ComposeConf{
		ProjectName: "etcd",
		FilePath:    "testdata/etcd.compose",
		Variables: map[string]string{
			"version": suite.etcdVersion,
		},
	}

	output, err := suite.compose.Start()
	require.NoError(suite.T(), err, string(output))

	suite.templates = map[string]string{
		"/foo/nginx/check_names":  `["http_check", "nginx"]`,
		"/foo/nginx/init_configs": `[{}, {}]`,
		"/foo/nginx/instances":    `[{"name": "test", "url": "http://%25%25host%25%25/", "timeout": 5}, {"foo": "bar"}]`,
	}

	suite.etcdURL = "http://localhost:2379/"

	suite.clientCfg = etcd_client.Config{
		Endpoints:               []string{suite.etcdURL},
		Transport:               etcd_client.DefaultTransport,
		HeaderTimeoutPerRequest: 1 * time.Second,
		Username:                etcdUser,
		Password:                etcdPass,
	}

	suite.setEtcdPassword()
}

func (suite *EtcdTestSuite) TearDownSuite() {
	suite.compose.Stop()
}

// put configuration back in a known state before each test
func (suite *EtcdTestSuite) SetupTest() {
	mockConfig := config.NewMock()
	mockConfig.Set("autoconf_template_dir", "/foo/")

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
	config := config.ConfigurationProviders{
		TemplateURL: suite.etcdURL,
		TemplateDir: "/foo",
	}
	p, err := providers.NewEtcdConfigProvider(config)
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
	config := config.ConfigurationProviders{
		TemplateURL: "http://127.0.0.1:1337",
		TemplateDir: "/foo",
	}
	p, err := providers.NewEtcdConfigProvider(config)
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Empty(suite.T(), checks)
}

func (suite *EtcdTestSuite) TestWorkingAuth() {
	suite.toggleEtcdAuth(true)
	config := config.ConfigurationProviders{
		TemplateURL: suite.etcdURL,
		TemplateDir: "/foo",
		Username:    etcdUser,
		Password:    etcdPass,
	}
	p, err := providers.NewEtcdConfigProvider(config)
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 2, len(checks))
}

func (suite *EtcdTestSuite) TestBadAuth() {
	suite.toggleEtcdAuth(true)
	config := config.ConfigurationProviders{
		TemplateURL: suite.etcdURL,
		TemplateDir: "/foo",
		Username:    etcdUser,
		Password:    "invalid",
	}
	p, err := providers.NewEtcdConfigProvider(config)
	assert.Nil(suite.T(), err)

	checks, err := p.Collect()
	assert.Nil(suite.T(), err)
	assert.Empty(suite.T(), checks)
}

func TestEtcdSuite(t *testing.T) {
	suite.Run(t, NewEtcdTestSuite("3_2_6", "datadog-agent-test-etcd"))
}
