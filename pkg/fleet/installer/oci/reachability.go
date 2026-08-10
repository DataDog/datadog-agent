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
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultProbeImage is the image used to probe registry reachability when no
// specific package is in flight. It must be a real image name so that
// image-scoped registry and auth overrides resolve exactly as they would for a
// real download.
const DefaultProbeImage = "agent-package"

// defaultProbeTimeout bounds a single registry probe. The installer's HTTP
// client has a 30s dial timeout and no overall timeout, which is fine for a
// multi-gigabyte download but far too long for a check that runs on the state
// refresh path.
const defaultProbeTimeout = 10 * time.Second

// FailureKind classifies why a registry could not be reached. The causes are
// separated because they need different owners and different remediation: a
// host with no route to the registry is a customer precondition, while an
// unparseable reference is a defect in what we published. Today they are
// indistinguishable because both surface as installer error code 1.
type FailureKind int

const (
	// FailureKindUnknown is an unclassified failure.
	FailureKindUnknown FailureKind = iota
	// FailureKindDNS is a name resolution failure.
	FailureKindDNS
	// FailureKindConnection is a refused, reset, unreachable or timed-out
	// connection: the host has no route to the registry.
	FailureKindConnection
	// FailureKindTLS is a TLS handshake or certificate verification failure,
	// typically an intercepting proxy without a trusted CA.
	FailureKindTLS
	// FailureKindAuthConfig is a local registry-credential file that could not
	// be read or parsed. The registry itself may be perfectly reachable.
	FailureKindAuthConfig
	// FailureKindAuthRejected is credentials the registry refused (401/403).
	FailureKindAuthRejected
	// FailureKindHTTPStatus is any other unexpected HTTP status.
	FailureKindHTTPStatus
	// FailureKindInvalidReference is a registry or image reference that could
	// not be parsed. No network request was made.
	FailureKindInvalidReference
)

// String implements fmt.Stringer.
func (k FailureKind) String() string {
	switch k {
	case FailureKindDNS:
		return "dns"
	case FailureKindConnection:
		return "connection"
	case FailureKindTLS:
		return "tls"
	case FailureKindAuthConfig:
		return "auth_config"
	case FailureKindAuthRejected:
		return "auth_rejected"
	case FailureKindHTTPStatus:
		return "http_status"
	case FailureKindInvalidReference:
		return "invalid_reference"
	default:
		return "unknown"
	}
}

// RegistryStatus is the outcome of probing one registry.
type RegistryStatus struct {
	// Registry is the reference that was attempted. getRefAndKeychain
	// guarantees no embedded userinfo, so it is safe to log and export.
	Registry string
	// Reachable is true when the registry answered its /v2/ endpoint and
	// accepted our credentials for a pull.
	Reachable bool
	// FailureKind classifies Err. Only meaningful when Err is non-nil.
	FailureKind FailureKind
	// Err is the underlying error, and the discriminator for whether this
	// registry was attempted at all: it is nil both when the registry is
	// reachable and when probing stopped before reaching it. Callers must not
	// read FailureKind without checking Err first, or a not-attempted registry
	// reads as a failure of unknown cause.
	Err error
}

// Reachability reports whether this host can currently reach a registry to
// download packages from.
type Reachability struct {
	// Registries holds one entry per registry the installer would try, in the
	// order it would try them.
	Registries []RegistryStatus
	// CheckedAt is when the result was produced. Zero means never checked:
	// callers must treat that as unknown, not as unreachable.
	CheckedAt time.Time
	// FromDownload is true when this result was observed from a real package
	// download rather than a standalone probe.
	FromDownload bool
}

// Reachable reports whether at least one registry is reachable. The installer
// falls back through the registry list, so the host can download a package as
// long as any one of them answers.
func (r *Reachability) Reachable() bool {
	if r == nil {
		return false
	}
	for _, s := range r.Registries {
		if s.Reachable {
			return true
		}
	}
	return false
}

// roundTripper builds the round tripper used to talk to registries, applying
// the mirror when one is configured.
func (d *Downloader) roundTripper() (http.RoundTripper, error) {
	rt := telemetry.WrapRoundTripper(d.client.Transport)
	if d.env.Mirror == "" {
		return rt, nil
	}
	rt, err := newMirrorTransport(rt, d.env.Mirror)
	if err != nil {
		return nil, fmt.Errorf("could not create mirror transport: %w", err)
	}
	return rt, nil
}

// CheckReachability probes every registry the installer would try for the given
// image, in order, and reports each outcome.
//
// The probe deliberately runs through the same reference list, keychain and
// transport a real download uses, rather than a plain HTTP request to the
// registry host. A bare connectivity check reports a host as healthy when its
// local credential file is unparseable, which is a failure mode we have
// observed in production and cannot afford to miss.
//
// Probing stops at the first reachable registry: the installer would stop there
// too, and every extra probe is a request every host in the fleet makes on a
// timer. Registries after the first success are reported with Reachable false
// and a nil Err, meaning "not attempted" — read Reachability.Reachable rather
// than requiring every entry to be true.
func (d *Downloader) CheckReachability(ctx context.Context, image string) *Reachability {
	if image == "" {
		image = DefaultProbeImage
	}
	res := &Reachability{CheckedAt: time.Now()}
	rt, err := d.roundTripper()
	if err != nil {
		// Not registry-specific: report it against the configured mirror so the
		// result is never silently empty.
		res.Registries = []RegistryStatus{{
			Registry:    d.env.Mirror,
			FailureKind: FailureKindUnknown,
			Err:         err,
		}}
		return res
	}

	// A tag is required to build a well-formed reference. Which tag is
	// irrelevant: the probe only pings /v2/ and negotiates a pull scope for the
	// repository, it never resolves the tag.
	probeURL := image + ":latest"
	if d.env.RegistryOverride == "" {
		// No override: seed the URL with the primary default registry so
		// getRefAndKeychains returns the real fallback list rather than treating
		// the bare image name as a Docker Hub reference.
		probeURL = defaultRegistries(d.env)[0] + "/" + probeURL
	}

	for _, refAndKeychain := range getRefAndKeychains(d.env, probeURL) {
		status := d.probeRegistry(ctx, refAndKeychain, rt)
		res.Registries = append(res.Registries, status)
		if status.Reachable {
			log.Debugf("Registry %s is reachable", status.Registry)
			// Stop here: the installer would too.
			break
		}
		log.Debugf("Registry %s is not reachable (%s): %v", status.Registry, status.FailureKind, status.Err)
	}
	return res
}

// probeRegistry probes a single registry. The steps are kept separate so each
// failure is classified by where it happened rather than by matching on error
// text: parsing never touches the network, and a keychain failure is a local
// credential-file problem regardless of what the registry would have said.
func (d *Downloader) probeRegistry(ctx context.Context, refAndKeychain urlWithKeychain, rt http.RoundTripper) RegistryStatus {
	status := RegistryStatus{Registry: refAndKeychain.ref}

	ref, err := name.ParseReference(refAndKeychain.ref)
	if err != nil {
		status.FailureKind = FailureKindInvalidReference
		status.Err = err
		return status
	}

	ctx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()

	auth, err := refAndKeychain.keychain.Resolve(ref.Context())
	if err != nil {
		// Reading or parsing the local credential file failed. The registry may
		// well be reachable; the host simply cannot be used as configured.
		status.FailureKind = FailureKindAuthConfig
		status.Err = err
		return status
	}

	// transport.NewWithContext pings the registry's /v2/ endpoint and completes
	// the auth handshake, which is the cheapest exchange that proves a pull
	// would be authorised.
	if _, err := transport.NewWithContext(ctx, ref.Context().Registry, auth, rt, []string{ref.Scope(transport.PullScope)}); err != nil {
		status.FailureKind = classifyRegistryError(err)
		status.Err = err
		return status
	}

	status.Reachable = true
	return status
}

// classifyRegistryError maps a registry error to a FailureKind. It inspects
// error types rather than message text so it does not break when an upstream
// dependency rewords an error.
func classifyRegistryError(err error) FailureKind {
	if err == nil {
		return FailureKindUnknown
	}

	var badName *name.ErrBadName
	if errors.As(err, &badName) {
		return FailureKindInvalidReference
	}

	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		switch transportErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return FailureKindAuthRejected
		default:
			return FailureKindHTTPStatus
		}
	}

	// TLS before the generic net checks: a certificate failure arrives wrapped
	// in a *net.OpError, which would otherwise be read as a connection problem.
	var certErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostnameErr) || errors.As(err, &recordHeaderErr) {
		return FailureKindTLS
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return FailureKindDNS
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return FailureKindConnection
	}
	// context.DeadlineExceeded already satisfies net.Error, so the check above
	// normally catches our own probe timeout. Keep this so a timeout is still
	// classified as a connection failure if a wrapper drops those methods.
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureKindConnection
	}

	return FailureKindUnknown
}

// ReachabilityFromDownloadError derives a reachability result from a failed
// download, so a real attempt can refresh the signal without spending a probe.
// It returns nil when err carries no per-registry detail, which means the
// failure happened somewhere other than the registry leg and therefore says
// nothing about reachability.
//
// This is only usable where the download runs in the same process as the cache.
// The daemon runs downloads in a datadog-installer subprocess and receives the
// failure as opaque text, so it invalidates its cache instead — see
// ReachabilityCache.Invalidate.
func ReachabilityFromDownloadError(err error) *Reachability {
	registryErrs := RegistryErrors(err)
	if len(registryErrs) == 0 {
		return nil
	}
	res := &Reachability{CheckedAt: time.Now(), FromDownload: true}
	for _, re := range registryErrs {
		res.Registries = append(res.Registries, RegistryStatus{
			Registry:    re.Registry,
			FailureKind: classifyRegistryError(re.Err),
			Err:         re.Err,
		})
	}
	return res
}

// defaultRegistries returns the default registry list for the configured site.
func defaultRegistries(e *env.Env) []string {
	if e.Site == "datad0g.com" {
		return defaultRegistriesStaging
	}
	return defaultRegistriesProd
}
