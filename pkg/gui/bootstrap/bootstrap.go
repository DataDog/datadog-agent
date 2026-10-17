// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package bootstrap hands the GUI intent token to a browser without ever
// putting it on a command line.
//
// The Agent GUI bootstraps a browser session from a single-use intent token
// that `agent launch-gui` (and the Windows systray) fetches over IPC. Passing
// that token to the browser as part of a URL means passing it to the OS
// URL-opener as an argv element, where any co-resident local user can read it
// through /proc/<pid>/cmdline or a process listing (VULN-92705, CWE-214).
//
// Instead, Write puts the token in a file that only the launching user can
// read and returns a file:// URL for it. The page auto-submits the token to the
// GUI's /auth endpoint. The reader of the file is therefore the *browser*,
// which runs as the launching user — so the file permissions are what enforce
// "same local user", and argv carries nothing but a path an attacker cannot
// open.
package bootstrap

import (
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	template "github.com/DataDog/datadog-agent/pkg/template/html"
)

// Token is the single-use credential the GUI issues on /agent/gui/intent. The
// id names the pending launch, the secret proves ownership of it.
type Token struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

const (
	// dirPrefix names the per-launch directory created in the user's temporary
	// directory. Sweep relies on it to recognize its own leftovers.
	dirPrefix = "datadog-agent-gui-"

	// pageName is the bootstrap page's file name inside that directory.
	pageName = "launch.html"

	// sweepAge is how long a leftover bootstrap directory is kept before Sweep
	// removes it. Anything this old is already inert: intent tokens are
	// single-use and expire after 30 seconds.
	sweepAge = 5 * time.Minute
)

//go:embed launch.html.tmpl
var templateFS embed.FS

var pageTemplate = template.Must(template.ParseFS(templateFS, "launch.html.tmpl"))

type pageData struct {
	Action string
	ID     string
	Secret string
}

// Write renders the bootstrap page for the given GUI address into a file only
// the current user can read, and returns a file:// URL pointing at it. The
// caller is expected to hand that URL to the OS URL-opener.
func Write(guiAddress string, tok Token) (string, error) {
	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		return "", fmt.Errorf("unable to create the GUI bootstrap directory: %w", err)
	}

	pagePath := filepath.Join(dir, pageName)

	// The 0600 is the whole point of this package: the browser runs as the user
	// who ran launch-gui, so it can read the token while a co-resident user
	// cannot. O_EXCL because the directory was just created under a random
	// name, so nothing should exist at this path yet.
	//
	// The mode does nothing on Windows, where it is not an access control
	// mechanism at all; there the file is covered by the ACL that the user's
	// %TEMP% already carries. The same asymmetry is spelled out in
	// comp/dataplane/preflightmode/impl/config.go.
	//
	// Note this deliberately does not use filesystem.Permission's
	// RestrictAccessToUser: that grants dd-agent on Unix and
	// Administrators/SYSTEM/ddagentuser on Windows, i.e. the Agent's own
	// account rather than the account the browser runs as, which would lock the
	// browser out. cmd/otel-agent/subcommands/flare/command.go rejects it for
	// the same reason.
	f, err := os.OpenFile(pagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("unable to create the GUI bootstrap page: %w", err)
	}

	data := pageData{
		Action: "http://" + guiAddress + "/auth",
		ID:     tok.ID,
		Secret: tok.Secret,
	}

	if err := pageTemplate.Execute(f, data); err != nil {
		f.Close()
		os.RemoveAll(dir)
		return "", fmt.Errorf("unable to render the GUI bootstrap page: %w", err)
	}

	if err := f.Close(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("unable to write the GUI bootstrap page: %w", err)
	}

	return fileURL(pagePath), nil
}

// Sweep removes bootstrap directories left behind by earlier launches. It is
// best effort and never reports failures: a leftover page is harmless because
// the token it holds is single-use and short lived.
//
// This needs no ownership checks. On Unix the temporary directory is sticky, so
// an entry belonging to another user simply fails to unlink; on Windows %TEMP%
// is already per user. Directory entries that are symlinks are skipped by the
// IsDir check rather than followed.
func Sweep() {
	tempDir := os.TempDir()

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-sweepAge)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), dirPrefix) {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}

		os.RemoveAll(filepath.Join(tempDir, entry.Name()))
	}
}

// fileURL turns a local path into a file:// URL. Windows paths are not rooted
// at "/" once slash-separated ("C:/Users/..."), so they need one added for the
// URL to have an empty authority.
func fileURL(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}

	u := url.URL{Scheme: "file", Path: slashed}
	return u.String()
}
