// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

/*
 * three_subinterp.cpp — Sub-interpreter lifecycle management for RTLoader
 * =======================================================================
 *
 * This file is ONLY compiled when -DENABLE_SUBINTERPRETERS=ON is passed to
 * CMake, which also defines the RTLOADER_HAS_SUBINTERPRETERS preprocessor
 * macro. It requires Python 3.14+.
 *
 * Purpose:
 *   Each Python integration check instance runs in its own sub-interpreter
 *   for isolation. A misbehaving check (memory leak, corrupted globals,
 *   monkey-patching) cannot affect other checks. This file provides the
 *   helper functions that create, look up, and destroy sub-interpreters,
 *   as well as the assignment policy that decides which interpreter a
 *   check should run in.
 *
 * Design overview:
 *   - _createSubInterpreter(): Creates a new sub-interpreter with its own
 *     GIL (PyInterpreterConfig_OWN_GIL) and copies sys.path from the main
 *     interpreter so that check modules can be found.
 *
 *   - _destroySubInterpreter(tstate): Destroys a sub-interpreter. Must be
 *     called with the sub-interpreter's tstate currently active (swapped in).
 *
 *   - _assignInterpreter(module_name): Policy function that decides which
 *     interpreter a check should use. Currently implements 1:1 (one new
 *     interpreter per check). To change the policy (e.g., per-type sharing),
 *     modify ONLY this function.
 *
 *   - _lookupCheckInterp(check): Finds which sub-interpreter a check instance
 *     belongs to, using the check's PyObject pointer as the lookup key.
 *
 *   - _removeCheckInterp(check): Removes a check from the mapping and returns
 *     its sub-interpreter tstate. Does NOT destroy the interpreter — the
 *     caller is responsible for that (see decref in three.cpp for ordering).
 *     Reason: the caller needs to swap into the sub-interpreter, Py_DECREF
 *     the check object, and THEN destroy — all in one swap. If we destroyed
 *     here, the check object would be invalid before the caller could release
 *     it (use-after-free).
 *
 * Thread safety:
 *   _checkToInterp is a C++ unordered_map accessed by multiple goroutines
 *   (via runCheck, decref, etc.). All accesses are protected by
 *   _checkToInterpMutex. This is a C++ mutex for a C++ data structure —
 *   it has nothing to do with the Python GIL.
 *
 * GIL ownership with OWN_GIL and Py_NewInterpreterFromConfig
 * (used in _createSubInterpreter):
 *   From the Python docs:
 *     "Upon success, tstate_p will be set to the first thread state created
 *      in the new sub-interpreter. This thread state is attached."
 *     "If the sub-interpreter is created with its own GIL then the attached
 *      thread state of the calling interpreter will be detached. When the
 *      function returns, the new interpreter's thread state will be attached
 *      to the current thread and the previous interpreter's attached thread
 *      state will remain detached."
 *
 *   "Attached" means this thread holds that interpreter's GIL and the
 *   thread state is active. "Detached" means this thread does NOT hold
 *   that interpreter's GIL.
 *
 *   So after Py_NewInterpreterFromConfig with OWN_GIL succeeds:
 *     - Main interpreter:     DETACHED (GIL released, tstate inactive)
 *     - New sub-interpreter:  ATTACHED (GIL held, tstate active)
 *   The function swaps us: we went in holding main's GIL, we come out
 *   holding the new sub-interpreter's GIL. No manual GIL acquire needed.
 *
 *   On failure, tstate_p is set to NULL and the calling interpreter's
 *   tstate "might be detached" — so we must re-attach it explicitly
 *   via PyEval_RestoreThread(main_tstate).
 *
 * GIL swap pattern for EXISTING sub-interpreters
 * (used by runCheck, cancelCheck, etc. in three.cpp):
 *   These methods need to switch from main to an already-created sub-
 *   interpreter. There is no Py_NewInterpreterFromConfig call here — the
 *   sub-interpreter already exists. So we must manually release/acquire
 *   GILs using PyEval_SaveThread and PyEval_RestoreThread.
 *
 *   With OWN_GIL, each interpreter has its own GIL, so PyThreadState_Swap
 *   alone is NOT sufficient — it only changes the thread state pointer
 *   without releasing/acquiring GILs.
 *
 *   The correct swap sequence:
 *     1. main_tstate = PyEval_SaveThread()   — release main GIL, detach
 *     2. PyEval_RestoreThread(sub_tstate)    — acquire sub-interp GIL, attach
 *     3. ... do work in sub-interpreter ...
 *     4. PyEval_SaveThread()                 — release sub-interp GIL, detach
 *     5. PyEval_RestoreThread(main_tstate)   — re-acquire main GIL, attach
 */

#include "three.h"

#include <sstream>

/*
 * _createSubInterpreter
 * ---------------------
 * Creates a new Python sub-interpreter with per-interpreter GIL.
 *
 * IMPORTANT: This function must be called while the main interpreter's GIL
 * is held (which is guaranteed because Go's stickyLock calls PyGILState_Ensure
 * before any C++ method is entered).
 *
 * Steps:
 *   1. Save the main interpreter's thread state (so we can re-attach later).
 *   2. Configure PyInterpreterConfig for full isolation:
 *      - use_main_obmalloc = 0: separate memory allocator per interpreter
 *        (REQUIRED when using own GIL — Python enforces this)
 *      - allow_fork = 0, allow_exec = 0, allow_threads = 1: standard safety
 *        settings. fork() from a sub-interpreter is dangerous, but threads
 *        are needed for I/O.
 *      - allow_daemon_threads = 0: daemon threads in sub-interpreters can
 *        cause crashes during shutdown.
 *      - check_multi_interp_extensions = 1: only allow C extensions that
 *        declare sub-interpreter support via Py_MOD_PER_INTERPRETER_GIL_SUPPORTED.
 *        This is what makes our multi-phase init work in Phase 1 necessary.
 *      - gil = PyInterpreterConfig_OWN_GIL: each sub-interpreter gets its own
 *        GIL, enabling true parallelism between checks.
 *   3. Call Py_NewInterpreterFromConfig to create the sub-interpreter.
 *      With OWN_GIL, this function DETACHES us from main (releases main GIL)
 *      and ATTACHES us to the new sub-interpreter (acquires its GIL). No
 *      manual GIL acquire is needed — the function does the swap for us.
 *   4. Copy sys.path entries from _pythonPaths into the new interpreter's
 *      sys.path, so that check modules (datadog_checks.*) can be found.
 *      We're in the sub-interpreter's context here (attached by step 3).
 *   5. Detach from sub-interpreter (PyEval_SaveThread — releases sub-interp
 *      GIL) and re-attach to main (PyEval_RestoreThread — acquires main GIL)
 *      so the caller returns to the main interpreter context.
 *
 * Returns:
 *   The new sub-interpreter's PyThreadState on success, or NULL on failure
 *   (with an error set via setError). On both success and failure, this
 *   function returns with the main interpreter's GIL held (re-attached).
 */
PyThreadState *Three::_createSubInterpreter()
{
    // Step 1: Save the main interpreter's thread state. PyThreadState_Get()
    // returns the currently attached thread state — this is main's tstate
    // since Go's stickyLock ensures we enter C++ with the main GIL held.
    // We save it so we can re-attach to main after creating the sub-interpreter.
    PyThreadState *main_tstate = PyThreadState_Get();

    // Step 2: Configure the sub-interpreter for maximum isolation.
    // PyInterpreterConfig is a plain C struct (no cleanup needed).
    PyInterpreterConfig config = {
        .use_main_obmalloc = 0,              // separate allocator (required for OWN_GIL)
        .allow_fork = 0,                     // fork from sub-interp is dangerous
        .allow_exec = 0,                     // exec replaces process — not useful here
        .allow_threads = 1,                  // needed for I/O in checks
        .allow_daemon_threads = 0,           // daemon threads crash on shutdown
        .check_multi_interp_extensions = 1,  // enforce Py_MOD_PER_INTERPRETER_GIL_SUPPORTED
        .gil = PyInterpreterConfig_OWN_GIL,  // each check gets its own GIL
    };

    // Step 3: Create the sub-interpreter.
    // On success, tstate_new is the new interpreter's thread state, and
    // the current thread is now attached to it (Python does the swap).
    // On failure, tstate_new is NULL and status contains the error.
    PyThreadState *tstate_new = NULL;
    PyStatus status = Py_NewInterpreterFromConfig(&tstate_new, &config);

    if (PyStatus_Exception(status)) {
        // Creation failed. Per the Python docs, the calling interpreter's
        // tstate "might be detached" on failure — so we must explicitly
        // re-attach to main via PyEval_RestoreThread (acquires main GIL).
        setError("failed to create sub-interpreter"
                 + (status.err_msg ? ": " + std::string(status.err_msg) : ""));
        PyEval_RestoreThread(main_tstate);
        return NULL;
    }

    if (tstate_new == NULL) {
        // Shouldn't happen if status is OK, but defensive check.
        // Same re-attach logic as above.
        setError("Py_NewInterpreterFromConfig returned NULL without error");
        PyEval_RestoreThread(main_tstate);
        return NULL;
    }

    // Step 4: Copy sys.path from our stored _pythonPaths into the new
    // interpreter's sys.path. Each sub-interpreter starts with a default
    // sys.path that may not include the paths where check packages live.
    //
    // We are currently attached to the new sub-interpreter (step 3
    // attached us and gave us its GIL), so PySys_GetObject("path")
    // returns the NEW interpreter's sys.path.
    if (!_pythonPaths.empty()) {
        char pathchr[] = "path";
        PyObject *path = PySys_GetObject(pathchr);  // borrowed reference
        if (path == NULL) {
            setError("sub-interpreter: could not access sys.path");
            // Destroy the sub-interpreter. Py_EndInterpreter requires us to
            // be attached (which we are). It destroys the interpreter and
            // releases its GIL. After this call, tstate_new is invalid, no
            // thread state is attached, and no GIL is held — we're just
            // running C++ code.
            // PyEval_RestoreThread then acquires the main GIL and attaches
            // main_tstate. We use PyEval_RestoreThread (not PyThreadState_Swap)
            // because with OWN_GIL each interpreter has its own GIL — we need
            // to actually acquire main's GIL, not just set the pointer.
            Py_EndInterpreter(tstate_new);
            PyEval_RestoreThread(main_tstate);
            return NULL;
        }

        // Insert paths at the BEGINNING of sys.path (index 0) so they
        // take priority over site-packages. Without this, the sub-interpreter
        // might pick up a real installed package instead of test stubs or
        // the agent's bundled packages. We insert in reverse order so the
        // original order is preserved at the front of the list.
        for (int i = (int)_pythonPaths.size() - 1; i >= 0; --i) {
            PyObject *p = PyUnicode_FromString(_pythonPaths[i].c_str());
            if (p == NULL) {
                setError("sub-interpreter: could not create path string: "
                         + _fetchPythonError());
                Py_EndInterpreter(tstate_new);
                PyEval_RestoreThread(main_tstate);
                return NULL;
            }
            int rc = PyList_Insert(path, 0, p);
            // PyList_Insert creates its own reference to p, so we must
            // release ours regardless of success or failure.
            Py_DECREF(p);
            if (rc == -1) {
                setError("sub-interpreter: could not insert into sys.path: "
                         + _fetchPythonError());
                Py_EndInterpreter(tstate_new);
                PyEval_RestoreThread(main_tstate);
                return NULL;
            }
        }
    }

    // Step 4b: Replay all module attributes that Go has set via
    // setModuleAttrString. These are attributes like datadog_agent._hostname,
    // datadog_agent._version, etc. that Go pushes into Python at startup.
    // Each sub-interpreter has its own sys.modules, so importing
    // "datadog_agent" here gives us the sub-interpreter's fresh copy —
    // which doesn't have these attributes yet. _replayModuleAttrs iterates
    // over _moduleAttrs (populated by setModuleAttrString) and sets each
    // attribute in this sub-interpreter's copy of the module.
    _replayModuleAttrs();

    // Step 5: Detach from sub-interpreter and re-attach to main.
    //
    // PyEval_SaveThread: releases the sub-interpreter's GIL and detaches
    //   us. After this, no GIL is held and no thread state is attached.
    //
    // PyEval_RestoreThread(main_tstate): acquires the main interpreter's
    //   GIL and attaches main_tstate. We must use PyEval_RestoreThread
    //   (not PyThreadState_Swap) because with OWN_GIL, we need to actually
    //   acquire the main GIL — PyThreadState_Swap only sets the pointer.
    //
    // The sub-interpreter's tstate remains valid but detached. The caller
    // stores it in _checkToInterp; later, runCheck/etc. will re-attach to
    // it via PyEval_RestoreThread(sub_tstate).
    PyEval_SaveThread();
    PyEval_RestoreThread(main_tstate);

    return tstate_new;
}

/*
 * _destroySubInterpreter
 * ----------------------
 * Destroys a sub-interpreter and ends its thread state.
 *
 * IMPORTANT PRECONDITION: The caller must be attached to the sub-interpreter
 * (holding its GIL, tstate is current). This is a requirement of
 * Py_EndInterpreter.
 *
 * After Py_EndInterpreter returns:
 *   - tstate is invalid (freed memory — do not use)
 *   - The sub-interpreter's GIL is gone (interpreter destroyed)
 *   - No thread state is attached, no GIL is held
 *   - We're just running C++ code
 *
 * The caller MUST re-attach to main immediately after via
 * PyEval_RestoreThread(main_tstate).
 *
 * Typical call sequence (in decref):
 *   PyThreadState *main_tstate = PyEval_SaveThread();   // release main GIL
 *   PyEval_RestoreThread(sub_tstate);                    // acquire sub-interp GIL
 *   Py_DECREF(check_object);                            // release check object
 *   _destroySubInterpreter(sub_tstate);                 // destroys interpreter
 *   // Now: no GIL held, no thread state, just C++ code
 *   PyEval_RestoreThread(main_tstate);                  // re-acquire main GIL
 */
void Three::_destroySubInterpreter(PyThreadState *tstate)
{
    if (tstate == NULL) {
        return;
    }

    // Py_EndInterpreter is the high-level API for destroying sub-interpreters.
    // It clears all modules, runs atexit handlers, and frees the interpreter
    // state. The tstate must be the current thread state.
    // After this call, tstate is invalid — do not use it.
    Py_EndInterpreter(tstate);
}

/*
 * _assignInterpreter
 * ------------------
 * Policy function: decides which sub-interpreter a check should run in.
 *
 * Current policy: 1:1 — every check instance gets a brand new sub-interpreter.
 * This provides maximum isolation: each check has its own sys.modules, globals,
 * and GIL.
 *
 * The module_name parameter is currently unused (cast to void to suppress
 * compiler warnings), but it exists so that future policies can use it
 * WITHOUT modifying getCheck() or any other caller. For example:
 *
 *   - Per-type policy: all instances of "datadog_checks.http_check" share
 *     one sub-interpreter. Look up module_name in a map; if found, return
 *     the existing tstate; if not, create a new one.
 *
 *   - Pool policy: round-robin across N pre-created sub-interpreters,
 *     ignoring module_name entirely.
 *
 * To change the assignment strategy, modify ONLY this function.
 *
 * Parameters:
 *   module_name - The Python module name of the check (e.g.,
 *                 "datadog_checks.http_check"). Unused in 1:1 policy.
 *
 * Returns:
 *   A sub-interpreter's PyThreadState, or NULL on failure.
 */
PyThreadState *Three::_assignInterpreter(const char *module_name)
{
    // Check if this module is blocklisted from running in sub-interpreters.
    // Blocklisted modules run in the main interpreter instead. This is needed
    // for checks that transitively depend on C extensions that don't declare
    // sub-interpreter support (e.g., go_expvar → pydantic → _pydantic_core).
    // The blocklist is populated from "subinterpreter_blocklist" in datadog.yaml.
    //
    // We use substring matching so the user can write just "go_expvar" and it
    // matches "datadog_checks.go_expvar.go_expvar" (the full __module__ path).
    if (module_name != NULL) {
        std::string mod(module_name);
        for (const auto &blocked : _subinterpBlocklist) {
            if (mod.find(blocked) != std::string::npos) {
                return NULL;  // NULL signals getCheck to use the main interpreter.
            }
        }
    }

    // 1:1 policy: always create a new sub-interpreter.
    return _createSubInterpreter();
}

/*
 * _lookupCheckInterp
 * ------------------
 * Finds which sub-interpreter a check instance belongs to.
 *
 * Parameters:
 *   check - The PyObject pointer to the check instance. This is the same
 *           pointer that Go stores as c.instance (in check.go) after
 *           getCheck() creates it. It serves as the key in _checkToInterp.
 *
 * Returns:
 *   The sub-interpreter's PyThreadState if found, or NULL if the check
 *   is not in the map (meaning it runs in the main interpreter, either
 *   because sub-interpreters were not used for this check or because it
 *   fell back to main after a module import failure).
 *
 * Thread safety:
 *   Acquires _checkToInterpMutex to protect the read from concurrent
 *   writes by other goroutines (e.g., another goroutine calling decref
 *   which calls _removeCheckInterp).
 */
PyThreadState *Three::_lookupCheckInterp(PyObject *check)
{
    std::lock_guard<std::mutex> lock(_checkToInterpMutex);
    auto it = _checkToInterp.find(check);
    if (it != _checkToInterp.end()) {
        return it->second;
    }
    return NULL;
}

/*
 * _removeCheckInterp
 * ------------------
 * Removes a check from the check-to-interpreter mapping and returns its
 * sub-interpreter's thread state. This is purely a bookkeeping operation
 * on the C++ map — it does NOT touch the Python sub-interpreter at all.
 *
 * The caller (decref) is responsible for the full cleanup sequence:
 *   1. Call _removeCheckInterp(check) to get tstate and remove from map
 *   2. Release main GIL: main_tstate = PyEval_SaveThread()
 *   3. Attach to sub-interp: PyEval_RestoreThread(tstate)
 *   4. Py_DECREF(check) — release object in its home interpreter
 *   5. Call _destroySubInterpreter(tstate) — destroys the interpreter
 *   6. Re-attach to main: PyEval_RestoreThread(main_tstate)
 *
 * Why not do all of this here? Two reasons:
 *   a) Ordering: The check object must be Py_DECREF'd BEFORE the interpreter
 *      is destroyed. If we destroyed here, the object would be invalid before
 *      the caller could release it (use-after-free).
 *   b) Future flexibility: In a per-type policy where multiple check instances
 *      share one interpreter, removing one check does NOT mean destroying the
 *      interpreter (other checks still use it). Separating removal from
 *      destruction means only the destruction logic needs to change.
 *
 * Parameters:
 *   check - The PyObject pointer to the check instance (same key used in
 *           _lookupCheckInterp).
 *
 * Returns:
 *   The sub-interpreter's PyThreadState if found and removed, or NULL if
 *   the check was not in the map.
 *
 * Thread safety:
 *   Acquires _checkToInterpMutex to protect the erase from concurrent access.
 */
PyThreadState *Three::_removeCheckInterp(PyObject *check)
{
    std::lock_guard<std::mutex> lock(_checkToInterpMutex);
    auto it = _checkToInterp.find(check);
    if (it != _checkToInterp.end()) {
        PyThreadState *tstate = it->second;
        _checkToInterp.erase(it);
        return tstate;
    }
    return NULL;
}

/*
 * _replayModuleAttrs
 * ------------------
 * Replays all stored module attributes into the current sub-interpreter.
 *
 * When Go calls setModuleAttrString("datadog_agent", "_hostname", "web-01"),
 * that sets the attribute on the MAIN interpreter's copy of the module. But
 * each sub-interpreter has its own sys.modules, so importing "datadog_agent"
 * in a sub-interpreter gives a fresh copy without _hostname.
 *
 * This function fixes that: it iterates over _moduleAttrs (populated by
 * setModuleAttrString in three.cpp) and sets each (module, attr, value) in
 * the current interpreter's copy of the module. It must be called while
 * attached to the sub-interpreter (holding its GIL).
 *
 * Called from _createSubInterpreter after sys.path is set up, before we
 * detach from the sub-interpreter.
 *
 * Example of what _moduleAttrs contains at runtime:
 *   [0] = { module="datadog_agent", attr="_hostname", value="web-01.prod" }
 *   [1] = { module="datadog_agent", attr="_version",  value="7.52.0"      }
 *   [2] = { module="datadog_agent", attr="_config",   value="/etc/dd"     }
 *
 * For each tuple, this function does the equivalent of:
 *   import datadog_agent
 *   datadog_agent._hostname = "web-01.prod"
 *
 * Errors are logged but not fatal — a missing attribute is better than a
 * failed check instantiation. If a module can't be imported (unlikely, since
 * our builtins are always available), we skip it and continue.
 *
 * Thread safety:
 *   No mutex needed. _moduleAttrs is only written by setModuleAttrString
 *   (under main GIL) and read here (also under main GIL, since
 *   _createSubInterpreter is called from getCheck which holds stickyLock).
 *   Even though we're attached to the sub-interpreter when this runs,
 *   we're in the same OS thread that holds the Go stickyLock, so no other
 *   goroutine can call setModuleAttrString concurrently.
 */
void Three::_replayModuleAttrs()
{
    for (const auto &entry : _moduleAttrs) {
        // Import the module in this sub-interpreter's context.
        // PyImport_ImportModule returns a new reference on success.
        PyObject *py_module = PyImport_ImportModule(entry.module.c_str());
        if (py_module == NULL) {
            // Module not available in this sub-interpreter. This is unusual
            // for our builtins (they support multi-phase init), but could
            // happen for third-party modules. Log and skip.
            PyErr_Clear();
            continue;
        }

        // Create a Python string from the C++ value string.
        PyObject *py_value = PyUnicode_FromString(entry.value.c_str());
        if (py_value == NULL) {
            // String creation failed (out of memory?). Skip this attribute.
            PyErr_Clear();
            Py_DECREF(py_module);
            continue;
        }

        // Set the attribute on the module: module.attr = value
        // PyObject_SetAttrString returns 0 on success, -1 on failure.
        if (PyObject_SetAttrString(py_module, entry.attr.c_str(), py_value) != 0) {
            // Failed to set attribute. Log and continue.
            PyErr_Clear();
        }

        Py_DECREF(py_value);
        Py_DECREF(py_module);
    }
}
