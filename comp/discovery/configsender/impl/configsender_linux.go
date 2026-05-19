// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package configsenderimpl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	configsender "github.com/DataDog/datadog-agent/comp/discovery/configsender/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
)

const (
	configFlag    = "discovery.config_files_sender.enabled"
	intakeURL     = "https://all-internal-intake-logs.staging.dog/v2/track/demoalpha/org/47653"
	pollInterval  = 30 * time.Second
	httpTimeout   = 10 * time.Second
	maxFileSize   = 256 * 1024
	configSource  = "app_native"
)

// Requires defines the dependencies for the configsender component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    pkgconfigmodel.Reader
	Hostname  hostnameinterface.Component
}

// Provides defines the output of the configsender component.
type Provides struct {
	Comp configsender.Component
}

// NewComponent wires the configsender component. When the feature flag is
// off this is a no-op — no goroutine is spawned, no socket is opened.
func NewComponent(reqs Requires) Provides {
	if !reqs.Config.GetBool(configFlag) {
		log.Infof("configsender disabled (set %s: true to enable)", configFlag)
		return Provides{Comp: &sender{}}
	}

	apiKey := strings.TrimSpace(reqs.Config.GetString("api_key"))
	s := &sender{
		hostname:  reqs.Hostname,
		apiKey:    apiKey,
		sysProbe:  sysprobeclient.GetCheckClient(),
		intake:    &http.Client{Timeout: httpTimeout},
		stopCh:    make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			log.Infof("configsender enabled, posting to %s", intakeURL)
			go s.run()
			return nil
		},
		OnStop: func(context.Context) error {
			close(s.stopCh)
			return nil
		},
	})

	return Provides{Comp: s}
}

type sender struct {
	hostname hostnameinterface.Component
	apiKey   string
	sysProbe *sysprobeclient.CheckClient
	intake   *http.Client
	seen     dedupSet
	stopCh   chan struct{}
}

func (s *sender) run() {
	s.tick() // first tick immediately so we don't wait 30s on startup
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			s.tick()
		}
	}
}

func (s *sender) tick() {
	pids, err := listProcPIDs()
	if err != nil {
		log.Debugf("configsender: listProcPIDs: %v", err)
		return
	}
	if len(pids) == 0 {
		return
	}
	resp, err := sysprobeclient.Post[model.ServicesResponse](
		s.sysProbe, "/services",
		core.Params{NewPids: pids},
		sysconfig.DiscoveryModule,
	)
	if err != nil {
		log.Debugf("configsender: fetch /services: %v", err)
		return
	}
	host, err := s.hostname.Get(context.Background())
	if err != nil || host == "" {
		log.Warnf("configsender: no hostname, skipping tick: %v", err)
		return
	}
	for _, svc := range resp.Services {
		integration := strings.ToLower(svc.GeneratedName)
		if integration == "" {
			continue
		}
		for _, path := range svc.ConfigFiles {
			s.process(host, integration, path)
		}
	}
}

func (s *sender) process(host, integration, path string) {
	ct := detectContentType(integration, path)
	if ct == "" {
		return
	}
	raw, err := readCapped(path, maxFileSize)
	if err != nil {
		log.Debugf("configsender: read %s: %v", path, err)
		return
	}
	if !utf8.Valid(raw) {
		log.Debugf("configsender: skip non-utf8 file %s", path)
		return
	}
	body, hash, err := buildEnvelope(host, integration, configSource, path, ct, raw)
	if err != nil {
		log.Warnf("configsender: build envelope for %s: %v", path, err)
		return
	}
	if !s.seen.addIfNew(host, integration, hash) {
		return
	}
	if err := s.post(body); err != nil {
		log.Warnf("configsender: post %s: %v", path, err)
		s.seen.forget(host, integration, hash)
		return
	}
	log.Infof("configsender: config sent host=%s integration=%s file=%s hash=%s", host, integration, path, hash[:12])
}

func (s *sender) post(body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, intakeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("DD-API-KEY", s.apiKey)
	}
	resp, err := s.intake.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, excerpt)
	}
	return nil
}

func readCapped(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

func listProcPIDs() ([]int32, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	pids := make([]int32, 0, 256)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pids = append(pids, int32(n))
	}
	return pids, nil
}
