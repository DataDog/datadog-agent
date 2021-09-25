package config

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

var (
	allowedProducts = map[string]struct{}{
		"LIVE_DEBUGGING": {},
	}
	errNotAllowed = errors.New("product not allowed")
)

// Store stores config provided by agent-core and returns
// latest configs to tracers.
type Store struct {
	mu      sync.RWMutex
	configs map[string]pbgo.ConfigResponse
}

// NewStore returns a new configuration store
func NewStore() *Store {
	return &Store{
		configs: make(map[string]pbgo.ConfigResponse),
	}
}

// IsAllowed returns whether a product is valid
func (s *Store) IsAllowed(product string) bool {
	_, ok := allowedProducts[product]
	return ok
}

// Get returns the latest configuration for a product
func (s *Store) Get(previousVersion uint64, product string) (*pbgo.ConfigResponse, bool, error) {
	if ok := s.IsAllowed(product); !ok {
		return nil, false, errNotAllowed
	}

	s.mu.RLock()
	cfg, ok := s.configs[product]
	s.mu.RUnlock()
	if !ok {
		err := s.newSubscriber(product)
		return nil, false, err
	}

	// todo: what happens when tracer version > agent version
	if remoteVersion := cfg.DirectoryTargets.Version; previousVersion >= remoteVersion {
		log.Warnf("A more recent version was already present (tracer version: %d, agent version: %d)", previousVersion, cfg.DirectoryTargets.Version)
		return nil, true, nil
	}

	return &cfg, true, nil
}

func (s *Store) newSubscriber(product string) error {
	log.Infof("Registering %s subscriber", product)
	productID, found := pbgo.Product_value[product]
	if !found {
		return fmt.Errorf("unknown product '%s'", product)
	}

	return service.NewGRPCSubscriber(pbgo.Product(productID), func(config *pbgo.ConfigResponse) error {
		log.Debugf("Fetched config version %d from remote config management", config.DirectoryTargets.Version)

		s.mu.Lock()
		defer s.mu.Unlock()
		// todo: trim diffs in function of last snapshot available version
		s.configs[product] = *config

		return nil
	})
}
