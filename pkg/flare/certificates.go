// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// Environment variables consulted by Go's crypto/x509 when loading the system
// root certificates on Unix systems other than macOS. See root_unix.go in the
// Go standard library.
const (
	certFileEnv = "SSL_CERT_FILE"
	certDirEnv  = "SSL_CERT_DIR"
)

// Default certificate locations hardcoded in Go's crypto/x509 for the supported
// platforms where roots are loaded from the filesystem (see root_linux.go and
// root_aix.go in the Go standard library). Keep in sync with the toolchain used
// to build the agent. On other supported platforms (macOS, Windows) Go reads
// the roots from the OS trust store instead.
var (
	certFilesByOS = map[string][]string{
		"linux": {
			"/etc/ssl/certs/ca-certificates.crt",                // Debian/Ubuntu/Gentoo etc.
			"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora/RHEL 6
			"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
			"/etc/pki/tls/cacert.pem",                           // OpenELEC
			"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS/RHEL 7
			"/etc/ssl/cert.pem",                                 // Alpine Linux
		},
		"aix": {
			"/var/ssl/certs/ca-bundle.crt",
		},
	}
	certDirsByOS = map[string][]string{
		"linux": {
			"/etc/ssl/certs",     // SLES10/SLES11, https://go.dev/issue/12139
			"/etc/pki/tls/certs", // Fedora/RHEL
		},
		"aix": {
			"/var/ssl/certs",
		},
	}
)

// Bounds to avoid blowing up the flare size or runtime.
const (
	certDirListMaxDepth   = 10
	certDirListMaxEntries = 10000
	certMaxParseSize      = 10 * 1024 * 1024
)

// provideCertificateSources reports how the Go runtime resolves the trusted
// root certificates on this host: which environment variables and default
// locations are used, and what they contain.
func provideCertificateSources(_ context.Context, fb flaretypes.FlareBuilder) error {
	// Only relevant on platforms where Go loads the roots from the filesystem.
	if _, ok := certFilesByOS[runtime.GOOS]; !ok {
		return nil
	}
	fb.AddFileFromFunc("certificates/tls_roots.txt", getCertificateSourcesReport) //nolint:errcheck
	return nil
}

func getCertificateSourcesReport() ([]byte, error) {
	var b bytes.Buffer

	fmt.Fprintf(&b, "%s: %s\n", certFileEnv, envOrNotSet(certFileEnv))
	fmt.Fprintf(&b, "%s: %s\n", certDirEnv, envOrNotSet(certDirEnv))
	fmt.Fprintln(&b)

	// unique fingerprints of all certificates Go would load
	loaded := make(map[[sha256.Size]byte]struct{})
	writeCertFilesSection(&b, loaded)
	writeCertDirListings(&b, loaded)

	fmt.Fprintf(&b, "total: %d certificates\n", len(loaded))

	return b.Bytes(), nil
}

func envOrNotSet(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return "(not set)"
}

// writeCertFilesSection lists the certificate bundle files Go will consider,
// with their status. Mirrors loadSystemRoots: the first file that can be read
// is the one loaded, even if it contains no certificate.
func writeCertFilesSection(b *bytes.Buffer, loaded map[[sha256.Size]byte]struct{}) {
	files := certFilesByOS[runtime.GOOS]
	if f := os.Getenv(certFileEnv); f != "" {
		files = []string{f}
	}

	used := false
	for _, f := range files {
		data, err := readCertFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(b, "%s: not found\n", f)
			} else {
				fmt.Fprintf(b, "%s: %v\n", f, err)
			}
			continue
		}
		if !used {
			used = true
			fmt.Fprintf(b, "%s: %d bytes, %d certificates (used)\n", f, len(data), countPEMCerts(data, loaded))
		} else {
			// Go only loads the first readable file; count the others for display only.
			fmt.Fprintf(b, "%s: %d bytes, %d certificates (not used)\n", f, len(data), countPEMCerts(data, nil))
		}
	}
	fmt.Fprintln(b)
}

// writeCertDirListings writes an ls -R style listing of each certificate
// directory Go will read, and adds the certificates found to loaded.
func writeCertDirListings(b *bytes.Buffer, loaded map[[sha256.Size]byte]struct{}) {
	dirs := certDirsByOS[runtime.GOOS]
	if d := os.Getenv(certDirEnv); d != "" {
		dirs = strings.Split(d, ":")
	}

	var stats certDirStats
	for _, d := range dirs {
		writeCertDirListing(b, d, 0, &stats, loaded)
	}
	if stats.truncated {
		fmt.Fprintln(b, "warning: listing truncated")
	}
	fmt.Fprintln(b)
}

// certDirStats accumulates statistics while listing certificate directories.
type certDirStats struct {
	dirs      int
	files     int
	truncated bool
}

// writeCertDirListing writes an ls -R style listing of dir into b, recursing
// into sub-directories up to certDirListMaxDepth. Symlinked directories are
// not followed. When loaded is not nil, the certificates found in dir are
// added to it; nested directories pass nil so their certificates are listed
// but not counted (Go only loads the immediate entries of each directory).
func writeCertDirListing(b *bytes.Buffer, dir string, depth int, stats *certDirStats, loaded map[[sha256.Size]byte]struct{}) {
	if depth >= certDirListMaxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(b, "%s: not found\n", dir)
		return
	}

	var subDirs []string
	dirBuffer := new(bytes.Buffer)
	for _, e := range entries {
		if stats.files+stats.dirs+len(subDirs) >= certDirListMaxEntries {
			stats.truncated = true
			break
		}
		name := e.Name()
		full := filepath.Join(dir, name)

		switch {
		case e.IsDir():
			subDirs = append(subDirs, name)
			fmt.Fprintf(dirBuffer, "%s/\n", name)

		case e.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				fmt.Fprintf(dirBuffer, "%s: cannot read link: %v\n", name, err)
				continue
			}
			// Mirror readUniqueDirectoryEntries: symlinks that point inside
			// the same directory (target without a "/") are skipped by Go to
			// avoid duplicates (e.g. openssl rehash style hashed names).
			if !strings.Contains(target, "/") {
				fmt.Fprintf(dirBuffer, "%s -> %s (skipped: same-directory symlink)\n", name, target)
				continue
			}
			st, err := os.Stat(full)
			switch {
			case err != nil:
				fmt.Fprintf(dirBuffer, "%s -> %s (dangling)\n", name, target)
			case st.IsDir():
				fmt.Fprintf(dirBuffer, "%s -> %s\n", name, target)
			default:
				if n, err := countCertsInFile(full, loaded); err != nil {
					fmt.Fprintf(dirBuffer, "%s -> %s (%s)\n", name, target, err)
				} else {
					fmt.Fprintf(dirBuffer, "%s -> %s (%d certificates)\n", name, target, n)
				}
			}

		default:
			stats.files++
			st, err := e.Info()
			if err != nil {
				fmt.Fprintf(dirBuffer, "%s: cannot stat: %v\n", name, err)
				continue
			}
			line := fmt.Sprintf("%s %10d %s  %s", st.Mode().String(), st.Size(), st.ModTime().Format(time.RFC3339), name)
			if n, err := countCertsInFile(full, loaded); err != nil {
				line += fmt.Sprintf(" (%s)", err)
			} else {
				line += fmt.Sprintf(" (%d certificates)", n)
			}
			fmt.Fprintln(dirBuffer, line)
		}
	}

	fmt.Fprintf(b, "%s: %d entries\n", dir, len(entries))
	b.Write(dirBuffer.Bytes())
	stats.dirs++

	for _, sub := range subDirs {
		writeCertDirListing(b, filepath.Join(dir, sub), depth+1, stats, nil)
	}
}

// readCertFile returns the content of path if it is a regular file not larger
// than certMaxParseSize. Symlinks are followed, so that a link to a huge file
// cannot exhaust memory and a FIFO or device cannot block flare generation.
func readCertFile(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if st.Size() > certMaxParseSize {
		return nil, errors.New("too large to parse")
	}
	return os.ReadFile(path)
}

// countCertsInFile parses the PEM certificates in path and adds them to
// loaded (deduplicated, like a CertPool). It returns the number of unique
// certificates found, or an error when the file cannot be read safely.
func countCertsInFile(path string, loaded map[[sha256.Size]byte]struct{}) (int, error) {
	data, err := readCertFile(path)
	if err != nil {
		return 0, err
	}
	return countPEMCerts(data, loaded), nil
}

// countPEMCerts parses the PEM certificate blocks of data and adds them to
// loaded if it is not nil (deduplicated, like a CertPool). It returns the
// number of unique certificates found in data.
func countPEMCerts(data []byte, loaded map[[sha256.Size]byte]struct{}) int {
	local := make(map[[sha256.Size]byte]struct{})
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		// Mirror CertPool.AppendCertsFromPEM.
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		local[sha256.Sum256(cert.Raw)] = struct{}{}
	}
	if loaded != nil {
		for sum := range local {
			loaded[sum] = struct{}{}
		}
	}
	return len(local)
}
