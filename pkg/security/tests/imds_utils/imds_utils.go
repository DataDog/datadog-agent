// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package imdsutils holds utils related to the IMDS tests
package imdsutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/vishvananda/netlink"
)

const (
	// IMDSSecurityCredentialsURL is the URL used by the IMDS tests
	IMDSSecurityCredentialsURL = "/latest/meta-data/iam/security-credentials/test"
	// AWSSecurityCredentialsAccessKeyIDTestValue is the AccessKeyID used by the IMDS tests
	AWSSecurityCredentialsAccessKeyIDTestValue = "ASIAIOSFODNN7EXAMPLE"
	// AWSSecurityCredentialsTypeTestValue is the AWS Credentials Type used by the IMDS tests
	AWSSecurityCredentialsTypeTestValue = "AWS-HMAC"
	// AWSSecurityCredentialsCodeTestValue is the AWS Credentials Code used by the IMDS tests
	AWSSecurityCredentialsCodeTestValue = "Success"
	// AWSSecurityCredentialsLastUpdatedTestValue is the AWS Credentials LastUpdated value used by the IMDS tests
	AWSSecurityCredentialsLastUpdatedTestValue = "2012-04-26T16:39:16Z"
	// AWSSecurityCredentialsExpirationTestValue is the AWS Credentials Expiration value used by the IMDS tests
	AWSSecurityCredentialsExpirationTestValue = "2324-05-01T12:00:00Z"
	// AWSIMDSServerTestValue is the IMDS Server used by the IMDS tests
	AWSIMDSServerTestValue = "EC2ws"
	// CSMDummyInterface is the Dummy interface name used by the IMDS tests
	CSMDummyInterface = "dummy_csm"
	// IMDSTestServerIP is the IMDS server IP used by the IMDS tests
	IMDSTestServerIP = "169.254.169.254"
	// IMDSTestServerPort is the IMDS server port used by the IMDS tests
	IMDSTestServerPort = 8080
)

// CreateDummyInterface creates a dummy interface and attaches it to the provided IP
func CreateDummyInterface(ip string, name string) (*netlink.Dummy, error) {
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
	}

	// delete existing dummy interface
	_ = netlink.LinkDel(dummy)

	// Add the dummy interface
	if err := netlink.LinkAdd(dummy); err != nil {
		return nil, fmt.Errorf("failed to create dummy interface %s: %v", name, err)
	}

	// attach the IMDS IP to the dummy interface
	addr := &netlink.Addr{IPNet: &net.IPNet{
		IP:   net.ParseIP(ip),
		Mask: net.CIDRMask(24, 32),
	}}
	if err := netlink.AddrAdd(dummy, addr); err != nil {
		return nil, fmt.Errorf("failed to attach IMDS IP to %s: %v", name, err)
	}

	// set dummy interface up
	if err := netlink.LinkSetUp(dummy); err != nil {
		return nil, fmt.Errorf("failed to set %s up: %v", name, err)
	}

	return dummy, nil
}

// RemoveDummyInterface removes the provided dummy interface
func RemoveDummyInterface(link *netlink.Dummy) error {
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete %s: %v", link.Name, err)
	}
	return nil
}

// CreateIMDSServer creates a fake IMDS server
func CreateIMDSServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(IMDSSecurityCredentialsURL, func(w http.ResponseWriter, r *http.Request) {
		// Define your custom JSON data
		data := map[string]interface{}{
			"AccessKeyId":     AWSSecurityCredentialsAccessKeyIDTestValue,
			"SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"Token":           "FQoDYXdzEL3//////////wEaDNzv2bUBTBHZWpL6iWLZAyaMGJnKlXNoDMHEFvgF7OeM8Cxz69tJNYk8GvIYVpOInuLeMfkplcQ2NeO6xVBa0gB0T6f/5AWhdV5SdpDoyCtYIvMDIG2a7DJVpuZ7d7vylWfFzohpV5Y7C7gWQuIdH/qc3kkWz3hkhCc+5iKmB+QxG30BPoCGOYYzN+QkGiPjZzXfTFdAfX/+/VY6DiVnl8MGB2TFdSRpF7GbcuhKhrkAnJ7UlNnnYVVtFfO9TlBMSbJH55CFv0FDACG0nHsIExSkD1Vau/nHeFLv6xMT+WAtI05/RtZZC8JfKJi4ST+TqB5ftc2qVLMy9AlWzrr2uN6R1fSeOESO7rf2Koq3m31CR8KKjYMXdo/38dNwxawf+3z8U8HhBc5sYXfcWHH7m0Q7DqQ3pPzMKFL/QPxTssP9lwJr2L7vqJxqN4Tcjq9+8pg=",
			"Expiration":      AWSSecurityCredentialsExpirationTestValue,
			"Code":            AWSSecurityCredentialsCodeTestValue,
			"LastUpdated":     AWSSecurityCredentialsLastUpdatedTestValue,
			"Type":            AWSSecurityCredentialsTypeTestValue,
		}

		// Convert data to JSON
		response, err := json.Marshal(data)
		if err != nil {
			http.Error(w, "couldn't marshal data", http.StatusInternalServerError)
			return
		}

		// Set Content-Type header to application/json
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Server", AWSIMDSServerTestValue)

		// Write JSON response
		w.Write(response)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// StopIMDSserver stops the provided server gracefully
func StopIMDSserver(server *http.Server) error {
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown server: %v", err)
	}
	return nil
}
