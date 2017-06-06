package integration

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	etcd_client "github.com/coreos/etcd/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	etcdImg       string = "quay.io/coreos/etcd:latest"
	containerName string = "datadog-agent-etcd0"
	etcdURL       string = "http://127.0.0.1:2379/"
	etcdUser      string = "root"
	etcdPass      string = "root"
)

type EtcdTestSuite struct {
	suite.Suite
	templates map[string]string
	clientCfg etcd_client.Config
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
	suite.createEtcd()

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

	cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})
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

func (suite *EtcdTestSuite) createEtcd() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	l, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		panic(err)
	}

	match := false
	for _, img := range l {
		if img.RepoTags[0] == etcdImg {
			suite.T().Logf("Found image %s", etcdImg)
			match = true
			break
		}
	}

	if !match {
		suite.T().Logf("Image %s not found, pulling", etcdImg)
		resp, err := cli.ImagePull(ctx, etcdImg, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		_, err = ioutil.ReadAll(resp) // Necessary for image pull to complete
		resp.Close()
		if err != nil {
			panic(err)
		}
	}

	containerConfig := &container.Config{
		Image: etcdImg,
		Cmd: []string{
			"/usr/local/bin/etcd",
			"-advertise-client-urls", "http://127.0.0.1:2379",
			"-listen-client-urls", "http://0.0.0.0:2379",
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"2379/tcp": []nat.PortBinding{nat.PortBinding{HostPort: "2379"}},
		},
	}

	_, err = cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, containerName)
	if err != nil {
		// containers already exists
		suite.T().Logf("Error creating container %s: %s", containerName, err)
	}

	if err := cli.ContainerStart(ctx, containerName, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}
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
