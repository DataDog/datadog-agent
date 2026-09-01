// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package macpkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// DatadogTeamID is the Apple Developer team identifier every Datadog-signed macOS artifact is
// signed with, as declared by the signing identities in omnibus/config/projects/agent.rb. It is
// the value the signature chain is pinned to: a valid signature from another team is a valid
// signature from someone else.
const DatadogTeamID = "JKFCB4CN7C"

const (
	pkgutilPath  = "/usr/sbin/pkgutil"
	spctlPath    = "/usr/sbin/spctl"
	staplerPath  = "/usr/bin/stapler"
	assessTarget = "install"
)

// Verifier decides whether a downloaded package may be installed.
type Verifier interface {
	// Verify reports nil only when the package is safe to hand to the system installer.
	Verify(ctx context.Context, pkg *DownloadedPackage, expectedDigest string) error
}

// Runner executes a command and returns its combined output. Nil runs the real binary; tests
// substitute a recorder.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// pkgVerifier is the production Verifier: digest, signature chain, team identifier, notarization.
type pkgVerifier struct {
	expectedTeamID string
	runner         Runner
}

// NewVerifier returns a Verifier pinned to the Datadog team identifier.
func NewVerifier() Verifier {
	return &pkgVerifier{expectedTeamID: DatadogTeamID}
}

// Verify runs the checks in order, cheapest and most conclusive first.
//
// The digest comes before the signature checks because it is the one check that does not depend
// on the machine's trust state: it answers "are these the bytes the catalog named", which a
// misconfigured or subverted Gatekeeper cannot affect. expectedDigest may be empty, in which case
// the digest check is skipped and reported as such -- a catalog that does not carry a digest is a
// gap in the catalog, not a reason to refuse every update.
func (v *pkgVerifier) Verify(ctx context.Context, pkg *DownloadedPackage, expectedDigest string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "macpkg.verify")
	defer func() { span.Finish(err) }()

	if pkg == nil || pkg.Path == "" {
		return errors.New("nothing to verify")
	}
	if info, statErr := os.Stat(pkg.Path); statErr != nil {
		return fmt.Errorf("could not stat %s: %w", pkg.Path, statErr)
	} else if info.Size() == 0 {
		return fmt.Errorf("%s is empty", pkg.Path)
	}

	if err := v.checkDigest(pkg, expectedDigest); err != nil {
		return err
	}
	if err := v.checkSignature(ctx, pkg.Path); err != nil {
		return err
	}
	return v.checkNotarization(ctx, pkg.Path)
}

func (v *pkgVerifier) checkDigest(pkg *DownloadedPackage, expectedDigest string) error {
	if expectedDigest == "" {
		return nil
	}
	if !strings.EqualFold(pkg.Digest, expectedDigest) {
		return fmt.Errorf("digest mismatch for %s: the catalog named %s, the download is %s", pkg.Path, expectedDigest, pkg.Digest)
	}
	return nil
}

// teamIDRe matches the team identifier pkgutil reports for a signed package. pkgutil's output is
// a human-readable report rather than a structured one, so only the one line that matters is
// matched.
var teamIDRe = regexp.MustCompile(`(?m)^\s*(?:1\.\s*)?Developer ID Installer:.*\((\w+)\)\s*$`)

// checkSignature runs pkgutil --check-signature and pins the team identifier.
//
// Two things have to be true and pkgutil reports both: that the package is signed by a chain the
// system trusts, which is its exit status, and that the leaf belongs to Datadog, which is only in
// its output. Accepting the exit status alone would accept any package signed by any Apple
// Developer ID.
func (v *pkgVerifier) checkSignature(ctx context.Context, pkgPath string) error {
	out, err := v.run(ctx, pkgutilPath, "--check-signature", pkgPath)
	if err != nil {
		return fmt.Errorf("%s is not validly signed: %w (%s)", pkgPath, err, strings.TrimSpace(string(out)))
	}
	matches := teamIDRe.FindSubmatch(out)
	if matches == nil {
		return fmt.Errorf("could not read a Developer ID Installer team identifier from the signature of %s (%s)", pkgPath, strings.TrimSpace(string(out)))
	}
	if teamID := string(matches[1]); teamID != v.expectedTeamID {
		return fmt.Errorf("%s is signed by team %s, not %s", pkgPath, teamID, v.expectedTeamID)
	}
	return nil
}

// checkNotarization asks Gatekeeper whether it would allow the package to install.
//
// spctl consults the notarization ticket, whether it is stapled to the package or fetched from
// Apple, so this is the check that catches a signed but unnotarized or revoked build. It is last
// because it is the only check that can depend on the network.
func (v *pkgVerifier) checkNotarization(ctx context.Context, pkgPath string) error {
	out, err := v.run(ctx, spctlPath, "--assess", "--type", assessTarget, "-vv", pkgPath)
	if err != nil {
		return fmt.Errorf("%s is not accepted by Gatekeeper: %w (%s)", pkgPath, err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "accepted") {
		return fmt.Errorf("Gatekeeper did not accept %s (%s)", pkgPath, strings.TrimSpace(string(out)))
	}
	return nil
}

func (v *pkgVerifier) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if v.runner != nil {
		return v.runner(ctx, name, args...)
	}
	return telemetry.CommandContext(ctx, name, args...).CombinedOutput()
}
