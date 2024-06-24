// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a simple client for the gotls_server.
package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func main() {
	flag.Parse()
	args := flag.Args()

	file, err := os.CreateTemp("/tmp", "gotls_server-*.log")
	if err != nil {
		log.Fatalf("could not create log file: %s", err)
	}
	log.Default().SetOutput(file)

	if len(args) < 1 {
		log.Fatalf("usage: gotls_server <server_addr>")
	}

	addr := os.Args[1]

	crtPath, keyPath, err := testutil.GetCertsPaths()
	if err != nil {
		log.Fatalf("Could not get certificates")
	}

	handler := func(w http.ResponseWriter, req *http.Request) {
		statusCode := testutil.StatusFromPath(req.URL.Path)
		if statusCode == 0 {
			log.Printf("wrong request format %s", req.URL.Path)
		} else {
			w.WriteHeader(int(statusCode))
		}

		defer req.Body.Close()
		_, err := io.Copy(w, req.Body)
		if err != nil {
			log.Printf("could not write response body: %s", err)
		}
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      http.HandlerFunc(handler),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}

	// Disabling http2
	srv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	srv.SetKeepAlivesEnabled(true)

	listenFn := func() error {
		ln, err := net.Listen("tcp", srv.Addr)
		if err == nil {
			_ = srv.ServeTLS(ln, crtPath, keyPath)
		}
		return err
	}

	if err := listenFn(); err != nil {
		log.Fatalf("server listen: %s", err)
	}
}
