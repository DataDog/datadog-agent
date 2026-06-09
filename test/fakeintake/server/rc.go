// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	yaml "gopkg.in/yaml.v3"

	core "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/server/rcstore"
)

// rcServerState holds the in-memory Remote Config repo for fakeintake.
//
// Fakeintake serves TUF-signed Remote Config payloads when --remoteconfig is
// enabled. Tests push configs via /fakeintake/rc/config; the agent polls
// /api/v0.1/configurations and receives the configs wrapped in TUF metadata.
type rcServerState struct {
	mu sync.Mutex

	enabled  bool
	orgUUID  string
	configs  map[string]rcstore.Config
	version  uint64
	polls    uint64
	lastPoll time.Time

	signing  ed25519.PrivateKey
	keyID    string
	rootJSON []byte

	keyPath          string
	keyData          string // hex-encoded seed; takes precedence over keyPath when non-empty
	initialStatePath string
}

func (s *rcServerState) configKey(c rcstore.Config) string {
	return fmt.Sprintf("%s/%s/%s/%s", c.OrgID, c.Product, c.ConfigID, c.ConfigName)
}

// addConfig stores or replaces a config and bumps the version counter.
func (s *rcServerState) addConfig(c rcstore.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[s.configKey(c)] = c
	s.version++
}

func (s *rcServerState) deleteConfig(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.configs[key]; !ok {
		return false
	}
	delete(s.configs, key)
	s.version++
	return true
}

func (s *rcServerState) snapshot() []rcstore.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]rcstore.Config, 0, len(s.configs))
	for _, c := range s.configs {
		out = append(out, c)
	}
	return out
}

func (s *rcServerState) configsForProducts(products []string) []rcstore.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := make(map[string]struct{}, len(products))
	for _, p := range products {
		wanted[p] = struct{}{}
	}
	out := make([]rcstore.Config, 0, len(s.configs))
	for _, c := range s.configs {
		if _, ok := wanted[c.Product]; ok {
			out = append(out, c)
		}
	}
	return out
}

func (s *rcServerState) recordPoll(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polls++
	s.lastPoll = now
}

// --- Options ---

// WithRemoteConfig enables fakeintake's Remote Config endpoints. The server
// loads or generates a persistent ed25519 signing key under
// ~/.fakeintake/signing.key and prints the resulting datadog.yaml snippet.
//
// orgUUID and apiKey are advisory: orgUUID is returned by /api/v0.1/org;
// apiKey is logged on mismatch but never enforced.
// Pass empty strings for defaults ("42" / "test-api-key").
func WithRemoteConfig(orgUUID string) Option {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to enable Remote Config.")
			return
		}
		if orgUUID == "" {
			orgUUID = "42"
		}
		fi.rc = &rcServerState{
			enabled: true,
			orgUUID: orgUUID,
			configs: make(map[string]rcstore.Config),
			version: 1,
		}
	}
}

// WithRemoteConfigKeyPath overrides the on-disk path for the ed25519 signing
// key seed. Empty falls back to ~/.fakeintake/signing.key.
func WithRemoteConfigKeyPath(path string) Option {
	return func(fi *Server) {
		if fi.rc == nil {
			return
		}
		fi.rc.keyPath = path
	}
}

// WithRemoteConfigKeyData supplies the ed25519 signing key as a hex-encoded
// 32-byte seed string. When set, the key is never written to disk and
// WithRemoteConfigKeyPath is ignored. Use this for ephemeral environments
// (e.g. ECS Fargate) where a fixed, pre-known key is required so the agent's
// config_root/director_root can be set at provisioning time.
func WithRemoteConfigKeyData(hexSeed string) Option {
	return func(fi *Server) {
		if fi.rc == nil {
			return
		}
		fi.rc.keyData = hexSeed
	}
}

// WithRemoteConfigVersion seeds the version counter (default 1). Used to keep
// the agent's remote-config.db in sync across restarts.
func WithRemoteConfigVersion(v uint64) Option {
	return func(fi *Server) {
		if fi.rc == nil {
			return
		}
		if v == 0 {
			v = 1
		}
		fi.rc.version = v
	}
}

// rcStateFile is the YAML schema accepted by --rc-state for preloading a
// single config at startup.
type rcStateFile struct {
	Product    string      `yaml:"product"`
	ConfigID   string      `yaml:"config_id"`
	ConfigName string      `yaml:"config_name"`
	Data       interface{} `yaml:"data"`
}

// WithRemoteConfigInitialState preloads one config from a YAML file.
func WithRemoteConfigInitialState(path string) Option {
	return func(fi *Server) {
		if fi.rc == nil {
			return
		}
		fi.rc.initialStatePath = path
	}
}

// initRC must be called once after all RC options have been applied. It loads
// the signing key, builds root.json, and applies any preloaded state.
func (fi *Server) initRC() error {
	rc := fi.rc
	if rc == nil || !rc.enabled {
		return nil
	}

	var (
		priv ed25519.PrivateKey
		err  error
	)
	if rc.keyData != "" {
		priv, err = rcstore.KeyFromHexSeed(rc.keyData)
		if err != nil {
			return fmt.Errorf("rc signing key (from --rc-key-data): %w", err)
		}
		log.Println("Remote Config: loaded signing key from --rc-key-data")
	} else {
		var generated bool
		priv, generated, err = rcstore.LoadOrCreateSigningKey(rc.keyPath)
		if err != nil {
			return fmt.Errorf("rc signing key: %w", err)
		}
		if generated {
			log.Println("Remote Config: generated new signing key — agent's remote-config.db must be flushed")
		}
	}
	rc.signing = priv

	pubHex := rcstore.PublicKeyHex(priv)
	keyID, err := rcstore.ComputeKeyID(pubHex)
	if err != nil {
		return fmt.Errorf("rc key id: %w", err)
	}
	rc.keyID = keyID

	root, err := rcstore.BuildRootJSON(priv, keyID, pubHex)
	if err != nil {
		return fmt.Errorf("rc root.json: %w", err)
	}
	rc.rootJSON = root

	log.Printf("Remote Config: keyid=%s pubkey=%s", keyID, pubHex)
	log.Printf("Remote Config: paste into datadog.yaml:\n  remote_configuration.config_root: '%s'\n  remote_configuration.director_root: '%s'", root, root)

	if rc.initialStatePath != "" {
		if err := fi.loadRCInitialState(rc.initialStatePath); err != nil {
			return fmt.Errorf("rc initial state: %w", err)
		}
	}
	return nil
}

func (fi *Server) loadRCInitialState(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f rcStateFile
	if err := yaml.Unmarshal(body, &f); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	dataBytes, err := json.Marshal(f.Data)
	if err != nil {
		return fmt.Errorf("re-marshal data: %w", err)
	}
	fi.rc.addConfig(rcstore.Config{
		OrgID:      fi.rc.orgUUID,
		Product:    f.Product,
		ConfigID:   f.ConfigID,
		ConfigName: f.ConfigName,
		Data:       dataBytes,
	})
	log.Printf("Remote Config: loaded initial state %s/%s/%s", f.Product, f.ConfigID, f.ConfigName)
	return nil
}

// --- Agent-facing handlers ---

func (fi *Server) handleRCConfigurations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rc := fi.rc
	if rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req := &core.LatestConfigsRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		http.Error(w, "decode request: "+err.Error(), http.StatusBadRequest)
		return
	}
	rc.recordPoll(fi.clock.Now().UTC())

	products := append(req.GetProducts(), req.GetNewProducts()...)
	cfgs := rc.configsForProducts(products)
	if len(cfgs) == 0 {
		log.Printf("Remote Config: no configs for products %v", products)
		http.Error(w, "no configurations available", http.StatusNotFound)
		return
	}

	rc.mu.Lock()
	version := rc.version
	rc.mu.Unlock()

	metas, err := rcstore.GenerateTUFMetas(cfgs, rc.signing, rc.keyID, rc.rootJSON, version)
	if err != nil {
		http.Error(w, "build metas: "+err.Error(), http.StatusInternalServerError)
		return
	}

	files := make([]*core.File, 0, len(cfgs))
	for _, c := range cfgs {
		files = append(files, &core.File{Path: c.Path(), Raw: c.Data})
	}
	resp := &core.LatestConfigsResponse{
		ConfigMetas: &core.ConfigMetas{
			Roots:      []*core.TopMeta{{Version: 1, Raw: metas.Root}},
			Timestamp:  &core.TopMeta{Version: version, Raw: metas.Timestamp},
			Snapshot:   &core.TopMeta{Version: version, Raw: metas.Snapshot},
			TopTargets: &core.TopMeta{Version: version, Raw: metas.Targets},
		},
		DirectorMetas: &core.DirectorMetas{
			Roots:     []*core.TopMeta{{Version: 1, Raw: metas.Root}},
			Timestamp: &core.TopMeta{Version: version, Raw: metas.Timestamp},
			Snapshot:  &core.TopMeta{Version: version, Raw: metas.Snapshot},
			Targets:   &core.TopMeta{Version: version, Raw: metas.Targets},
		},
		TargetFiles: files,
	}
	out, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, "encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(out)
	log.Printf("Remote Config: served %d files (version %d)", len(cfgs), version)
}

func (fi *Server) handleRCOrg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	out, err := proto.Marshal(&core.OrgDataResponse{Uuid: fi.rc.orgUUID})
	if err != nil {
		http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(out)
}

func (fi *Server) handleRCStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	out, err := proto.Marshal(&core.OrgStatusResponse{Enabled: true, Authorized: true})
	if err != nil {
		http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(out)
}

// --- Control handlers (called by tests / fakeintakectl) ---

func (fi *Server) handleRCAddConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	var req api.RCAddConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Product == "" || req.ConfigID == "" || req.ConfigName == "" {
		http.Error(w, "product, config_id, config_name are required", http.StatusBadRequest)
		return
	}
	if req.OrgID == "" {
		req.OrgID = fi.rc.orgUUID
	}
	// Re-marshal data so it's stable bytes regardless of how the caller
	// formatted it. If Data is empty, use {} so target hashes are valid.
	dataBytes := []byte(req.Data)
	if len(dataBytes) == 0 {
		dataBytes = []byte("{}")
	} else {
		var any interface{}
		if err := json.Unmarshal(dataBytes, &any); err != nil {
			http.Error(w, "data must be valid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		out, err := json.Marshal(any)
		if err != nil {
			http.Error(w, "re-marshal data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dataBytes = out
	}
	fi.rc.addConfig(rcstore.Config{
		OrgID:      req.OrgID,
		Product:    req.Product,
		ConfigID:   req.ConfigID,
		ConfigName: req.ConfigName,
		Data:       dataBytes,
	})
	w.WriteHeader(http.StatusCreated)
}

func (fi *Server) handleRCListConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	cfgs := fi.rc.snapshot()
	out := make([]api.RCConfig, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, api.RCConfig{
			OrgID: c.OrgID, Product: c.Product, ConfigID: c.ConfigID, ConfigName: c.ConfigName, Data: c.Data,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (fi *Server) handleRCDeleteConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/fakeintake/rc/config/")
	if key == "" {
		http.Error(w, "missing config key in path", http.StatusBadRequest)
		return
	}
	if !fi.rc.deleteConfig(key) {
		http.Error(w, "config not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (fi *Server) handleRCStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if fi.rc == nil {
		http.Error(w, "remote config not enabled", http.StatusNotFound)
		return
	}
	fi.rc.mu.Lock()
	stats := api.RCStats{
		Polls:        fi.rc.polls,
		LastPoll:     fi.rc.lastPoll,
		Version:      fi.rc.version,
		ConfigsCount: len(fi.rc.configs),
		KeyID:        fi.rc.keyID,
		PublicKey:    rcstore.PublicKeyHex(fi.rc.signing),
		RootJSON:     string(fi.rc.rootJSON),
	}
	fi.rc.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
