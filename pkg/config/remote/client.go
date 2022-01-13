package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	m               sync.Mutex
	ctx             context.Context
	close           func()
	facts           Facts
	enabledProducts map[data.Product]struct{}
	pollInterval    time.Duration

	grpc          pbgo.AgentSecureClient
	partialClient *uptane.PartialClient
	configs       *configs

	lastPollErr error

	apmSamplingUpdates chan APMSamplingUpdate
}

// Facts are facts used to identify the client
type Facts struct {
	ID      string
	Name    string
	Version string
}

// NewClient creates a new client
func NewClient(facts Facts, products []data.Product) (*Client, error) {
	client, err := newClient(facts, products)
	if err != nil {
		return nil, err
	}
	go client.pollLoop()
	return client, nil
}

func newClient(facts Facts, products []data.Product, dialOpts ...grpc.DialOption) (*Client, error) {
	token, err := security.FetchAuthToken()
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire agent auth token")
	}
	ctx, close := context.WithCancel(context.Background())
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	grpcClient, err := agentgrpc.GetDDAgentSecureClient(ctx, dialOpts...)
	if err != nil {
		close()
		return nil, err
	}
	partialClient, err := uptane.NewPartialClient()
	if err != nil {
		close()
		return nil, err
	}
	enabledProducts := make(map[data.Product]struct{})
	for _, product := range products {
		enabledProducts[product] = struct{}{}
	}
	return &Client{
		ctx:                ctx,
		facts:              facts,
		enabledProducts:    enabledProducts,
		grpc:               grpcClient,
		close:              close,
		pollInterval:       1 * time.Second,
		partialClient:      partialClient,
		apmSamplingUpdates: make(chan APMSamplingUpdate, 8),
		configs:            newConfigs(),
	}, nil
}

// Close closes the client
func (c *Client) Close() {
	c.close()
	close(c.apmSamplingUpdates)
}

func (c *Client) pollLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.pollInterval):
			c.lastPollErr = c.poll()
			if c.lastPollErr != nil {
				log.Errorf("could not poll remote-config agent service: %v", c.lastPollErr)
			}
		}
	}
}

func (c *Client) products() []data.Product {
	var products []data.Product
	for product := range c.enabledProducts {
		products = append(products, product)
	}
	return products
}

func (c *Client) poll() error {
	c.m.Lock()
	defer c.m.Unlock()
	state := c.partialClient.State()
	lastPollErr := ""
	if c.lastPollErr != nil {
		lastPollErr = c.lastPollErr.Error()
	}
	response, err := c.grpc.ClientGetConfigs(c.ctx, &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			Id:      c.facts.ID,
			Name:    c.facts.Name,
			Version: c.facts.Version,
			State: &pbgo.ClientState{
				RootVersion:    state.RootVersion,
				TargetsVersion: state.TargetsVersion,
				Configs:        c.configs.state(),
				HasError:       c.lastPollErr != nil,
				Error:          lastPollErr,
			},
			Products: data.ProductListToString(c.products()),
		},
	})
	if err != nil {
		return err
	}
	err = c.partialClient.Update(response)
	if err != nil {
		return err
	}
	configFiles, err := c.buildConfigFiles()
	if err != nil {
		return err
	}
	updates := c.configs.update(c.products(), configFiles)
	c.publishUpdates(updates)
	return nil
}

func (c *Client) buildConfigFiles() (configFiles, error) {
	targets, err := c.partialClient.Targets()
	if err != nil {
		return nil, err
	}
	var configFiles configFiles
	for targetPath, target := range targets {
		targetPathMeta, err := data.ParseFilePathMeta(targetPath)
		if err != nil {
			return nil, err
		}
		if _, productEnabled := c.enabledProducts[targetPathMeta.Product]; productEnabled {
			targetContent, err := c.partialClient.TargetFile(targetPath)
			if err != nil {
				return nil, err
			}
			targetVersion, err := targetVersion(target.Custom)
			if err != nil {
				return nil, err
			}
			configFiles = append(configFiles, configFile{
				pathMeta: targetPathMeta,
				version:  targetVersion,
				raw:      targetContent,
			})
		}
	}
	return configFiles, nil
}

func (c *Client) publishUpdates(update update) {
	if update.apmSamplingUpdate != nil {
		select {
		case c.apmSamplingUpdates <- *update.apmSamplingUpdate:
		default:
			log.Warnf("apm sampling update queue is full, dropping configuration")
		}
	}
}

// APMSamplingUpdates returns a chan to consume apm sampling updates
func (c *Client) APMSamplingUpdates() <-chan APMSamplingUpdate {
	return c.apmSamplingUpdates
}
