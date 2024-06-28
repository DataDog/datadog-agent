// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryotelimpl

import (
	"embed"
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

//go:embed dummy_data
var dummyFS embed.FS

const freshnessTO = 30 * time.Second
const httpTO = 5 * time.Second

type configGetter func(*url.URL) (otelMetadata, error)

type freshConfig struct {
	stale       bool
	conf        otelMetadata
	collectFunc configGetter
	source      *url.URL
	t           *time.Timer
	mu          sync.RWMutex
}

func newFreshConfig(source string, f configGetter) (*freshConfig, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}

	return &freshConfig{
		stale:       true,
		source:      u,
		collectFunc: f,
	}, nil
}

func (f *freshConfig) isStale() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.stale
}

func (f *freshConfig) getConfig() (otelMetadata, error) {
	if !f.isStale() {
		return f.conf, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	c, err := f.collectFunc(f.source)
	if err != nil {
		return nil, err
	}

	f.conf = c
	f.stale = false

	// mark as stale after TO period
	f.t = time.AfterFunc(freshnessTO, func() {
		f.mu.Lock()
		defer f.mu.Unlock()

		f.stale = true
	})

	return f.conf, nil
}

func scrub(s string) string {
	// Errors come from internal use of a Reader interface. Since we are reading from a buffer, no errors
	// are possible.
	scrubString, _ := scrubber.ScrubString(s)
	return scrubString
}

func copyAndScrub(o otelMetadata) otelMetadata {
	data := make(otelMetadata)
	for k, v := range o {
		if s, ok := v.(string); ok {
			data[k] = scrub(s)
		} else {
			data[k] = v
		}
	}

	return data
}
