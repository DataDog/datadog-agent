package config

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// DisruptionData represents the full disruption data from etcd (with metadata)
type DisruptionData struct {
	Component      string           `json:"component"`
	DisruptionType string           `json:"disruptionType"`
	Category       string           `json:"category"`
	ContainerName  string           `json:"containerName"`
	Config         DisruptionConfig `json:"config"`
}

// DisruptionConfig holds configuration for a disruption
type DisruptionConfig struct {
	Enabled   bool    `json:"enabled"`
	DelayMs   int     `json:"delay_ms"`
	ErrorRate float64 `json:"error_rate"`
	TimeoutMs int     `json:"timeout_ms"`
}

// ConfigEntry holds a simple key/value configuration
type ConfigEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Manager handles configuration values from etcd (including disruption toggles)
type Manager struct {
	etcdClient   *clientv3.Client
	configCache  map[string]*DisruptionConfig
	configValues map[string]string
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewManager creates a new config manager backed by etcd
func NewManager() (*Manager, error) {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		log.Println("ETCD_ENDPOINTS not set, config manager disabled (using defaults)")
		return &Manager{
			configCache:  make(map[string]*DisruptionConfig),
			configValues: make(map[string]string),
		}, nil
	}

	endpointList := strings.Split(endpoints, ",")

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpointList,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		etcdClient:   cli,
		configCache:  make(map[string]*DisruptionConfig),
		configValues: make(map[string]string),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start background watcher
	go m.watchConfigEntries()
	go m.watchConfigValues()

	// Initial sync
	go m.syncConfigEntries()
	go m.syncConfigValues()

	return m, nil
}

// Close shuts down the config manager
func (m *Manager) Close() {
	if m.etcdClient != nil {
		m.cancel()
		m.etcdClient.Close()
	}
}

// syncConfigEntries syncs disruption configuration entries with etcd
func (m *Manager) syncConfigEntries() {
	if m.etcdClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := m.etcdClient.Get(ctx, "config/", clientv3.WithPrefix())
	if err != nil {
		log.Printf("Failed to sync config cache: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		value := string(kv.Value)

		var data DisruptionData
		if err := json.Unmarshal([]byte(value), &data); err != nil {
			log.Printf("Failed to parse disruption data for %s: %v", key, err)
			continue
		}

		m.configCache[key] = &data.Config
	}
}

// syncConfigValues syncs key/value configuration from etcd (config/ prefix)
func (m *Manager) syncConfigValues() {
	if m.etcdClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := m.etcdClient.Get(ctx, "config/", clientv3.WithPrefix())
	if err != nil {
		log.Printf("Failed to sync config cache: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		m.configValues[key] = string(kv.Value)
	}
}

// watchConfigEntries watches for disruption configuration changes
func (m *Manager) watchConfigEntries() {
	if m.etcdClient == nil {
		return
	}

	watchChan := m.etcdClient.Watch(m.ctx, "config/", clientv3.WithPrefix())

	for {
		select {
		case <-m.ctx.Done():
			return
		case wresp := <-watchChan:
			for _, ev := range wresp.Events {
				key := string(ev.Kv.Key)

				m.mu.Lock()
				if ev.Type == clientv3.EventTypeDelete {
					delete(m.configCache, key)
				} else {
					var data DisruptionData
					if err := json.Unmarshal(ev.Kv.Value, &data); err != nil {
						log.Printf("Failed to parse disruption data for %s: %v", key, err)
						m.mu.Unlock()
						continue
					}
					m.configCache[key] = &data.Config
				}
				m.mu.Unlock()
			}
		}
	}
}

// watchConfigValues watches for simple config value changes
func (m *Manager) watchConfigValues() {
	if m.etcdClient == nil {
		return
	}

	watchChan := m.etcdClient.Watch(m.ctx, "config/", clientv3.WithPrefix())

	for {
		select {
		case <-m.ctx.Done():
			return
		case wresp := <-watchChan:
			for _, ev := range wresp.Events {
				key := string(ev.Kv.Key)

				m.mu.Lock()
				if ev.Type == clientv3.EventTypeDelete {
					delete(m.configValues, key)
				} else {
					m.configValues[key] = string(ev.Kv.Value)
				}
				m.mu.Unlock()
			}
		}
	}
}

// getConfigEntry gets a disruption configuration
func (m *Manager) getConfigEntry(name string) *DisruptionConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := "config/" + name
	if config, ok := m.configCache[key]; ok {
		return config
	}

	return nil
}

// GetConfig returns a simple config value for the given key (from config/<key> in etcd)
func (m *Manager) GetConfig(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := "config/" + name
	val, ok := m.configValues[key]
	return val, ok
}

// IsEnabled checks if a disruption is enabled
func (m *Manager) IsEnabled(name string) bool {
	config := m.getConfigEntry(name)
	return config != nil && config.Enabled
}

// GetDelayMs gets the delay in milliseconds for a disruption
func (m *Manager) GetDelayMs(name string) int {
	config := m.getConfigEntry(name)
	if config != nil {
		return config.DelayMs
	}
	return 0
}

// GetErrorRate gets the error rate for a disruption (0.0 to 1.0)
func (m *Manager) GetErrorRate(name string) float64 {
	config := m.getConfigEntry(name)
	if config != nil {
		return config.ErrorRate
	}
	return 0.0
}

// ApplyDelay applies a delay if the disruption is enabled
func (m *Manager) ApplyDelay(name string) {
	if m.IsEnabled(name) {
		delay := m.GetDelayMs(name)
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}
}

// ShouldFail checks if a request should fail based on error rate
func (m *Manager) ShouldFail(name string) bool {
	if !m.IsEnabled(name) {
		return false
	}

	errorRate := m.GetErrorRate(name)
	if errorRate <= 0 {
		return false
	}

	// Simple random check (in production, use a better random source)
	return time.Now().UnixNano()%100 < int64(errorRate*100)
}
