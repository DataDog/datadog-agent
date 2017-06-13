package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/coreos/etcd/client"
)

// EtcdConfigProvider implements the Config Provider interface
// It should be called periodically and returns templates from etcd for AutoConf.
type EtcdConfigProvider struct {
	Client client.KeysAPI
}

// NewEtcdConfigProvider returns a new Client connected to a etcd backend.
func NewEtcdConfigProvider() (*Client, error) {
	tplURL, etcdUsername, etcdPassword := getConnInfo()

	clientCfg := client.Config{
		Endpoints:               []string{tplURL},
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	if len(etcdUsername) > 0 && len(etcdPassword) > 0 {
		log.Info("Using provided etcd credentials: username ", etcdUsername)
		clientCfg.Username = etcdUsername
		clientCfg.Password = etcdPassword
	}

	cl, err := client.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to instantiate the etcd client: %s", err)
	}

	c := client.NewKeysAPI(cl)
	backend := &EtcdConfigProvider{c}

	client := &Client{}
	client.Init(backend)
	return client, nil
}

// List return the full path of every nodes inside a location
func (p *EtcdConfigProvider) List(key string) ([]string, error) {
	resp, err := p.Client.Get(context.Background(), key, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to list '%s' from etcd: %s", key, err)
	}

	identifiers := make([]string, 0)
	for _, node := range resp.Node.Nodes {
		identifiers = append(identifiers, node.Key)
	}
	return identifiers, nil
}

// ListName return the name (not full Path) of every nodes inside a location
func (p *EtcdConfigProvider) ListName(key string) ([]string, error) {
	resp, err := p.Client.Get(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}

	identifiers := make([]string, 0)
	for _, node := range resp.Node.Nodes {
		split := strings.Split(node.Key, "/")
		identifiers = append(identifiers, split[len(split)-1])
	}
	return identifiers, nil
}

// Get returns the value for a key
func (p *EtcdConfigProvider) Get(key string) ([]byte, error) {
	resp, err := p.Client.Get(context.Background(), key, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve %s from etcd: %s", key, err)
	}

	return []byte(resp.Node.Value), nil
}
