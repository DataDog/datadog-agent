// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package healthport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/status/health"
)

const defaultTimeout = time.Second

var server *http.Server

func Serve(ctx context.Context, port int) error {
	if port == 0 {
		return errors.New("port should be non-zero")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%v", port))
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           healthHandler{},
		ReadTimeout:       defaultTimeout,
		ReadHeaderTimeout: defaultTimeout,
		WriteTimeout:      defaultTimeout,
	}

	go srv.Serve(ln)
	go closeOnContext(ctx, srv)
	return nil
}

func closeOnContext(ctx context.Context, srv *http.Server) {
	<-ctx.Done()
	srv.Close() // srv will close the listener
}

type healthHandler struct{}

func (h healthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	health, err := health.GetStatusNonBlocking()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	var b strings.Builder
	b.WriteString("Agent health: ")

	if len(health.Unhealthy) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		b.WriteString("FAIL\n")
	} else {
		b.WriteString("PASS\n")
	}

	if len(health.Healthy) > 0 {
		b.WriteString("=== Healthy components ===\n")
		b.WriteString(strings.Join(health.Healthy, ", "))
		b.WriteString("\n")
	}
	if len(health.Unhealthy) > 0 {
		b.WriteString("=== Unhealthy components ===\n")
		b.WriteString(strings.Join(health.Unhealthy, ", "))
		b.WriteString("\n")
	}

	w.Write([]byte(b.String()))
}
