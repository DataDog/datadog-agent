// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"sync"
	"time"

	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// defaultIPCCertRefetchInterval is the minimum delay between two attempts to
// read the IPC certificate from the host.
const defaultIPCCertRefetchInterval = 10 * time.Second

// ipcCertProvider presents the host's IPC certificate to agent servers that
// request it, reading ipc_cert.pem from the host on demand. It backs
// tls.Config.GetClientCertificate: returning an empty certificate sends no
// client certificate, which keeps handshakes working with servers that don't
// verify client certificates or expect another CA.
type ipcCertProvider struct {
	fetch           func() (string, error) // reads the certificate file from the host
	refetchInterval time.Duration          // test override of defaultIPCCertRefetchInterval

	mu        sync.Mutex
	cert      *tls.Certificate
	subject   []byte // RawSubject of cert: the CA agent IPC servers accept
	lastFetch time.Time
}

func newIPCCertProvider(fetch func() (string, error)) *ipcCertProvider {
	return &ipcCertProvider{fetch: fetch, refetchInterval: defaultIPCCertRefetchInterval}
}

// getCertificate implements tls.Config.GetClientCertificate.
func (p *ipcCertProvider) getCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Fetch on the first handshake, then refresh periodically: the certificate
	// is regenerated on agent reinstall with the same subject, and AcceptableCAs
	// alone cannot detect that.
	if time.Since(p.lastFetch) > p.refetchInterval {
		p.fetchLocked()
	}

	// The server lists the CAs it accepts; send an empty certificate if the
	// IPC certificate isn't among them (e.g. the cluster-agent trust domain).
	// An empty non-nil Certificate means "no client certificate".
	if p.matchesAcceptableCAs(info.AcceptableCAs) {
		return p.cert, nil
	}
	return &tls.Certificate{}, nil
}

// matchesAcceptableCAs reports whether the server accepts the CA of the cached
// certificate. Agent IPC servers list it in CertificateRequestInfo.AcceptableCAs.
func (p *ipcCertProvider) matchesAcceptableCAs(acceptableCAs [][]byte) bool {
	if p.cert == nil {
		return false
	}
	for _, ca := range acceptableCAs {
		if bytes.Equal(ca, p.subject) {
			return true
		}
	}
	return false
}

// fetchLocked reads the certificate file and updates the cache. Best effort:
// on error the cache is left unchanged.
func (p *ipcCertProvider) fetchLocked() {
	p.lastFetch = time.Now()

	content, err := p.fetch()
	if err != nil {
		return
	}
	// The file holds both the certificate and the key; X509KeyPair picks the
	// right PEM block from each argument.
	cert, err := tls.X509KeyPair([]byte(content), []byte(content))
	if err != nil {
		return
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return
	}
	p.cert = &cert
	p.subject = leaf.RawSubject
}

// fetchIPCCert reads ipc_cert.pem from the host.
func (h *Host) fetchIPCCert() (string, error) {
	out, err := h.Execute(fetchIPCCertCommand(h.osFamily))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func fetchIPCCertCommand(osFamily oscomp.Family) string {
	switch osFamily {
	case oscomp.WindowsFamily:
		return "Get-Content -Raw -Path C:\\ProgramData\\Datadog\\ipc_cert.pem"
	case oscomp.MacOSFamily:
		return "sudo cat /opt/datadog-agent/etc/ipc_cert.pem"
	default:
		return "sudo cat /etc/datadog-agent/ipc_cert.pem"
	}
}
