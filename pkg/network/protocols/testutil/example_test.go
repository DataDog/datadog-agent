// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package testutil_test

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

func ExampleWithPCAP_plaintext() {
	t := &testing.T{}

	// Run tcpdump alongside the test and only save the resulting
	// PCAP if the test failed. We don't need the keylog writer
	// here so we discard it.
	_ = testutil.WithPCAP(t, "80", testutil.GetShortTestName("HTTP", "simple request"), false)

	client := &http.Client{}
	resp, err := client.Get("http://httpbin.org/status/200")
	if err != nil {
		t.Errorf("error while making request")
	}

	if err = resp.Body.Close(); err != nil {
		t.Errorf("error while closing request body")
	}
}

func ExampleWithPCAP_tls() {
	t := &testing.T{}

	// Run tcpdump alongside the test and always save the resulting PCAP.
	klw := testutil.WithPCAP(t, "443", testutil.GetShortTestName("HTTP", "simple request - TLS"), true)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				KeyLogWriter:       klw,
			},
		},
	}
	resp, err := client.Get("https://httpbin.org/status/200")
	if err != nil {
		t.Errorf("error while making request")
	}

	if err = resp.Body.Close(); err != nil {
		t.Errorf("error while closing request body")
	}
}
