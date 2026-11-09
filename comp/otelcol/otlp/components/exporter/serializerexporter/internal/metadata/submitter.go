// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metadata submits metric descriptions and units to the Datadog
// metric metadata API: https://docs.datadoghq.com/api/latest/metrics/#edit-metric-metadata
package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const (
	// rateLimitRequests per rateLimitInterval cap the requests to the API, retries included.
	rateLimitRequests = 1000
	rateLimitInterval = 20 * time.Second
	// queueSize is the maximum number of metadata waiting to be sent.
	queueSize = 1000
	// cacheSize is the number of metrics whose last submitted metadata the cache remembers.
	cacheSize = 1 << 14
)

// Metadata is the metadata of a metric.
type Metadata struct {
	Name        string
	Description string
	Unit        string
}

// request is the body of a metric metadata update.
// The API clears omitted fields, so the unit must always be sent alongside the description.
type request struct {
	Description string `json:"description"`
	Unit        string `json:"unit,omitempty"`
	PerUnit     string `json:"per_unit,omitempty"`
}

// Submitter sends metric metadata to the Datadog API from a single worker.
// It uses a cache to reduce the rate at which descriptions are submitted.
type Submitter struct {
	set       component.TelemetrySettings
	clientCfg confighttp.ClientConfig
	endpoint  string
	apiKey    string
	appKey    string

	mu sync.Mutex
	// pending holds the metadata last queued for each metric that is waiting to be sent.
	pending map[string]Metadata
	// sent holds the metadata last sent for each metric, including metadata the API rejected.
	sent  *simplelru.LRU[string, Metadata]
	queue chan Metadata

	limiter *rate.Limiter
	backOff backoff.BackOff

	cancel context.CancelFunc
	done   chan struct{}
}

// NewSubmitter creates a [Submitter].
func NewSubmitter(set component.TelemetrySettings, clientCfg confighttp.ClientConfig, endpoint, apiKey, appKey string) (*Submitter, error) {
	sent, err := simplelru.NewLRU[string, Metadata](cacheSize, nil)
	if err != nil {
		return nil, err
	}
	return &Submitter{
		set:       set,
		clientCfg: clientCfg,
		endpoint:  endpoint,
		apiKey:    apiKey,
		appKey:    appKey,
		pending:   make(map[string]Metadata),
		sent:      sent,
		queue:     make(chan Metadata, queueSize),
		limiter:   rate.NewLimiter(rate.Every(rateLimitInterval/rateLimitRequests), 1),
		backOff:   backoff.NewExponentialBackOff(),
	}, nil
}

// Submit queues the metadata in ms that differs from the metadata last queued or sent for the same metric.
// It never blocks: it drops metadata if the queue is full.
func (s *Submitter) Submit(ms ...Metadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var dropped int
	for _, m := range ms {
		last, ok := s.pending[m.Name]
		if !ok {
			last, ok = s.sent.Get(m.Name)
		}
		if ok && last == m {
			continue
		}
		select {
		case s.queue <- m:
			s.pending[m.Name] = m
		default:
			dropped++
		}
	}
	if dropped > 0 {
		s.set.Logger.Warn("Metric metadata queue is full, dropping metadata", zap.Int("dropped", dropped))
	}
}

// Start starts the worker.
func (s *Submitter) Start(ctx context.Context, host component.Host) error {
	client, err := s.clientCfg.ToClient(ctx, host.GetExtensions(), s.set)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.run(runCtx, client)
	}()
	return nil
}

// Shutdown stops the worker. Queued metadata is dropped.
func (s *Submitter) Shutdown(ctx context.Context) error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Submitter) run(ctx context.Context, client *http.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-s.queue:
			err := s.send(ctx, client, m)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				s.set.Logger.Warn("Failed to submit metric metadata", zap.String("metric", m.Name), zap.Error(err))
			}
			s.finish(m, err)
		}
	}
}

// finish records the outcome of sending m.
// Metadata that failed with a temporary error is forgotten, so that a later Submit retries it.
func (s *Submitter) finish(m Metadata, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[m.Name] == m {
		delete(s.pending, m.Name)
	}
	if err == nil || errors.Is(err, backoff.ErrPermanent) {
		s.sent.Add(m.Name, m)
	}
}

// send sends m, retrying with exponential backoff on network errors, 429 and 5xx responses.
func (s *Submitter) send(ctx context.Context, client *http.Client, m Metadata) error {
	unit, perUnit, _ := strings.Cut(m.Unit, "/")
	body, err := json.Marshal(request{Description: m.Description, Unit: unit, PerUnit: perUnit})
	if err != nil {
		return err
	}
	endpoint := s.endpoint + "/api/v1/metrics/" + url.PathEscape(m.Name)
	_, err = backoff.Retry(ctx, func() (struct{}, error) {
		if err := s.limiter.Wait(ctx); err != nil {
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, s.put(ctx, client, endpoint, body)
	}, backoff.WithBackOff(s.backOff))
	return err
}

func (s *Submitter) put(ctx context.Context, client *http.Client, endpoint string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return backoff.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", s.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", s.appKey)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	switch {
	case resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("%s: %s", resp.Status, respBody)
	default:
		return backoff.Permanent(fmt.Errorf("%s: %s", resp.Status, respBody))
	}
}
