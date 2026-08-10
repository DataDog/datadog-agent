// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

func TestClassifyRegistryError(t *testing.T) {
	// A reference that really fails to parse, rather than a hand-built
	// name.ErrBadName: its only field is unexported.
	_, badNameErr := name.ParseReference("NOT A REFERENCE")
	require.Error(t, badNameErr)

	tests := []struct {
		name string
		err  error
		want FailureKind
	}{
		{"nil", nil, FailureKindUnknown},
		{"bad reference", badNameErr, FailureKindInvalidReference},
		{"wrapped bad reference", fmt.Errorf("download: %w", badNameErr), FailureKindInvalidReference},
		{"401", &transport.Error{StatusCode: http.StatusUnauthorized}, FailureKindAuthRejected},
		{"403", &transport.Error{StatusCode: http.StatusForbidden}, FailureKindAuthRejected},
		{"500", &transport.Error{StatusCode: http.StatusInternalServerError}, FailureKindHTTPStatus},
		{"404", &transport.Error{StatusCode: http.StatusNotFound}, FailureKindHTTPStatus},
		{"certificate verification", &tls.CertificateVerificationError{Err: errors.New("boom")}, FailureKindTLS},
		{"unknown authority", x509.UnknownAuthorityError{}, FailureKindTLS},
		{"hostname mismatch", x509.HostnameError{Certificate: &x509.Certificate{}}, FailureKindTLS},
		{"record header", tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}, FailureKindTLS},
		{"dns", &net.DNSError{Err: "no such host", Name: "install.datadoghq.com"}, FailureKindDNS},
		{"connection refused", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, FailureKindConnection},
		{"deadline exceeded", context.DeadlineExceeded, FailureKindConnection},
		{"unrelated", errors.New("something else"), FailureKindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyRegistryError(tt.err))
		})
	}
}

// TestClassifyRegistryErrorTLSBeforeNet pins the ordering that matters: a
// certificate failure arrives wrapped in a *net.OpError, and reading it as a
// connection problem would blame the customer's network for what is really a
// certificate problem.
func TestClassifyRegistryErrorTLSBeforeNet(t *testing.T) {
	err := &net.OpError{
		Op:  "remote error",
		Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
	}
	assert.Equal(t, FailureKindTLS, classifyRegistryError(err))
}

func TestFailureKindString(t *testing.T) {
	// Every kind must have a distinct label: the backend groups on it.
	seen := map[string]FailureKind{}
	for k := FailureKindUnknown; k <= FailureKindInvalidReference; k++ {
		s := k.String()
		assert.NotEqual(t, "", s)
		if prev, dup := seen[s]; dup {
			t.Fatalf("kinds %d and %d share the label %q", prev, k, s)
		}
		seen[s] = k
	}
	assert.Equal(t, "unknown", FailureKind(42).String())
}

func TestReachabilityReachable(t *testing.T) {
	tests := []struct {
		name string
		r    *Reachability
		want bool
	}{
		{"nil", nil, false},
		{"no registries", &Reachability{}, false},
		{
			// The installer falls back through the registry list, so a host that
			// reaches any one of them can download. Reporting this as unreachable
			// would reject a host that works.
			name: "fallback reachable",
			r: &Reachability{Registries: []RegistryStatus{
				{Registry: "a", FailureKind: FailureKindDNS},
				{Registry: "b", Reachable: true},
			}},
			want: true,
		},
		{
			name: "none reachable",
			r: &Reachability{Registries: []RegistryStatus{
				{Registry: "a", FailureKind: FailureKindDNS},
				{Registry: "b", FailureKind: FailureKindConnection},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.Reachable())
		})
	}
}

func TestReachabilityFromDownloadError(t *testing.T) {
	// No per-registry detail: the failure happened somewhere other than the
	// registry leg, so it says nothing about reachability and must not be
	// reported as a reachability result.
	assert.Nil(t, ReachabilityFromDownloadError(nil))
	assert.Nil(t, ReachabilityFromDownloadError(errors.New("could not extract layers")))

	err := errors.Join(
		&RegistryError{Registry: "install.datadoghq.com/agent-package", Err: &net.DNSError{Err: "no such host"}},
		&RegistryError{Registry: "gcr.io/datadoghq/agent-package", Err: &transport.Error{StatusCode: http.StatusUnauthorized}},
	)
	r := ReachabilityFromDownloadError(err)
	require.NotNil(t, r)
	assert.True(t, r.FromDownload)
	assert.False(t, r.CheckedAt.IsZero())
	assert.False(t, r.Reachable())
	require.Len(t, r.Registries, 2)
	assert.Equal(t, "install.datadoghq.com/agent-package", r.Registries[0].Registry)
	assert.Equal(t, FailureKindDNS, r.Registries[0].FailureKind)
	assert.Equal(t, FailureKindAuthRejected, r.Registries[1].FailureKind)
}

func TestDefaultRegistries(t *testing.T) {
	assert.Equal(t, defaultRegistriesStaging, defaultRegistries(&env.Env{Site: "datad0g.com"}))
	assert.Equal(t, defaultRegistriesProd, defaultRegistries(&env.Env{Site: "datadoghq.com"}))
	assert.Equal(t, defaultRegistriesProd, defaultRegistries(&env.Env{}))
}

func TestCheckReachabilityInvalidReferenceIsNotANetworkError(t *testing.T) {
	// The defect this whole signal exists to fix: an unparseable reference and a
	// host with no route to the registry are both installer error code 1 today.
	// The probe must tell them apart without making a request.
	d := NewDownloader(&env.Env{RegistryOverride: "NOT A REGISTRY"}, http.DefaultClient)
	r := d.CheckReachability(context.Background(), "agent-package")
	require.NotNil(t, r)
	require.NotEmpty(t, r.Registries)
	assert.False(t, r.Reachable())
	for _, s := range r.Registries {
		assert.Equal(t, FailureKindInvalidReference, s.FailureKind)
	}
}

func TestCheckReachabilityProbesEveryDefaultRegistry(t *testing.T) {
	// With no override the probe must walk the real fallback list rather than
	// treating the bare image name as a Docker Hub reference. Point the
	// downloader at a transport that fails every request so no test reaches the
	// network.
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	})}
	d := NewDownloader(&env.Env{}, client)

	r := d.CheckReachability(context.Background(), "")
	require.NotNil(t, r)
	assert.False(t, r.Reachable())
	assert.Len(t, r.Registries, len(defaultRegistriesProd))
	for i, s := range r.Registries {
		assert.Contains(t, s.Registry, defaultRegistriesProd[i])
		assert.Contains(t, s.Registry, DefaultProbeImage)
		assert.Equal(t, FailureKindConnection, s.FailureKind, "registry %s", s.Registry)
		assert.Error(t, s.Err)
	}
}

func TestCheckReachabilityStopsAtFirstReachable(t *testing.T) {
	// Every extra probe is a request every host in the fleet makes on a timer,
	// so probing must stop where a real download would.
	var requested []string
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.Host)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     http.Header{},
			Request:    req,
		}, nil
	})}
	d := NewDownloader(&env.Env{}, client)

	r := d.CheckReachability(context.Background(), "")
	require.NotNil(t, r)
	assert.True(t, r.Reachable())
	require.Len(t, r.Registries, 1)
	assert.Contains(t, r.Registries[0].Registry, defaultRegistriesProd[0])
	assert.NotEmpty(t, requested)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
