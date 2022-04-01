package client

import (
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/internal/uptane"
)

var (
	idSize     = 21
	idAlphabet = []rune("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// Client is a remoteconfig client
type Client struct {
	m        sync.Mutex
	id       string
	products map[string]struct{}

	partialClient  *uptane.PartialClient
	currentTargets *uptane.PartialClientTargets

	currentConfigs *configList
}

func NewClient(embededRoot []byte, products []string) *Client {
	productsMap := make(map[string]struct{})
	for _, product := range products {
		productsMap[product] = struct{}{}
	}
	return &Client{
		id:             generateID(),
		products:       productsMap,
		partialClient:  uptane.NewPartialClient(embededRoot),
		currentTargets: &uptane.PartialClientTargets{},
		currentConfigs: newConfigList(),
	}
}

func (c *Client) ID() string {
	return c.id
}

func generateID() string {
	bytes := make([]byte, idSize)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	id := make([]rune, idSize)
	for i := 0; i < idSize; i++ {
		id[i] = idAlphabet[bytes[i]&63]
	}
	return string(id[:idSize])
}

type State struct {
	RootVersion    int64
	TargetsVersion int64
}

func (c *Client) State() State {
	return State{
		RootVersion:    c.partialClient.RootVersion(),
		TargetsVersion: c.currentTargets.Version(),
	}
}

func (c *Client) GetConfigs(time int64) Configs {
	c.m.Lock()
	defer c.m.Unlock()
	return c.currentConfigs.getCurrentConfigs(c.id, time)
}

type File struct {
	Path string
	Raw  []byte
}

type Update struct {
	Roots       [][]byte
	Targets     []byte
	TargetFiles map[string][]byte
}

func (c *Client) Update(update Update) error {
	c.m.Lock()
	defer c.m.Unlock()
	if len(update.Roots) == 0 && len(update.Targets) == 0 {
		return nil
	}
	newTargets, err := c.partialClient.Update(update.Roots, c.currentTargets, update.Targets, update.TargetFiles)
	if err != nil {
		return err
	}
	newConfigs := newConfigList()
	for targetPath, targetMeta := range newTargets.Targets() {
		configMeta, err := parseConfigMeta(targetPath, *targetMeta.Custom)
		if err != nil {
			return err
		}
		_, hasProduct := c.products[configMeta.path.Product]
		if !hasProduct || !configMeta.scopedToClient(c.id) {
			continue
		}
		configContents, found := newTargets.TargetFile(targetPath)
		if !found {
			return fmt.Errorf("missing config file: %s", targetPath)
		}
		config := config{
			meta:     configMeta,
			contents: configContents,
			hash:     configHash(configMeta, configContents),
		}
		err = newConfigs.addConfig(config)
		if err != nil {
			return err
		}
	}
	c.currentConfigs = newConfigs
	c.currentTargets = newTargets
	return nil
}
