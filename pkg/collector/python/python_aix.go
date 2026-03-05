// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix && python

package python

// Force libpython3.13.so into the agent binary's XCOFF startup-load chain.
//
// Background
// ----------
// Python C extension modules (lib-dynload/_decimal.so, etc.) carry Python API
// symbols (PyArg_ParseTuple, PyBool_FromLong, …) with no explicit library
// dependency in their XCOFF loader section (IMPid = ".").  On AIX, such
// unresolved symbols are looked up in the process-global symbol table, which
// is populated only by libraries that appear in the XCOFF startup-load chain
// (i.e. in the XCOFF loader section of the main executable or its startup-
// linked dependencies).
//
// Without this file, libpython3.13.so is loaded only via dlopen, transitively
// through libdatadog-agent-three.so when rtloader initialises Python.  A
// dlopen-loaded library does NOT add its symbols to the global symbol table.
// As a result every Python C extension fails with:
//   ImportError: Symbol PyXxx is not exported from dependent module agent.
//
// Fix
// ---
// The init() function below calls C.Py_IsInitialized() once at process startup.
// Go's CGO layer sees this Go→C call and emits a //go:cgo_import_dynamic
// directive that places libpython3.13.a(shr_64.o) in the agent binary's XCOFF
// loader section.  Consequently libpython3.13.so is loaded at process startup,
// before any Python C extension module is imported, and all ~1680 Python API
// symbols enter the global symbol table where the extension modules can find them.
//
// Note: a C-internal reference (static void* holding &Py_IsInitialized) is NOT
// sufficient — Go's linker only generates XCOFF import entries for symbols that
// are called from the Go side through CGO.  A direct Go→C call is required.
//
// Py_IsInitialized() returns 0 when called here (Python not yet started) and
// has no side effects — it is a safe, idempotent read of an internal flag.
//
// Part 2 — Exporting Python API symbols from the agent binary
// The -bE:python.exp flag (passed via CGO_LDFLAGS env at build time, NOT via
// #cgo LDFLAGS which would be rejected by Go's CGO security filter) causes the
// linker to add all ~2762 Python API symbols to the agent binary's own EXP
// (export) table.  Extension modules have Python API symbols with IMPid="."
// in their XCOFF, meaning "find in the main program's EXP table".  Without
// -bE, those symbols are not exported from the agent and every extension fails:
//   ImportError: Symbol PyXxx is not exported from dependent module agent.
// With -bE:python.exp, the agent exports the symbols and extensions load cleanly.
// See packaging/aix/stages/04-agent.sh for how -bE is passed at build time.
//
// The -L path resolves to /opt/datadog-agent/embedded/lib at build time
// (${SRCDIR}/../../../embedded is a symlink to the staging tree created by
// Stage 02).  At runtime the same directory exists as a real directory
// installed by installp, and LIBPATH in the agent-svc wrapper includes it.

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../embedded/lib -lpython3.13

// Forward declaration — we do not include Python.h to avoid pulling in
// the entire CPython header tree; Py_IsInitialized has a stable ABI.
extern int Py_IsInitialized(void);
*/
import "C"

func init() {
	// Call Py_IsInitialized() to create a live Go→C reference.  This forces
	// the CGO linker to emit a //go:cgo_import_dynamic for Py_IsInitialized,
	// placing libpython3.13.a(shr_64.o) in the XCOFF startup-load chain.
	// Python is not yet started here so the call always returns 0; we discard
	// the result.  There are no side effects.
	_ = C.Py_IsInitialized()
}
