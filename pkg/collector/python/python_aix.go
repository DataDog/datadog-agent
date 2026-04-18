// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix && python

package python

// Force libpython.so into the agent binary's XCOFF startup-load chain.
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
// Without this file, libpython.so is loaded only via dlopen, transitively
// through libdatadog-agent-three.so when rtloader initialises Python.  A
// dlopen-loaded library does NOT add its symbols to the global symbol table.
// As a result every Python C extension fails with:
//
//	ImportError: Symbol PyXxx is not exported from dependent module agent.
//
// Fix
// ---
// The init() function below calls C.Py_IsInitialized() once at process startup.
// Go's CGO layer sees this Go→C call and emits a //go:cgo_import_dynamic
// directive that places libpython.a(shr_64.o) in the agent binary's XCOFF
// loader section.  Consequently libpython.so is loaded at process startup,
// before any Python C extension module is imported, and all Python API
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
// The -bE:python.exp flag (passed via CGO_LDFLAGS at build time, NOT via
// #cgo LDFLAGS which would be rejected by Go's CGO security filter) causes the
// linker to add all Python API symbols to the agent binary's own EXP
// (export) table.  Extension modules have Python API symbols with IMPid="."
// in their XCOFF, meaning "find in the main program's EXP table".  Without
// -bE, those symbols are not exported from the agent and every extension fails:
//
//	ImportError: Symbol PyXxx is not exported from dependent module agent.
//
// With -bE:python.exp, the agent exports the symbols and extensions load cleanly.

/*
// Forward declaration — we do not include Python.h to avoid pulling in
// the entire CPython header tree; Py_IsInitialized has a stable ABI.
// -lpython3 is supplied via CGO_LDFLAGS by get_build_flags() in
// tasks/libs/common/utils.py, using the correct embedded_path regardless
// of where the source tree is located.
extern int Py_IsInitialized(void);
*/
import "C"

func init() {
	// Call Py_IsInitialized() to create a live Go→C reference.  This forces
	// the CGO linker to emit a //go:cgo_import_dynamic for Py_IsInitialized,
	// placing libpython.a(shr_64.o) in the XCOFF startup-load chain.
	// Python is not yet started here so the call always returns 0; we discard
	// the result.  There are no side effects.
	_ = C.Py_IsInitialized()
}
