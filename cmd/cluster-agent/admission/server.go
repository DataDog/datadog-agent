// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build kubeapiserver

package admission

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunServer creates and start a k8s admission webhook server
func RunServer(mainCtx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", mutateFunc)
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
		// TLSConfig: &tls.Config{
		// 	GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// 		// TODO
		// 		return &tls.Certificate{}, nil
		// 	},
		// },
	}
	go func() error {
		// return log.Error(server.ListenAndServeTLS("", ""))
		return log.Error(server.ListenAndServe())
	}()

	<-mainCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func mutateFunc(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "hello world\n")
}
