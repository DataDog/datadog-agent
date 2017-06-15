package providers

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/samuel/go-zookeeper/zk"
)

// ZookeeperConfigProvider implements the Config Provider interface It should
// be called periodically and returns templates from Zookeeper for AutoConf.
type ZookeeperConfigProvider struct {
	client *zk.Conn
	event  <-chan zk.Event
}

const zkTimeout = 2 * time.Second

// NewZookeeperConfigProvider returns a new Client connected to a Zookeeper backend.
func NewZookeeperConfigProvider() (*Client, error) {
	tplURL, _, _ := getConnInfo()

	urls := strings.Split(tplURL, ",")
	c, evt, err := zk.Connect(urls, zkTimeout)
	if err != nil {
		return nil, fmt.Errorf("ZookeeperConfigProvider: couldn't connect to '%s': %s", tplURL, err)
	}
	backend := &ZookeeperConfigProvider{
		client: c,
		event:  evt,
	}

	client := &Client{}
	client.Init(backend)
	return client, nil
}

// List return the full path of every nodes inside a location
func (z *ZookeeperConfigProvider) List(key string) ([]string, error) {
	res, _, err := z.client.Children(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to list '%s' from zookeeper: %s", key, err)
	}

	// zookeeper pkg return children names, we have to return the full path
	paths := []string{}
	for _, p := range res {
		paths = append(paths, path.Join(key, p))
	}
	return paths, nil
}

// ListName return the name (not full Path) of every nodes inside a location
func (z *ZookeeperConfigProvider) ListName(key string) ([]string, error) {
	res, _, err := z.client.Children(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to list '%s' from zookeeper: %s", key, err)
	}
	return res, nil
}

// Get returns the value for a key
func (z *ZookeeperConfigProvider) Get(key string) ([]byte, error) {
	res, _, err := z.client.Get(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve %s from zookeeper: %s", key, err)
	}
	return res, nil
}
