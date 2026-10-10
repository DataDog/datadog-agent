// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command localdog runs a local Datadog backend: point a Datadog Agent at it to receive
// metrics, logs and traces on your laptop, and explore them with the localdog web app.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/test/localdog/server"
	"github.com/DataDog/datadog-agent/test/localdog/store"
)

func main() {
	opts := store.DefaultOptions()
	addr := flag.String("addr", "127.0.0.1:8282", "address to listen on (use 0.0.0.0:8282 to accept agents from other hosts/containers)")
	uiDir := flag.String("ui-dir", "", "optional directory with the built localdog web app (web-ui static-apps/localdog/dist), served at /")
	flag.IntVar(&opts.MaxLogs, "max-logs", opts.MaxLogs, "maximum number of logs kept in memory")
	flag.IntVar(&opts.MaxSpans, "max-spans", opts.MaxSpans, "maximum number of spans kept in memory")
	flag.IntVar(&opts.MaxPointsPerSeries, "max-points", opts.MaxPointsPerSeries, "maximum number of points kept per metric series")
	flag.DurationVar(&opts.Retention, "retention", opts.Retention, "how long data is kept")
	dataDir := flag.String("data-dir", defaultDataDir(), "directory where received data is persisted across restarts (empty to keep data in memory only)")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *version {
		fmt.Println(server.Version)
		return
	}

	st := store.New(opts)
	snapshotPath := ""
	if *dataDir != "" {
		snapshotPath = filepath.Join(*dataDir, "snapshot.gob.gz")
		if err := st.LoadSnapshot(snapshotPath); err != nil {
			log.Printf("could not restore %s: %v", snapshotPath, err)
		} else {
			st.Prune()
			stats := st.Stats()
			log.Printf("restored %d logs, %d spans, %d metric series from %s", stats.Logs, stats.Spans, stats.MetricSeries, snapshotPath)
		}
	}
	save := func() {
		if snapshotPath == "" {
			return
		}
		if err := st.SaveSnapshot(snapshotPath); err != nil {
			log.Printf("could not save %s: %v", snapshotPath, err)
		}
	}
	handler := server.New(st, server.Options{UIDir: *uiDir})
	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				st.Prune()
				save()
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	url := "http://" + *addr
	log.Printf("localdog %s listening on %s", server.Version, url)
	log.Printf("point your Datadog Agent at it with:")
	log.Printf("  DD_DD_URL=%[1]s DD_APM_DD_URL=%[1]s DD_LOGS_CONFIG_LOGS_DD_URL=%[2]s DD_LOGS_CONFIG_LOGS_NO_SSL=true DD_LOGS_CONFIG_FORCE_USE_HTTP=true", url, *addr)
	if handler.HasUI() {
		log.Printf("web app: %s/", url)
	} else {
		log.Printf("no web app bundled: pass -ui-dir, or build with `dda inv localdog.build --with-ui`")
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	save()
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".localdog")
}
