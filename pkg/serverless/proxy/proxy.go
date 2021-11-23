// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build serverlessexperimental

package proxy

import (
	"io"
	"net"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Start starts the proxy
// This proxy allows us to inspect traffic from/to the AWS Lambda Runtime API
func Start() {
	if strings.ToLower(os.Getenv("DD_EXPERIMENTAL_ENABLE_PROXY")) == "true" {
		log.Debug("the experimental proxy feature is enabled")
		setup()
	}
}

func setup() {
	originalRuntimeAPIAdress := "127.0.0.1:9001" // todo: fix this hardcoded value
	reRoutedRuntimeAPIAdress := "127.0.0.1:9000" // todo: fix this hardcoded value
	network := "tcp"
	listener, err := net.Listen(network, reRoutedRuntimeAPIAdress)
	if err != nil {
		log.Error("could not start the proxy")
	} else {
		for {
			proxyConnexion, err := listener.Accept()
			if err != nil {
				log.Error("could not accept the connection", err)
			} else {
				go handleRequest(network, originalRuntimeAPIAdress, proxyConnexion)
			}
		}
	}
}

func handleRequest(network string, originalRuntimeAPIAdress string, proxyConnexion net.Conn) {
	defer proxyConnexion.Close()
	originalConnexion, err := net.Dial(network, originalRuntimeAPIAdress)
	if err != nil {
		log.Error("error dialing remote addr", err)
		return
	}
	defer originalConnexion.Close()
	closeChannel := make(chan struct{}, 2)
	go inspectAndForwardRequest(closeChannel, originalConnexion, proxyConnexion)
	go inspectAndForwardRequest(closeChannel, proxyConnexion, originalConnexion)
	<-closeChannel
}

func inspectAndForwardRequest(closeChannel chan struct{}, dst io.Writer, src io.Reader) {
	// todo: parse correctly headers/body instead of logging it to stdout
	io.Copy(os.Stdout, io.TeeReader(src, dst))
	closeChannel <- struct{}{}
}
