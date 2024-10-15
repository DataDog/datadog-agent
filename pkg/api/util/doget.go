// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ShouldCloseConnection is an option to DoGet to indicate whether to close the underlying
// connection after reading the response
type ShouldCloseConnection int

const (
	// LeaveConnectionOpen keeps the underlying connection open after reading the request response
	LeaveConnectionOpen ShouldCloseConnection = iota
	// CloseConnection closes the underlying connection after reading the request response
	CloseConnection
)

// ReqOptions are options when making a request
type ReqOptions struct {
	Conn      ShouldCloseConnection
	Ctx       context.Context
	Authtoken string
}

// AddrResolver is a map that provides, for a given Agent domain name, a function to retrieve its real transport address (e.g., "core-cmd" -> "127.0.0.1:5001").
// The function can return either the address or an error.
type AddrResolver map[string]func() (string, error)

// The following constant values represent the Agent domain names
const (
	CoreCmd        = "core-cmd"        // CoreCmd is the core Agent command endpoint
	CoreIPC        = "core-ipc"        // CoreIPC is the core Agent configuration synchronisation endpoint
	CoreExpvar     = "core-expvar"     // CoreExpvar is the core Agent expvar endpoint
	TraceCmd       = "trace-cmd"       // TraceCmd is the trace Agent command endpoint
	TraceExpvar    = "trace-expvar"    // TraceExpvar is the trace Agent expvar endpoint
	SecurityCmd    = "security-cmd"    // SecurityCmd is the security Agent command endpoint
	SecurityExpvar = "security-expvar" // SecurityExpvar is the security Agent expvar endpoint
	ProcessCmd     = "process-agent"   // ProcessCmd is the process Agent command endpoint
	ProcessExpvar  = "process-expvar"  // ProcessExpvar is the process Agent expvar endpoint
	ClusterAgent   = "cluster-agent"   // ClusterAgent is the Cluster Agent command endpoint
)

type dialContext func(ctx context.Context, network string, addr string) (net.Conn, error)

var db = AddrResolver{
	CoreCmd: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("cmd_port")), nil
	},
	CoreIPC: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		port := config.GetInt("agent_ipc.port")
		if port <= 0 {
			return "", fmt.Errorf("agent_ipc.port cannot be <= 0")
		}

		return net.JoinHostPort(config.GetString("agent_ipc.host"), strconv.Itoa(port)), nil
	},
	CoreExpvar: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("expvar_port")), nil
	},

	TraceCmd: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("apm_config.debug.port")), nil
	},
	TraceExpvar: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("apm_config.debug.port")), nil
	},

	ProcessCmd: func() (string, error) {
		return pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	},
	ProcessExpvar: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("process_config.expvar_port")), nil
	},

	SecurityCmd: func() (string, error) {
		return pkgconfigsetup.GetSecurityAgentAPIAddressPort(pkgconfigsetup.Datadog())
	},
	SecurityExpvar: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("security_agent.expvar_port")), nil
	},

	ClusterAgent: func() (string, error) {
		config := pkgconfigsetup.Datadog()
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, config.GetString("cluster_agent.cmd_port")), nil
	},
}

// OverrideResolver allows you to upsert a getter in the shared [AddrResolver].
// This function is intended for testing purposes only.
func OverrideResolver(src, target string) {
	db[src] = func() (string, error) {
		return target, nil
	}
}

// ClientBuilder is a struct used to build an [*net/http.Client].
type ClientBuilder struct {
	tr      *http.Transport
	timeout time.Duration
}

// GetClient returns a ClientBuilder struct that lets you create an Agent-specific client.
// To get an [*net/http.Client] object from the return value, call the Build() function.
// To provide specific features to your client, call the related With...() functions.
//
// Note: The order in which the With functions are called does not affect the final configuration
//
// # Example usage
//
//	client := GetClient().WithNoVerify().WithResolver().Build()
//
// This example creates an HTTP client with no TLS verification and a custom resolver.
func GetClient() ClientBuilder {
	return ClientBuilder{
		tr: &http.Transport{},
	}
}

// WithNoVerify configures the client to skip TLS verification.
//
// Example usage:
//
// # Example usage
//
//	client := GetClient().WithNoVerify().Build()
//
// This example creates an HTTP client that skips TLS verification.
func (c ClientBuilder) WithNoVerify() ClientBuilder {
	c.tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return c
}

// WithTimeout sets the timeout for the HTTP client.
//
// # Example usage
//
//	client := GetClient().WithTimeout(30 * time.Second).Build()
//
// This example creates an HTTP client with a 30-second timeout.
func (c ClientBuilder) WithTimeout(to time.Duration) ClientBuilder {
	c.timeout = to
	return c
}

// WithResolver configures the client to use a custom resolver.
//
// # Example usage
//
//	client := GetClient().WithResolver().Build()
//
// This example creates an HTTP client with a custom resolver.
func (c ClientBuilder) WithResolver() ClientBuilder {
	c.tr.DialContext = newDialContext()

	return c
}

// Build constructs the [*net/http.Client] with the configured options.
//
// # Example usage
//
//	client := GetClient().WithNoVerify().WithTimeout(30 * time.Second).WithResolver().Build()
//
// This example creates an HTTP client with no TLS verification, a 30-second timeout, and a custom resolver.
func (c ClientBuilder) Build() *http.Client {
	return &http.Client{
		Transport: c.tr,
		Timeout:   c.timeout,
	}
}

// DoGet is a wrapper around performing HTTP GET requests
func DoGet(c *http.Client, url string, conn ShouldCloseConnection) (body []byte, e error) {
	return DoGetWithOptions(c, url, &ReqOptions{Conn: conn})
}

// DoGetWithOptions is a wrapper around performing HTTP GET requests
func DoGetWithOptions(c *http.Client, url string, options *ReqOptions) (body []byte, e error) {
	if options.Authtoken == "" {
		options.Authtoken = GetAuthToken()
	}

	if options.Ctx == nil {
		options.Ctx = context.Background()
	}

	req, e := http.NewRequestWithContext(options.Ctx, "GET", url, nil)
	if e != nil {
		return body, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+options.Authtoken)
	if options.Conn == CloseConnection {
		req.Close = true
	}

	r, e := c.Do(req)
	if e != nil {
		return body, e
	}
	body, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return body, e
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

// DoPost is a wrapper around performing HTTP POST requests
func DoPost(c *http.Client, url string, contentType string, body io.Reader) (resp []byte, e error) {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return resp, e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return resp, e
	}
	resp, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return resp, e
	}
	if r.StatusCode >= 400 {
		return resp, errors.New(string(resp))
	}
	return resp, nil
}

// DoPostChunked is a wrapper around performing HTTP POST requests that stream chunked data
func DoPostChunked(c *http.Client, url string, contentType string, body io.Reader, onChunk func([]byte)) error {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()

	var m int
	buf := make([]byte, 4096)
	for {
		m, e = r.Body.Read(buf)
		if m < 0 || e != nil {
			break
		}
		onChunk(buf[:m])
	}

	if r.StatusCode == 200 {
		return nil
	}
	return e
}
