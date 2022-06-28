package remoteconfig

import (
	"encoding/json"
	"fmt"

	rcdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/theupdateframework/go-tuf/data"
)

// uptaneClient defines what is needed to perform uptane checks and return the current tuf state
type UptaneClient interface {
	Update(response *pbgo.LatestConfigsResponse) error
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetsMeta() ([]byte, error)
	TargetsCustom() ([]byte, error)
	TUFVersionState() (uptane.TUFVersions, error)
}

// BackendClient is a client for requesting and managing updates from the Remote Configuration backend
//
// The client mostly focuses on the update request/response lifecycle management, processing updates
// and tracking any downstream clients that the client needs to request configs for on their behalf.
// Notably, the networking layer is not handled by the client. This makes it easier to test certain
// aspects of the client by making it agnostic to how updates are retrieved.
type BackendClient struct {
	// Client information
	hostname string
	version  string

	// Products and new products (used when building update requests)
	products    map[rcdata.Product]struct{}
	newProducts map[rcdata.Product]struct{}

	// Stores TUF metadata and config files and runs TUF verifications
	UptaneClient

	// List of downstream clients that this client needs to ask for configs for from the backend
	downstreamClients *ClientTracker

	// Informs the backend of any recent errors for DD telemetry
	lastUpdateErr error
}

// NewBackendClient creates a new BackendClient
func NewBackendClient(hostname string, version string, uptaneClient UptaneClient, clientTracker *ClientTracker) *BackendClient {
	return &BackendClient{
		hostname:          hostname,
		version:           version,
		products:          make(map[rcdata.Product]struct{}),
		newProducts:       make(map[rcdata.Product]struct{}),
		UptaneClient:      uptaneClient,
		downstreamClients: clientTracker,
		lastUpdateErr:     nil,
	}
}

// Apply applies the provided update from the remote config backend
func (c *BackendClient) Apply(update *pbgo.LatestConfigsResponse) error {
	err := c.Update(update)
	if err != nil {
		c.lastUpdateErr = err
		return err
	}

	for product := range c.newProducts {
		c.products[product] = struct{}{}
	}
	c.newProducts = make(map[rcdata.Product]struct{})

	return err
}

// BuildUpdateRequest builds a request struct that the remote config backend can process.
func (c *BackendClient) BuildUpdateRequest(forceRefresh bool) (*pbgo.LatestConfigsRequest, error) {
	activeClients := c.downstreamClients.ActiveClients()
	c.refreshProducts(activeClients)

	previousTUFVersions := uptane.TUFVersions{}

	var err error
	if !forceRefresh {
		previousTUFVersions, err = c.TUFVersionState()
		if err != nil {
			fmt.Printf("error in tuf version state\n")
			log.Warnf("could not get previous TUF version state: %v", err)
			if c.lastUpdateErr != nil {
				c.lastUpdateErr = fmt.Errorf("%v: %v", err, c.lastUpdateErr)
			} else {
				c.lastUpdateErr = err
			}
		}
	}

	backendState, err := c.getOpaqueBackendState()
	if err != nil {
		fmt.Printf("error in get opaque state\n")
		log.Warnf("could not get previous backend client state: %v", err)
		if c.lastUpdateErr != nil {
			c.lastUpdateErr = fmt.Errorf("%v: %v", err, c.lastUpdateErr)
		} else {
			c.lastUpdateErr = err
		}
	}

	request := buildLatestConfigsRequest(c.hostname, previousTUFVersions, activeClients, c.products, c.newProducts, c.lastUpdateErr, backendState)

	return request, nil
}

// TrackDownstreamClient registers the given downstream client so that the BackendClient will ask for
// configs on its behalf based on its needs.
func (c *BackendClient) TrackDownstreamClient(client *pbgo.Client) {
	c.downstreamClients.Seen(client)
}

// refreshProducts goes through the current active client list and determines what products
// are new to this update.
//
// If the backend TUF state hasn't changed, but we have a new product, the backend needs to know
// that it needs to process the update still for new files related to that product.
func (c *BackendClient) refreshProducts(activeClients []*pbgo.Client) {
	for _, client := range activeClients {
		for _, product := range client.Products {
			if _, hasProduct := c.products[rcdata.Product(product)]; !hasProduct {
				c.newProducts[rcdata.Product(product)] = struct{}{}
			}
		}
	}
}

// getOpaqueBackendState retrieves the blob of data stored in the Director repo's Targets
// metadata file.
//
// The backend uses this data for its own tracking purposes and needs it sent back to it
// on every update request.
func (c *BackendClient) getOpaqueBackendState() ([]byte, error) {
	rawTargetsCustom, err := c.TargetsCustom()
	if err != nil {
		return nil, err
	} else if len(rawTargetsCustom) == 0 {
		return nil, nil
	}

	custom, err := parseTargetsCustom(rawTargetsCustom)
	if err != nil {
		return nil, err
	}
	return custom.OpaqueBackendState, nil
}

// buildLatestConfigsRequest is a helper function for constructing a new update request from the remote config backend
func buildLatestConfigsRequest(hostname string, state uptane.TUFVersions, activeClients []*pbgo.Client, products map[rcdata.Product]struct{}, newProducts map[rcdata.Product]struct{}, lastUpdateErr error, clientState []byte) *pbgo.LatestConfigsRequest {
	productsList := make([]rcdata.Product, len(products))
	i := 0
	for k := range products {
		productsList[i] = k
		i++
	}
	newProductsList := make([]rcdata.Product, len(newProducts))
	i = 0
	for k := range newProducts {
		newProductsList[i] = k
		i++
	}

	lastUpdateErrString := ""
	if lastUpdateErr != nil {
		lastUpdateErrString = lastUpdateErr.Error()
	}
	return &pbgo.LatestConfigsRequest{
		Hostname:                     hostname,
		AgentVersion:                 version.AgentVersion,
		Products:                     rcdata.ProductListToString(productsList),
		NewProducts:                  rcdata.ProductListToString(newProductsList),
		CurrentConfigSnapshotVersion: state.ConfigSnapshot,
		CurrentConfigRootVersion:     state.ConfigRoot,
		CurrentDirectorRootVersion:   state.DirectorRoot,
		ActiveClients:                activeClients,
		BackendClientState:           clientState,
		HasError:                     lastUpdateErr != nil,
		Error:                        lastUpdateErrString,
	}
}

// targetsCustom contains data not related to the TUF protocol sent by the backend
type targetsCustom struct {
	// OpaqueBackendState is used by the backend and must be sent back with every update request
	OpaqueBackendState []byte `json:"opaque_backend_state"`
}

// parseTargetsCustom parses the custom data stored in the TUF custom field by the backend
func parseTargetsCustom(rawTargetsCustom []byte) (targetsCustom, error) {
	var custom targetsCustom
	err := json.Unmarshal(rawTargetsCustom, &custom)
	if err != nil {
		return targetsCustom{}, err
	}
	return custom, nil
}
