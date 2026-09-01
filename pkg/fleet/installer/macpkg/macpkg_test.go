// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package macpkg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

// signedByDatadog is the shape pkgutil --check-signature reports for a package signed with the
// Datadog installer identity. Only the team identifier line is load-bearing.
const signedByDatadog = `Package "datadog-agent-7.99.0.pkg":
   Status: signed by a developer certificate issued by Apple (Development)
   Signed with a trusted timestamp on: 2026-01-01 00:00:00 +0000
   Certificate Chain:
    1. Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)
       Expires: 2030-01-01 00:00:00 +0000
    2. Developer ID Certification Authority
    3. Apple Root CA
`

const signedBySomeoneElse = `Package "evil.pkg":
   Status: signed by a developer certificate issued by Apple (Development)
   Certificate Chain:
    1. Developer ID Installer: Somebody Else, Inc. (AAAAAAAAAA)
    2. Developer ID Certification Authority
    3. Apple Root CA
`

const unsigned = `Package "datadog-agent-7.99.0.pkg":
   Status: no signature
`

const gatekeeperAccepted = `datadog-agent-7.99.0.pkg: accepted
source=Notarized Developer ID
`

const gatekeeperRejected = `datadog-agent-7.99.0.pkg: rejected
source=no usable signature
`

// recordingRunner returns a Runner that answers each command with a canned output and error, and
// records what it was asked to run.
func recordingRunner(t *testing.T, answers map[string]struct {
	out []byte
	err error
}, calls *[][]string) Runner {
	t.Helper()
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string{name}, args...))
		answer, ok := answers[name]
		if !ok {
			t.Fatalf("unexpected command %s %v", name, args)
		}
		return answer.out, answer.err
	}
}

func writePkg(t *testing.T, content string) *DownloadedPackage {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "datadog-agent.pkg")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	sum := sha256.Sum256([]byte(content))
	return &DownloadedPackage{Path: path, Version: "7.99.0", Digest: hex.EncodeToString(sum[:]), dir: dir}
}

func okAnswers() map[string]struct {
	out []byte
	err error
} {
	return map[string]struct {
		out []byte
		err error
	}{
		pkgutilPath: {out: []byte(signedByDatadog)},
		spctlPath:   {out: []byte(gatekeeperAccepted)},
	}
}

// TestVerifyAcceptsADatadogSignedNotarizedPackage is the one path that must succeed. Everything
// else in this file is a rejection, so without this the verifier could reject unconditionally and
// still pass.
func TestVerifyAcceptsADatadogSignedNotarizedPackage(t *testing.T) {
	pkg := writePkg(t, "payload")
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, okAnswers(), &calls)}

	require.NoError(t, verifier.Verify(context.Background(), pkg, pkg.Digest))

	// The order is part of the contract: the digest is checked before anything is shelled out
	// to, and the signature before Gatekeeper, which is the only check that can touch the
	// network.
	require.Len(t, calls, 2)
	assert.Equal(t, pkgutilPath, calls[0][0])
	assert.Contains(t, calls[0], "--check-signature")
	assert.Equal(t, spctlPath, calls[1][0])
}

// TestVerifyRejectsADigestMismatchBeforeShellingOut is the check that does not depend on the
// machine's trust state, so it has to come first: it answers "are these the bytes the catalog
// named", which a subverted Gatekeeper cannot affect.
func TestVerifyRejectsADigestMismatchBeforeShellingOut(t *testing.T) {
	pkg := writePkg(t, "payload")
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, okAnswers(), &calls)}

	err := verifier.Verify(context.Background(), pkg, strings.Repeat("0", 64))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest mismatch")
	assert.Empty(t, calls, "the verifier shelled out before the digest matched")
}

// TestVerifyRejectsAnotherTeamsSignature is the rejection that a naive implementation misses:
// pkgutil exits 0 for any package signed by any Apple Developer ID, so accepting the exit status
// alone accepts a validly signed package from someone else entirely.
func TestVerifyRejectsAnotherTeamsSignature(t *testing.T) {
	pkg := writePkg(t, "payload")
	answers := okAnswers()
	answers[pkgutilPath] = struct {
		out []byte
		err error
	}{out: []byte(signedBySomeoneElse)}
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, answers, &calls)}

	err := verifier.Verify(context.Background(), pkg, pkg.Digest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AAAAAAAAAA")
	assert.Len(t, calls, 1, "the verifier went on to Gatekeeper after the team identifier did not match")
}

// TestVerifyRejectsAnUnsignedPackage covers the case where pkgutil exits non-zero.
func TestVerifyRejectsAnUnsignedPackage(t *testing.T) {
	pkg := writePkg(t, "payload")
	answers := okAnswers()
	answers[pkgutilPath] = struct {
		out []byte
		err error
	}{out: []byte(unsigned), err: errors.New("exit status 1")}
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, answers, &calls)}

	err := verifier.Verify(context.Background(), pkg, pkg.Digest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not validly signed")
}

// TestVerifyRejectsASignatureWithNoReadableTeamID is the fail-closed case: if the output cannot be
// parsed, the team identifier is unknown, and unknown is not the expected one.
func TestVerifyRejectsASignatureWithNoReadableTeamID(t *testing.T) {
	pkg := writePkg(t, "payload")
	answers := okAnswers()
	answers[pkgutilPath] = struct {
		out []byte
		err error
	}{out: []byte("Package \"x.pkg\":\n   Status: signed\n")}
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, answers, &calls)}

	err := verifier.Verify(context.Background(), pkg, pkg.Digest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team identifier")
}

// TestVerifyRejectsAPackageGatekeeperDoesNotAccept is the notarization check: a signed but
// unnotarized or revoked build gets this far and no further.
func TestVerifyRejectsAPackageGatekeeperDoesNotAccept(t *testing.T) {
	pkg := writePkg(t, "payload")
	for name, answer := range map[string]struct {
		out []byte
		err error
	}{
		"non-zero exit":              {out: []byte(gatekeeperRejected), err: errors.New("exit status 3")},
		"exit zero but not accepted": {out: []byte("something else entirely")},
	} {
		t.Run(name, func(t *testing.T) {
			answers := okAnswers()
			answers[spctlPath] = answer
			var calls [][]string
			verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, answers, &calls)}
			assert.Error(t, verifier.Verify(context.Background(), pkg, pkg.Digest))
		})
	}
}

// TestVerifyRejectsAnEmptyOrAbsentFile covers the degenerate inputs, which reach the verifier when
// a download failed in a way that still produced a file.
func TestVerifyRejectsAnEmptyOrAbsentFile(t *testing.T) {
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		t.Fatalf("the verifier shelled out to %s for a file it should have rejected", name)
		return nil, nil
	}}

	assert.Error(t, verifier.Verify(context.Background(), nil, ""))
	assert.Error(t, verifier.Verify(context.Background(), &DownloadedPackage{Path: filepath.Join(t.TempDir(), "absent.pkg")}, ""))
	assert.Error(t, verifier.Verify(context.Background(), writePkg(t, ""), ""))
}

// TestVerifySkipsTheDigestWhenTheCatalogCarriesNone records the deliberate decision: a catalog
// entry with no digest is a gap in the catalog, not a reason to refuse every update, so the
// signature and notarization checks still run and still decide.
func TestVerifySkipsTheDigestWhenTheCatalogCarriesNone(t *testing.T) {
	pkg := writePkg(t, "payload")
	var calls [][]string
	verifier := &pkgVerifier{expectedTeamID: DatadogTeamID, runner: recordingRunner(t, okAnswers(), &calls)}

	require.NoError(t, verifier.Verify(context.Background(), pkg, ""))
	assert.Len(t, calls, 2)
}

// --- SystemInstaller ---

// TestSystemInstallerTargetsTheVolume pins the one argument that cannot be got wrong: -target is a
// volume, and the payload's destination is baked into the package. Passing a directory would
// silently install nothing where it is wanted.
func TestSystemInstallerTargetsTheVolume(t *testing.T) {
	var calls [][]string
	installer := SystemInstaller{Runner: func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte("installer: The install was successful."), nil
	}}

	require.NoError(t, installer.Install(context.Background(), "/tmp/x.pkg"))
	require.Len(t, calls, 1)
	assert.Equal(t, []string{systemInstallerPath, "-pkg", "/tmp/x.pkg", "-target", "/"}, calls[0])
}

// TestSystemInstallerSurfacesTheInstallersOwnOutput matters because the system installer's
// description of a failure is only in its output, never in its exit status.
func TestSystemInstallerSurfacesTheInstallersOwnOutput(t *testing.T) {
	installer := SystemInstaller{Runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("installer: Error - the package is not compatible"), errors.New("exit status 1")
	}}

	err := installer.Install(context.Background(), "/tmp/x.pkg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not compatible")
}

// --- PayloadCheck ---

func completeTree(t *testing.T, root string, version string) {
	t.Helper()
	for _, required := range requiredPayloadPaths {
		path := filepath.Join(root, version, required)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0755))
	}
}

// TestPayloadCheckAcceptsACompleteTree is the gate opening.
func TestPayloadCheckAcceptsACompleteTree(t *testing.T) {
	root := t.TempDir()
	completeTree(t, root, "7.99.0")

	complete, err := PayloadCheck{Root: root}.Complete("7.99.0")
	require.NoError(t, err)
	assert.True(t, complete)
}

// TestPayloadCheckRejectsATreeMissingExactlyOnePath is the case the check exists for. A tree
// missing one binary is the outcome of a system installer that exited zero having written a
// partial payload, and it is indistinguishable from a good tree by any cheaper test.
func TestPayloadCheckRejectsATreeMissingExactlyOnePath(t *testing.T) {
	for _, missing := range requiredPayloadPaths {
		t.Run(missing, func(t *testing.T) {
			root := t.TempDir()
			completeTree(t, root, "7.99.0")
			require.NoError(t, os.Remove(filepath.Join(root, "7.99.0", missing)))

			complete, err := PayloadCheck{Root: root}.Complete("7.99.0")
			require.NoError(t, err, "a missing path is an answer, not an error")
			assert.False(t, complete, "the check accepted a tree with no %s", missing)
		})
	}
}

// TestPayloadCheckRejectsADanglingSymlink covers the difference between Stat and Lstat: launchd
// follows symlinks, so a link to a missing target is as incomplete as an absent file.
func TestPayloadCheckRejectsADanglingSymlink(t *testing.T) {
	root := t.TempDir()
	completeTree(t, root, "7.99.0")
	target := filepath.Join(root, "7.99.0", requiredPayloadPaths[0])
	require.NoError(t, os.Remove(target))
	require.NoError(t, os.Symlink(filepath.Join(root, "nowhere"), target))

	complete, err := PayloadCheck{Root: root}.Complete("7.99.0")
	require.NoError(t, err)
	assert.False(t, complete)
}

// TestPayloadCheckRejectsAnAbsentOrUnnamedVersion covers the degenerate inputs.
func TestPayloadCheckRejectsAnAbsentOrUnnamedVersion(t *testing.T) {
	check := PayloadCheck{Root: t.TempDir()}

	complete, err := check.Complete("7.99.0")
	require.NoError(t, err)
	assert.False(t, complete)

	_, err = check.Complete("")
	assert.Error(t, err)
}

// TestRequiredPayloadPathsCoverEveryJob is the drift guard. The required-path list is only correct
// if it is the union of what the job definitions resolve through, and nothing in the type system
// says so -- a job added with a new program path would otherwise pass the completeness check
// against a tree that cannot start it.
func TestRequiredPayloadPathsCoverEveryJob(t *testing.T) {
	labels, err := embedded.LaunchdJobs(embedded.LaunchdExperiment)
	require.NoError(t, err)
	require.NotEmpty(t, labels)

	const poolPrefix = "/opt/datadog-packages/datadog-agent/experiment/"
	required := map[string]bool{}
	for _, path := range requiredPayloadPaths {
		required[path] = true
	}

	for _, label := range labels {
		definition, err := embedded.GetLaunchdJob(label, embedded.LaunchdExperiment)
		require.NoError(t, err)
		program := firstProgramArgument(t, definition)
		require.True(t, strings.HasPrefix(program, poolPrefix),
			"the %s-exp job runs %s, which is not in the pool, so the completeness check cannot cover it", label, program)
		relative := strings.TrimPrefix(program, poolPrefix)
		assert.True(t, required[relative],
			"the %s-exp job runs %s, which requiredPayloadPaths does not list", label, relative)
	}

	// And the other direction, so the list cannot quietly accumulate paths nothing depends on.
	// It is not a plain set equality: the installer binary is required without being run by any
	// -exp job, because the installer daemon is deliberately left running through an experiment
	// and reaches its binary through the façade, which resolves into whichever version is
	// stable. It still has to be in every version's payload, or promoting a version would leave
	// the façade pointing at a binary that is not there.
	notRunByAnExperimentJob := map[string]string{
		filepath.Join("embedded", "bin", "installer"): "reached through the façade by the installer daemon, which is never stopped for an experiment",
	}
	programs := map[string]bool{}
	for _, label := range labels {
		definition, err := embedded.GetLaunchdJob(label, embedded.LaunchdExperiment)
		require.NoError(t, err)
		programs[strings.TrimPrefix(firstProgramArgument(t, definition), poolPrefix)] = true
	}
	for _, path := range requiredPayloadPaths {
		if reason, ok := notRunByAnExperimentJob[path]; ok {
			t.Logf("%s is required but run by no -exp job: %s", path, reason)
			continue
		}
		assert.True(t, programs[path], "requiredPayloadPaths lists %s, which no -exp job runs and which is not listed as an exception", path)
	}
}

// firstProgramArgument pulls ProgramArguments[0] out of a plist with a token-stream walk. A full
// plist parser is not worth a dependency for one key.
func firstProgramArgument(t *testing.T, content []byte) string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	var key string
	inProgramArguments := false
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "key":
				var value string
				require.NoError(t, decoder.DecodeElement(&value, &element))
				key = value
				if key == "ProgramArguments" {
					inProgramArguments = true
				}
			case "array":
				if inProgramArguments {
					depth++
				}
			case "string":
				var value string
				require.NoError(t, decoder.DecodeElement(&value, &element))
				if inProgramArguments && depth > 0 {
					return value
				}
			}
		}
	}
	t.Fatalf("no ProgramArguments found in %s", content)
	return ""
}

// --- Downloader ---

// TestDownloadWritesTheBytesAndReportsTheirDigest is the success path, and pins that the digest is
// of what landed rather than of what was promised.
func TestDownloadWritesTheBytesAndReportsTheirDigest(t *testing.T) {
	body := "the payload"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/datadog-agent-7.99.0.pkg", r.URL.Path)
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	downloader := &Downloader{client: server.Client(), env: &env.Env{}, tmpDir: t.TempDir()}
	pkg, err := downloader.Download(context.Background(), server.URL, "7.99.0")
	require.NoError(t, err)
	defer pkg.Cleanup()

	content, err := os.ReadFile(pkg.Path)
	require.NoError(t, err)
	assert.Equal(t, body, string(content))
	sum := sha256.Sum256([]byte(body))
	assert.Equal(t, hex.EncodeToString(sum[:]), pkg.Digest)
}

// TestDownloadLeavesNothingBehindWhenItFails is what makes verification-on-disk acceptable: the
// file is in scratch, is named by nothing, and is gone on any failure.
func TestDownloadLeavesNothingBehindWhenItFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := &Downloader{client: server.Client(), env: &env.Env{}, tmpDir: tmpDir}
	_, err := downloader.Download(context.Background(), server.URL+"/x.pkg", "7.99.0")
	require.Error(t, err)

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed download left scratch behind")
}

// TestDownloadDoesNotRetryAClientError separates a 404, which will never succeed, from a 503,
// which might. Retrying a 404 three times only delays the failure report.
func TestDownloadDoesNotRetryAClientError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	downloader := &Downloader{client: server.Client(), env: &env.Env{}, tmpDir: t.TempDir()}
	_, err := downloader.Download(context.Background(), server.URL+"/x.pkg", "7.99.0")
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

// TestDownloadRetriesAServerError is the other half.
func TestDownloadRetriesAServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < networkRetries {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "payload")
	}))
	defer server.Close()

	downloader := &Downloader{client: server.Client(), env: &env.Env{}, tmpDir: t.TempDir()}
	pkg, err := downloader.Download(context.Background(), server.URL+"/x.pkg", "7.99.0")
	require.NoError(t, err)
	defer pkg.Cleanup()
	assert.Equal(t, networkRetries, attempts)
}

// TestResolveURLFormsAndFailures documents which inputs the downloader can act on. The important
// case is the last one: with nothing to resolve against it refuses rather than guessing a URL,
// because a guessed URL that happens to exist is the worst outcome available.
func TestResolveURLFormsAndFailures(t *testing.T) {
	withMirror := &Downloader{env: &env.Env{Mirror: "https://mirror.example/agent/"}}
	withoutMirror := &Downloader{env: &env.Env{}}

	resolved, err := withoutMirror.resolveURL("https://example.com/a/datadog-agent-7.99.0.pkg", "7.99.0")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a/datadog-agent-7.99.0.pkg", resolved, "an explicit .pkg URL must be used as given")

	resolved, err = withoutMirror.resolveURL("https://example.com/a", "7.99.0")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a/datadog-agent-7.99.0.pkg", resolved)

	resolved, err = withMirror.resolveURL("oci://install.datadoghq.com/agent-package:7.99.0", "7.99.0")
	require.NoError(t, err)
	assert.Equal(t, "https://mirror.example/agent/datadog-agent-7.99.0.pkg", resolved)

	_, err = withoutMirror.resolveURL("oci://install.datadoghq.com/agent-package:7.99.0", "7.99.0")
	assert.Error(t, err, "the downloader guessed a URL with nothing to resolve against")

	_, err = withMirror.resolveURL("https://example.com/a", "")
	assert.Error(t, err, "the downloader accepted an unnamed version")
}
