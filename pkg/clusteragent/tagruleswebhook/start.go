// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagruleswebhook

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"k8s.io/client-go/dynamic"

	model "github.com/DataDog/datadog-agent/pkg/config/model"
)

// DefaultPort is the webhook serving port when none is configured.
const DefaultPort = 9443

// Options configures the webhook server.
type Options struct {
	// Enabled gates the server; read from cluster_agent.tag_rules.webhook.enabled.
	Enabled bool
	// Port is the serving port; read from cluster_agent.tag_rules.webhook.port.
	Port int
	// CertFile and KeyFile are an optional serving certificate pair. When
	// empty, a self-signed pair is generated at startup and its CA bundle
	// is logged (base64) for the ValidatingWebhookConfiguration caBundle.
	CertFile string
	KeyFile  string
	// Client is the dynamic client used to list existing TagRules for
	// tag-key uniqueness checks.
	Client dynamic.Interface
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// OptionsFromConfig reads the cluster_agent.tag_rules.webhook.* settings.
// The cert pair stays empty here (self-signed mode) unless the caller sets it.
func OptionsFromConfig(cfg model.Reader, client dynamic.Interface) Options {
	port := cfg.GetInt("cluster_agent.tag_rules.webhook.port")
	if port <= 0 {
		port = DefaultPort
	}
	return Options{
		Enabled: cfg.GetBool("cluster_agent.tag_rules.webhook.enabled"),
		Port:    port,
		Client:  client,
		Logger:  slog.Default(),
	}
}

// Start runs the validating webhook server until the context is cancelled.
// It is a no-op when disabled. It blocks on serving; callers run it in a
// goroutine.
func Start(ctx context.Context, opts Options) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if !opts.Enabled {
		logger.Info("tag-rule webhook disabled")
		return nil
	}

	handler, err := NewWebhook(&dynamicTagKeyLister{client: opts.Client}, logger)
	if err != nil {
		return fmt.Errorf("building tag-rule webhook: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle(ValidatePath, handler)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var cert tls.Certificate
	if opts.CertFile != "" && opts.KeyFile != "" {
		if cert, err = tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile); err != nil {
			return fmt.Errorf("loading webhook certificate: %w", err)
		}
	} else {
		certPEM, keyPEM, caBundle, err := selfSignedCert()
		if err != nil {
			return fmt.Errorf("generating webhook certificate: %w", err)
		}
		if cert, err = tls.X509KeyPair(certPEM, keyPEM); err != nil {
			return fmt.Errorf("loading generated webhook certificate: %w", err)
		}
		logger.Info("tag-rule webhook serving with a self-signed certificate; set the ValidatingWebhookConfiguration caBundle to", "ca_bundle", caBundle)
	}
	server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServeTLS("", "") }()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("tag-rule webhook shutdown", "error", err)
		}
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("tag-rule webhook server: %w", err)
	case <-ctx.Done():
		return nil
	}
}
