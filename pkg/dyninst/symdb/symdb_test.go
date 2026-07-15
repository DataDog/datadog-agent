// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package symdb_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestSymDB(t *testing.T) {
	cfgs, err := testprogs.GetCommonConfigs()
	require.NoError(t, err)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			binaryPath, err := testprogs.GetBinary("simple", cfg)
			require.NoError(t, err)
			t.Logf("exploring binary: %s", binaryPath)
			symbols, err := symdb.ExtractSymbols(
				binaryPath,
				object.NewInMemoryLoader(),
				symdb.ExtractOptions{
					Scope: symdb.ExtractScopeAllSymbols,
				})
			require.NoError(t, err, "failed to extract symbols from %s", binaryPath)
			require.NotEmpty(t, symbols.Packages)

			// Look at a couple of symbols as a smoke test.
			pkg, ok := findPackage(symbols, "main")
			require.Truef(t, ok, "package 'main' not found in %s", binaryPath)
			fn, ok := findFunction(pkg, "stringArg")
			require.Truef(t, ok, "function 'stringArg' not found in package 'main' in %s", binaryPath)
			require.NotZero(t, fn.StartLine)
			require.NotZero(t, fn.EndLine)
			require.Less(t, fn.StartLine, fn.EndLine)
			v, ok := findVariable(fn.Scope, "s")
			require.Truef(t, ok, "variable 's' not found in function 'stringArg' in package 'main' in %s", binaryPath)
			require.True(t, v.FunctionArgument)
			require.NotZero(t, v.DeclLine)
			require.NotEmpty(t, v.AvailableLineRanges)
		})
	}
}

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the snapshot files")

const snapshotDir = "testdata/snapshot"

// TestSymDBSnapshot exercises the full upload pipeline — the same one used
// by the production uploader at pkg/dyninst/module/symdb.go. It drives
// uploader.RunUploadLoop against an in-process httptest.Server with a
// flush threshold of 1 byte (so every AddPackage triggers an immediate
// Flush) and renders each captured wire-form package through
// symdbprinter.SerializeScope into a golden file. Each captured batch is
// written as a distinct "=== yield N [final=T/F] ===" section.
func TestSymDBSnapshot(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	progs := testprogs.MustGetPrograms(t)
	sem := dyninsttest.MakeSemaphore()
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			t.Parallel()
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					t.Parallel()
					defer sem.Acquire()()
					binaryPath := testprogs.MustGetBinary(t, prog, cfg)
					t.Logf("exploring binary: %s", binaryPath)
					it, err := symdb.PackagesIterator(
						binaryPath,
						object.NewInMemoryLoader(),
						symdb.ExtractOptions{
							Scope: symdb.ExtractScopeMainModuleOnly,
						})
					require.NoError(t, err, "failed to open iterator on %s", binaryPath)

					cs := newCapturingServer(t)
					defer cs.close()
					enc, err := uploader.NewBatchEncoder(
						cs.url(), "snapshot-svc", "snapshot-ver", "snapshot-rid",
						uuid.New(), nil, nil,
					)
					require.NoError(t, err)
					defer func() { _ = enc.Close() }()

					// flushThreshold = 1 forces a Flush after every
					// AddPackage (the encoder Flush()es gzip after each
					// scope, so Size() reflects post-compression bytes
					// >= 1 immediately). This makes batches align 1:1
					// with iterator yields.
					stats, err := uploader.RunUploadLoop(
						context.Background(), enc, it, "", 1,
					)
					require.NoError(t, err)

					captured := cs.captured()
					require.Equal(t, stats.Packages, len(captured),
						"expected one captured batch per uploaded package")
					require.NotZero(t, stats.Packages,
						"expected at least one package yield")

					var sb strings.Builder
					for i, batch := range captured {
						require.Lenf(t, batch.scopes, 1,
							"expected exactly one scope per batch (batch %d)", i+1)
						fmt.Fprintf(&sb, "=== yield %d [final=%t] ===\n", i+1, batch.final)
						require.NoError(t, symdbprinter.SerializeScope(&sb, batch.scopes[0]))
					}

					out := sb.String()
					outputFile := path.Join(snapshotDir, prog+".streaming."+cfg.String()+".out")
					if *rewrite {
						tmpFile, err := os.CreateTemp(snapshotDir, ".out")
						require.NoError(t, err)
						name := tmpFile.Name()
						defer func() { _ = os.Remove(name) }()
						_, err = tmpFile.WriteString(out)
						require.NoError(t, err)
						require.NoError(t, tmpFile.Close())
						require.NoError(t, os.Rename(name, outputFile))
					} else {
						expected, err := os.ReadFile(outputFile)
						require.NoError(t, err)
						require.Equal(t, string(expected), out)
					}
				})
			}
		})
	}
}

func findPackage(s symdb.Symbols, pkgName string) (symdb.Package, bool) {
	for _, pkg := range s.Packages {
		if pkg.Name == pkgName {
			return pkg, true
		}
	}
	return symdb.Package{}, false
}

func findFunction(pkg symdb.Package, fnName string) (symdb.Function, bool) {
	for _, fn := range pkg.Functions {
		if fn.Name == fnName {
			return fn, true
		}
	}
	return symdb.Function{}, false
}

func findVariable(scope symdb.Scope, varName string) (symdb.Variable, bool) {
	for _, variable := range scope.Variables {
		if variable.Name == varName {
			return variable, true
		}
	}
	return symdb.Variable{}, false
}

// capturingServer is a small in-process HTTP server that decodes each
// multipart SymDB upload body into a capturedBatch. Used by
// TestSymDBSnapshot to drive the real upload pipeline against an in-memory
// sink and golden-test what bytes go on the wire.
type capturingServer struct {
	t      *testing.T
	server *httptest.Server

	mu      sync.Mutex
	batches []capturedBatch
}

type capturedBatch struct {
	final  bool
	scopes []uploader.Scope
}

func newCapturingServer(t *testing.T) *capturingServer {
	cs := &capturingServer{t: t}
	cs.server = httptest.NewServer(http.HandlerFunc(cs.handle))
	return cs
}

func (cs *capturingServer) url() string { return cs.server.URL }
func (cs *capturingServer) close()      { cs.server.Close() }

func (cs *capturingServer) captured() []capturedBatch {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]capturedBatch, len(cs.batches))
	copy(out, cs.batches)
	return out
}

func (cs *capturingServer) handle(w http.ResponseWriter, r *http.Request) {
	batch, err := decodeUpload(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("decode upload: %v", err), http.StatusInternalServerError)
		return
	}
	cs.mu.Lock()
	cs.batches = append(cs.batches, batch)
	cs.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// uploadEnvelope mirrors the JSON envelope BatchEncoder writes. Only the
// fields the snapshot test reads back are listed; everything else is
// ignored by encoding/json.
type uploadEnvelope struct {
	Final  bool             `json:"final"`
	Scopes []uploader.Scope `json:"scopes"`
}

func decodeUpload(r *http.Request) (capturedBatch, error) {
	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return capturedBatch{}, fmt.Errorf("parse content-type: %w", err)
	}
	if contentType != "multipart/form-data" {
		return capturedBatch{}, fmt.Errorf("unexpected content-type %q", contentType)
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return capturedBatch{}, fmt.Errorf("read multipart: %w", err)
		}
		if part.FormName() != "file" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		gz, err := gzip.NewReader(part)
		if err != nil {
			return capturedBatch{}, fmt.Errorf("gunzip file part: %w", err)
		}
		raw, err := io.ReadAll(gz)
		_ = gz.Close()
		if err != nil {
			return capturedBatch{}, fmt.Errorf("read gunzipped: %w", err)
		}
		var env uploadEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return capturedBatch{}, fmt.Errorf("unmarshal envelope: %w", err)
		}
		return capturedBatch{final: env.Final, scopes: env.Scopes}, nil
	}
	return capturedBatch{}, errors.New("no file part in multipart request")
}
