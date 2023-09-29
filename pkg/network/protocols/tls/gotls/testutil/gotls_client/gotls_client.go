// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("usage: %s <server_addr> <number_of_requests>", os.Args[0])
	}

	serverAddr := os.Args[1]
	reqCount, err := strconv.Atoi(os.Args[2])
	if err != nil || reqCount < 0 {
		log.Fatalf("invalid value %q for number of request", os.Args[2])
	}

	client := http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	in := make([]byte, 1)
	_, err = io.ReadFull(os.Stdin, in)
	if err != nil {
		log.Fatalf("could not read from stdin: %s", err)
	}

	for i := 0; i < reqCount; i++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, http.StatusOK, i), nil)
		if err != nil {
			log.Fatalf("could not generate HTTP request: %s", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("could not do HTTPS request: %s", err)
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("could not read response body: %s", err)
		}

		resp.Body.Close()
	}

}
