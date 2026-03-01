// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package httpimpl contains dogstatsd http server implementation
package httpimpl

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type server struct {
	config   config.Component
	log      log.Component
	tagger   tagger.Component
	hostname hostname.Component
	out      serializer

	http  *http.Server
	http2 *http2.Server
}

type serializer interface {
	SendIterableSeries(metrics.SerieSource) error
	SendSketch(metrics.SketchesSource) error
}

func (s *server) start(ctx context.Context) error {
	if !s.config.GetBool("dogstatsd_experimental_http.enabled") {
		s.log.Debug("dogstatsd http server disabled")
		return nil
	}

	hostname, err := s.hostname.Get(ctx)
	if err != nil {
		return fmt.Errorf("error fetching hostname: %w", err)
	}

	base := handlerBase{
		log:      s.log,
		tagger:   s.tagger,
		hostname: hostname,
		out:      s.out,
	}

	mux := &http.ServeMux{}
	mux.Handle("POST /series", &seriesHandler{base})

	s.http2 = &http2.Server{}
	s.http = &http.Server{
		Handler: h2c.NewHandler(mux, s.http2),
	}
	if err := http2.ConfigureServer(s.http, s.http2); err != nil {
		return fmt.Errorf("failed to configure http2 server: %w", err)
	}

	addr := s.config.GetString("dogstatsd_experimental_http.listen_address")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create dogstatsd http server on %q: %w", addr, err)
	}

	s.log.Debugf("starting dogstatsd http server on %q", addr)

	go func() {
		err := s.http.Serve(listener)
		if err == http.ErrServerClosed {
			s.log.Debugf("dogstatsd http server stopped normally")
		} else {
			s.log.Errorf("dogstatsd http server stopped with error: %v", err)
		}
	}()

	return nil
}

func (s *server) stop(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
