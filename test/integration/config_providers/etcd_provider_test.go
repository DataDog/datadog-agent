package integration

import (
	"context"
	"testing"
	"time"

	etcd_client "github.com/coreos/etcd/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	etcdImg string = "quay.io/coreos/etcd:latest"
)

var etcdAddr string

var templates = map[string]string{
	"/foo/nginx/check_names":  "[\"http_check\", \"nginx\"]",
	"/foo/nginx/init_configs": "[{}, {}]",
	"/foo/nginx/instances":    "[{\"name\": \"test\", \"url\": \"http://%25%25host%25%25/\", \"timeout\": 5}, {\"foo\": \"bar\"}]",
}

// pull the latest etcd image, create a standalone etcd container
func setup() string {

	cID := createEtcd()
	etcdAddr := getEtcdAddr(cID)

	etcdURL := "http://" + etcdAddr + ":2379/"
	config.Datadog.Set("autoconf_template_url", etcdURL)
	config.Datadog.Set("autoconf_template_dir", "/foo/")

	// wait for etcd to start
	time.Sleep(time.Second)

	populateEtcd(etcdURL)

	return cID
}

func createEtcd() string {
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
			match = true
			break
		}
	}
	if !match {
		_, err = cli.ImagePull(ctx, etcdImg, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
	}

	containerConfig := &container.Config{
		Image: etcdImg,
		Cmd: []string{
			"/usr/local/bin/etcd",
			"-name", "etcd0",
			"-advertise-client-urls", "http://0.0.0.0:2379",
			"-listen-client-urls", "http://0.0.0.0:2379",
			"-initial-advertise-peer-urls", "http://127.0.0.1:2379",
			"-listen-peer-urls", "http://0.0.0.0:2800",
			"-initial-cluster-token", "etcd-cluster-1",
			"-initial-cluster", "etcd0=http://127.0.0.1:2379",
			"-initial-cluster-state", "new",
		},
	}

	resp, err := cli.ContainerCreate(ctx, containerConfig, nil, nil, "etcd0")
	if err != nil {
		panic(err)
	}
	cID := resp.ID

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	return cID
}

func populateEtcd(addr string) {
	// get etcd client
	clientCfg := etcd_client.Config{
		Endpoints:               []string{addr},
		Transport:               etcd_client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}
	cl, err := etcd_client.New(clientCfg)
	if err != nil {
		panic(err)
	}
	c := etcd_client.NewKeysAPI(cl)

	ctx := context.Background()

	for k, v := range templates {
		_, err := c.Set(ctx, k, v, nil)
		if err != nil {
			panic(err)
		}
	}
}

func getEtcdAddr(cID string) string {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	co, err := cli.ContainerInspect(context.Background(), cID)
	if err != nil {
		panic(err)
	}

	return co.NetworkSettings.IPAddress
}

// TODO: handle image
func teardown(cID string) {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	cli.ContainerRemove(ctx, cID, types.ContainerRemoveOptions{Force: true})
}

func TestMain(m *testing.M) {
	cID := setup()

	m.Run()

	teardown(cID)
}

func TestWorkingConnection(t *testing.T) {
	p, err := providers.NewEtcdConfigProvider()
	if err != nil {
		panic(err)
	}
	checks, err := p.Collect()
	if err != nil {
		panic(err)
	}

	assert.Equal(t, len(checks), 2)
}

func TestBadConnection(t *testing.T) {
	config.Datadog.Set("autoconf_template_url", "http://127.0.0.1:1337")

	p, err := providers.NewEtcdConfigProvider()
	assert.Nil(t, err)

	checks, err := p.Collect()
	assert.Nil(t, err)
	assert.Empty(t, checks)
}
