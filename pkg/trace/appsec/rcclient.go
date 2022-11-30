package appsec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	rc "github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
)

type ProductUpdate map[string][]byte

type Callback func(u ProductUpdate) error

// ClientConfig contains the required values to configure a remoteconfig client
type ClientConfig struct {
	// The env this tracer is running in
	Env string
	// The rate at which the client should poll the agent for updates
	PollRate time.Duration
	// A list of remote config products this client is interested in
	Products []string
	// The tracer's runtime id
	RuntimeID string
	// The name of the user's application
	ServiceName  string
	Capabilities []byte
}

// A Client interacts with an Agent to update and track the state of remote
// configuration
type Client struct {
	ClientConfig
	grpcClient pbgo.AgentSecureClient
	ctx        context.Context
	close      context.CancelFunc

	clientID   string
	repository *rc.Repository
	stop       chan struct{}

	callbacks map[string][]Callback

	lastError error
}

// NewClient creates a new remoteconfig Client
func NewClient(config ClientConfig) (*Client, error) {
	ctx, close := context.WithCancel(context.Background())

	grpcClient, err := grpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		close()
		return nil, fmt.Errorf("could not instantiate the tracer remote config client: %v", err)
	}

	repo, err := rc.NewUnverifiedRepository()
	if err != nil {
		close()
		return nil, err
	}

	return &Client{
		ClientConfig: config,
		grpcClient:   grpcClient,
		ctx:          ctx,
		close:        close,
		clientID:     generateID(),
		repository:   repo,
		stop:         make(chan struct{}),
		lastError:    nil,
		callbacks:    map[string][]Callback{},
	}, nil
}

func (c *Client) RegisterCallback(f Callback, product string) {
	c.callbacks[product] = append(c.callbacks[product], f)
}

// Start starts the client's update poll loop
func (c *Client) Start() {
	ticker := time.NewTicker(c.PollRate)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.lastError = c.updateState()
			if c.lastError != nil {
				log.Errorf("appsec: could not update remote-config state: %v", c.lastError)
			}
		}
	}
}

// Stop stops the client's update poll loop
func (c *Client) Stop() {
	close(c.stop)
	c.close()
}

func (c *Client) updateState() error {
	req, err := c.newUpdateRequest()
	if err != nil {
		return err
	}

	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("could not acquire agent auth token: %v", err)
	}
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx := metadata.NewOutgoingContext(c.ctx, md)

	response, err := c.grpcClient.ClientGetConfigs(ctx, req)
	if err != nil {
		return err
	}

	return c.applyUpdate(response)
}

func (c *Client) applyUpdate(pbUpdate *pbgo.ClientGetConfigsResponse) error {
	fileMap := make(map[string][]byte, len(pbUpdate.TargetFiles))
	productUpdates := make(map[string]ProductUpdate, len(c.Products))
	for _, f := range pbUpdate.TargetFiles {
		fileMap[f.Path] = f.Raw
		for _, p := range c.Products {
			productUpdates[p] = make(ProductUpdate)
			if strings.Contains(f.Path, p) {
				productUpdates[p][f.Path] = f.Raw
			}
		}
	}

	update := rc.Update{
		TUFRoots:      pbUpdate.Roots,
		TUFTargets:    pbUpdate.Targets,
		TargetFiles:   fileMap,
		ClientConfigs: pbUpdate.ClientConfigs,
	}

	mapify := func(s *rc.RepositoryState) map[string]string {
		m := make(map[string]string)
		for i := range s.Configs {
			path := s.CachedFiles[i].Path
			product := s.Configs[i].Product
			m[path] = product
		}
		return m
	}

	// Check the repository state before and after the update to detect which configs are not being sent anymore.
	// This is needed because some products can stop sending configurations, and we want to make sure that the subscribers
	// are provided with this information in this case
	stateBefore, _ := c.repository.CurrentState()
	products, err := c.repository.Update(update)
	if err != nil {
		return err
	}
	stateAfter, _ := c.repository.CurrentState()

	// Create a config files diff between before/after the update to see which config files are missing
	mBefore := mapify(&stateBefore)
	mAfter := mapify(&stateAfter)
	for k := range mAfter {
		delete(mBefore, k)
	}

	// Set the payload data to nil for missing config files. The callbacks then can handle the nil config case to detect
	// that this config will not be updated anymore.
	updatedProducts := make(map[string]bool)
	for path, product := range mBefore {
		if productUpdates[product] == nil {
			productUpdates[product] = make(ProductUpdate)
		}
		productUpdates[product][path] = nil
		updatedProducts[product] = true
	}
	// Aggregate updated products and missing products so that callbacks get called for both
	for _, p := range products {
		updatedProducts[p] = true
	}

	// Performs the callbacks registered for all updated products and update the application status in the repository
	// (RCTE2)
	for p := range updatedProducts {
		for _, fn := range c.callbacks[p] {
			err := fn(productUpdates[p])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) newUpdateRequest() (*pbgo.ClientGetConfigsRequest, error) {
	state, err := c.repository.CurrentState()
	if err != nil {
		return nil, err
	}

	if state.RootsVersion < 1 {
		state.RootsVersion = 1
	}

	pbCachedFiles := make([]*pbgo.TargetFileMeta, 0, len(state.CachedFiles))
	for _, f := range state.CachedFiles {
		pbHashes := make([]*pbgo.TargetFileHash, 0, len(f.Hashes))
		for alg, hash := range f.Hashes {
			pbHashes = append(pbHashes, &pbgo.TargetFileHash{
				Algorithm: alg,
				Hash:      hex.EncodeToString(hash),
			})
		}
		pbCachedFiles = append(pbCachedFiles, &pbgo.TargetFileMeta{
			Path:   f.Path,
			Length: int64(f.Length),
			Hashes: pbHashes,
		})
	}

	hasError := c.lastError != nil
	errMsg := ""
	if hasError {
		errMsg = c.lastError.Error()
	}

	pbConfigState := make([]*pbgo.ConfigState, 0, len(state.Configs))
	for _, f := range state.Configs {
		pbConfigState = append(pbConfigState, &pbgo.ConfigState{
			Id:      f.ID,
			Version: f.Version,
			Product: f.Product,
		})
	}

	req := pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			State: &pbgo.ClientState{
				RootVersion:    uint64(state.RootsVersion),
				TargetsVersion: uint64(state.TargetsVersion),
				ConfigStates:   pbConfigState,
				HasError:       hasError,
				Error:          errMsg,
			},
			Id:       c.clientID,
			Products: c.Products,
			IsTracer: true,
			ClientTracer: &pbgo.ClientTracer{
				RuntimeId:     c.RuntimeID,
				Language:      "cpp",
				TracerVersion: "0.1",
				Service:       c.ServiceName,
				Env:           c.Env,
			},
			Capabilities: c.Capabilities,
		},
		CachedTargetFiles: pbCachedFiles,
	}

	return &req, nil
}

var (
	idSize     = 21
	idAlphabet = []rune("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

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
