package client

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/internal/uptane"
)

// Client is a remoteconfig client
type Client struct {
	m sync.Mutex

	products      map[string]struct{}
	partialClient *uptane.PartialClient
}

func NewClient(embededRoot []byte) *Client {
	return &Client{
		products:      make(map[string]struct{}),
		partialClient: uptane.NewPartialClient(embededRoot),
	}
}

func (c *Client) AddProduct(product string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.products[product] = struct{}{}
}

func (c *Client) RemoveProduct(product string) {
	c.m.Lock()
	defer c.m.Unlock()
	delete(c.products, product)
}

func (c *Client) GetConfigs(time int64) {
	c.m.Lock()
	defer c.m.Unlock()
}

type File struct {
	Path string
	Raw  []byte
}

type Update struct {
	Roots       [][]byte
	Targets     []byte
	TargetFiles []File
}

func (c *Client) Update(update Update) error {
	c.m.Lock()
	defer c.m.Unlock()
	targetFiles := make(map[string][]byte)
	for _, targetFile := range update.TargetFiles {
		targetFiles[targetFile.Path] = targetFile.Raw
	}
	return c.partialClient.Update(update.Roots, update.Targets, targetFiles)
}
