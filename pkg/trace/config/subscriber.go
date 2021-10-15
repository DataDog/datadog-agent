package config

import (
	"context"
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Subscriber subscribes for configuration updates from agent-core.
// Provides to trace-agent clients the latest available configurations.
type Subscriber struct {
	mu sync.RWMutex
	// configs is the latest debugging configurations available
	configs        pbgo.ConfigResponse
	once           sync.Once
	stopSubscriber context.CancelFunc
}

// NewSubscriber returns a new configuration store
func NewSubscriber() *Subscriber {
	return &Subscriber{}
}

// Get returns the latest available configurations
func (s *Subscriber) Get(req *pbgo.GetConfigsRequest) (*pbgo.ConfigResponse, error) {
	if req.Product != pbgo.Product_LIVE_DEBUGGING {
		return nil, errors.New("not allowed")
	}
	s.once.Do(s.subscribe)
	// No new configurations available in store
	if req.CurrentConfigProductVersion >= s.getCurrentVersion() {
		return nil, nil
	}
	return s.respond(req), nil
}

// respond generates a new config response
func (s *Subscriber) respond(req *pbgo.GetConfigsRequest) *pbgo.ConfigResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := &pbgo.ConfigResponse{
		ConfigDelegatedTargetVersion: s.configs.ConfigDelegatedTargetVersion,
		ConfigSnapshotVersion:        s.configs.ConfigSnapshotVersion,
		DirectoryTargets:             topMetaCopy(s.configs.DirectoryTargets),
		TargetFiles:                  targetFilesCopy(s.configs.TargetFiles),
	}
	for _, root := range s.configs.DirectoryRoots {
		if root.Version <= req.CurrentDirectorRootVersion {
			continue
		}
		res.DirectoryRoots = append(res.DirectoryRoots, topMetaCopy(root))

	}
	return res
}

// Stop listening for new configurations
func (s *Subscriber) Stop() {
	if s.stopSubscriber != nil {
		s.stopSubscriber()
	}
}

func (s *Subscriber) getCurrentVersion() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configs.ConfigDelegatedTargetVersion
}

func (s *Subscriber) subscribe() {
	close, err := service.NewGRPCSubscriber(pbgo.Product_LIVE_DEBUGGING, s.loadNewConfig)
	if err != nil {
		log.Errorf("Error when subscribing to remote config management %v", err)
		return
	}
	s.mu.Lock()
	s.stopSubscriber = close
	s.mu.Unlock()
}

func (s *Subscriber) loadNewConfig(new *pbgo.ConfigResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	new.DirectoryRoots = append(s.configs.DirectoryRoots, new.DirectoryRoots...)
	s.configs = *new
	return nil
}

func topMetaCopy(old *pbgo.TopMeta) *pbgo.TopMeta {
	rawCopy := make([]byte, len(old.Raw))
	copy(rawCopy, old.Raw)
	return &pbgo.TopMeta{
		Version: old.Version,
		Raw:     rawCopy,
	}
}

func targetFilesCopy(old []*pbgo.File) []*pbgo.File {
	if old == nil {
		return nil
	}
	new := make([]*pbgo.File, 0, len(old))
	for _, f := range old {
		rawCopy := make([]byte, len(f.Raw))
		copy(rawCopy, f.Raw)
		new = append(new, &pbgo.File{
			Path: f.Path,
			Raw:  rawCopy,
		})
	}
	return new
}
