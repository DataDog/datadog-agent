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
 * GIL ownership with OWN_GIL and Py_NewInterpreterFromConfig:
 *   "Attached" = thread holds that interpreter's GIL and tstate is active.
 *   "Detached" = thread does NOT hold that interpreter's GIL.
 *
 *   After Py_NewInterpreterFromConfig with OWN_GIL succeeds:
 *     - Main interpreter:     DETACHED (GIL released, tstate inactive)
 *     - New sub-interpreter:  ATTACHED (GIL held, tstate active)
 *   On failure, tstate_p is NULL and the calling tstate "might not exist"
 *   — we handle this with PyThreadState_Swap (safe for both states).
 *
 * GIL swap pattern for EXISTING sub-interpreters
 * (used by runCheck, cancelCheck, etc. in three.cpp):
 *   PyThreadState_Swap: detaches current tstate (releases its GIL) and
 *   attaches target tstate (acquires its GIL) in one call.
 *   (CPython source: _PyThreadState_Detach(old) + _PyThreadState_Attach(new))
 *
 *     1. main_tstate = PyThreadState_Swap(sub_tstate)  — switch to sub-interp
 *     2. ... do work in sub-interpreter ...
 *     3. PyThreadState_Swap(main_tstate)               — switch back to main
 *
 *   IMPORTANT: Swap requires being currently attached. After Py_EndInterpreter
 *   the thread is detached — must use PyEval_RestoreThread instead
 *   (see _destroySubInterpreter).
 *
 * PyThreadState_Swap source: https://github.com/python/cpython/blob/22c8590e40a13070d75b1e7f9af01252b1b2e9ce/Python/pystate.c#L2593
 * PyEval_RestoreThread source: https://github.com/python/cpython/blob/22c8590e40a13070d75b1e7f9af01252b1b2e9ce/Python/ceval_gil.c#L653
 */



#include "three.h"

#include <sstream>

/*
 * _createSubInterpreter
 * ---------------------
 * Creates a new Python sub-interpreter with per-interpreter GIL.
 *
 * Must be called while holding the main interpreter's GIL (guaranteed
 * by Go's stickyLock → PyGILState_Ensure).
 *
 * Returns:
 *   The new sub-interpreter's PyThreadState on success, or NULL on failure
 *   (with an error set via setError). NULL means the check should run in
 *   the main interpreter. On both success and failure, this function
 *   returns with the main interpreter's GIL held.
 */
PyThreadState *Three::_createSubInterpreter()
{
    // Step 1: Save the main interpreter's thread state.
    PyThreadState *main_tstate = PyThreadState_Get();

    // Step 2: Configure the sub-interpreter (Plain C struct no cleanup needed, c.f., https://github.com/python/cpython/blob/main/Include/cpython/pylifecycle.h).
    // Field values referenced from: https://docs.python.org/3/c-api/subinterpreters.html
    PyInterpreterConfig config = {
        .use_main_obmalloc = 0,              // separate allocator (required for OWN_GIL)
        .allow_fork = 0,                     // fork from sub-interp is dangerous
        .allow_exec = 0,                     // exec replaces process — not useful here
        .allow_threads = 1,                  // needed for I/O in checks
        .allow_daemon_threads = 0,           // daemon threads crash on shutdown
        .check_multi_interp_extensions = 1,  // enforce Py_MOD_PER_INTERPRETER_GIL_SUPPORTED
        .gil = PyInterpreterConfig_OWN_GIL,  // each check gets its own GIL
    };

    // Step 3: Create the sub-interpreter (https://docs.python.org/3/c-api/subinterpreters.html#c.Py_NewInterpreterFromConfig)
    // On success we're attached to the new interpreter. On failure tstate_new is NULL.
    PyThreadState *tstate_new = NULL;
    PyStatus status = Py_NewInterpreterFromConfig(&tstate_new, &config);

    if (PyStatus_Exception(status) || tstate_new == NULL) {
        setError("failed to create sub-interpreter" + (status.err_msg ? ": " + std::string(status.err_msg) : ""));

        /*
         * Python doc: "If creation of the new interpreter is unsuccessful, tstate_p is set to NULL; no exception is set since the exception state is stored in the attached thread state, which might not exist."
         * We don't know if we're attached or detached.
         * I'm guessing that this means that failure could happen:
         *   1. Failure before detach: still attached to main
         *   2. Failure after detach: thread left detached
         * Guessing: but not sure. PyThreadState_Swap handles both: detaches old if attached, then attaches main. Safe regardless of which state we're actually in.
         */
        PyThreadState_Swap(main_tstate);
        return NULL;
    }


    // Step 4: Copy _pythonPaths (agent's Python package search paths, e.g., dist/, checks.d/ - see cmd/agent/common/common.go) into the sub-interpreter's sys.path.
    if (_pythonPaths.empty()) {
        setError("sub-interpreter: _pythonPaths is empty — no Python sys.path search paths were added "
                 "(e.g., dist/, checks.d/, additional_checksd). Imports will likely fail.");
    } else {
        // https://docs.python.org/3/c-api/sys.html#c.PySys_GetObject
        PyObject *path = PySys_GetObject("path");  // borrowed reference so no need to Py_DECREF after

        if (path == NULL) {
            setError("sub-interpreter: could not access sys.path: " + _fetchPythonError());
            _destroySubInterpreter(tstate_new, main_tstate);
            return NULL;
        }

        // Insert at index 0 so agent paths take priority over site-packages (Python searches sys.path front-to-back).
        // e.g., if datadog_checks is also pip-installed in site-packages, front-insertion ensures the agent's bundled version wins.
        // Reverse iteration so the final order in sys.path matches _pythonPaths.
        for (int i = (int)_pythonPaths.size() - 1; i >= 0; --i) {
            // Fresh string needed - objects can't be shared across interpreters (use_main_obmalloc = 0).
            PyObject *p = PyUnicode_FromString(_pythonPaths[i].c_str());
            if (p == NULL) { // OOM
                setError("sub-interpreter: could not create path string: " + _fetchPythonError());
                _destroySubInterpreter(tstate_new, main_tstate);
                return NULL;
            }
            int rc = PyList_Insert(path, 0, p);
            // PyList_Insert creates its own reference to p, so we must release ours. Even in case of failure, good practice to clean up before _destroySubInterpreter.
            // https://docs.python.org/3/c-api/list.html#c.PyList_Insert --> C API that steals references are explicitly called out (e.g., PyList_SET_ITEM), not the case for PyList_Insert.
            Py_DECREF(p);
            if (rc == -1) {
                setError("sub-interpreter: could not insert into sys.path: "
                         + _fetchPythonError());
                _destroySubInterpreter(tstate_new, main_tstate);
                return NULL;
            }
        }
    } // end else (_pythonPaths not empty)

    // Step 4b: Copy module attributes set by Go via setModuleAttrString() into this sub-interpreter.
    // Each sub-interpreter has its own sys.modules, so its copies of modules start without these attributes.
    // Currently the only production caller is SetPythonPsutilProcPath (pkg/collector/python/helpers.go) which sets psutil.PROCFS_PATH.
    _copyModuleAttrs();

    // Step 5: Detach from sub-interpreter and re-attach to main.
    // tstate_new remains valid but detached. getCheck stores it in _checkToInterp; later, runCheck/cancelCheck/decref/etc. use it to Swap into the sub-interpreter.
    PyThreadState_Swap(main_tstate);

    return tstate_new;
}

/*
 * _destroySubInterpreter
 * ----------------------
 * Destroys a sub-interpreter and re-attaches to the given interpreter.
 *
 * PRECONDITION: Caller must be attached to the sub-interpreter.
 * This is a requirement of Py_EndInterpreter (no attached thread state on return: https://docs.python.org/3.12/c-api/init.html#c.Py_EndInterpreter)
 *
 * After this function returns, the caller is attached to restore_tstate.
 *
 * Typical call sequence (in decref):
 *   PyThreadState *main_tstate = PyThreadState_Swap(sub_tstate);  // switch to sub-interp
 *   Py_DECREF(check_object);                                      // release check object
 *   _destroySubInterpreter(sub_tstate, main_tstate);              // destroys + re-attaches to main
 */
void Three::_destroySubInterpreter(PyThreadState *tstate, PyThreadState *restore_tstate)
{
    if (tstate == NULL) {
        return;
    }

    Py_EndInterpreter(tstate);
    PyEval_RestoreThread(restore_tstate);
}

/*
 * _assignInterpreter
 * ------------------
 * Policy function: decides which sub-interpreter a check should run in.
 *
 * Current policy: 1:1 — every check instance gets its own sub-interpreter,
 * unless the module is blocklisted (falls back to main interpreter).
 *
 * Future policy examples:
 *   - Per-type: all instances of "datadog_checks.http_check" share one
 *     sub-interpreter.
 *   - Pool: round-robin across N pre-created sub-interpreters.
 *
 * To change the assignment strategy, modify ONLY this function.
 *
 * Returns:
 *   A sub-interpreter's PyThreadState, or NULL if the module is blocklisted
 *   or creation failed (caller falls back to main interpreter).
 */
PyThreadState *Three::_assignInterpreter(const char *module_name)
{
    // Blocklisted modules fall back to the main interpreter. Needed for checks
    // with transitive C extension deps that don't support sub-interpreters
    // (e.g., go_expvar → pydantic → _pydantic_core).
    // Populated from "subinterpreter_blocklist" in datadog.yaml.
    // Substring matching: "go_expvar" matches "datadog_checks.go_expvar.go_expvar" (the full __module__ path).
    if (module_name != NULL) {
        std::string mod(module_name);
        for (const auto &blocked : _subinterpBlocklist) {
            if (mod.find(blocked) != std::string::npos) {
                return NULL;  // NULL signals getCheck to use the main interpreter.
            }
        }
    }

    return _createSubInterpreter();
}

/*
 * _lookupCheckInterp
 * ------------------
 * Finds which sub-interpreter a check instance belongs to.
 * Uses the check's PyObject pointer as the key in _checkToInterp.
 *
 * Returns NULL if the check is not in the map (runs in main interpreter).
 * Thread-safe: acquires _checkToInterpMutex.
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
 * Removes a check from _checkToInterp and returns its sub-interpreter's
 * tstate. Purely a C++ map operation — does NOT touch the Python sub-interpreter.
 * Only called from decref() in three.cpp.
 *
 * decref's full cleanup sequence:
 *   1. _removeCheckInterp(check) — get tstate, remove from map
 *   2. PyThreadState_Swap(tstate) — switch to sub-interp
 *   3. Py_DECREF(check) — release object in its home interpreter
 *   4. _destroySubInterpreter(tstate, main_tstate) — destroys + re-attaches
 *
 * Destruction is separated from removal so that the interpreter can be
 * reused (e.g., shared by multiple checks, or recycled for future checks).
 *
 * Returns NULL if the check was not in the map.
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
 * _copyModuleAttrs
 * ------------------
 * Copies all stored module attributes into the current sub-interpreter.
 * Must be called while attached to the sub-interpreter (holding its GIL).
 *
 * _moduleAttrs is populated by setModuleAttrString (three.cpp), which sets
 * attributes on the MAIN interpreter's modules. Sub-interpreters have their
 * own sys.modules, so fresh imports lack those attributes. This function
 * copies them.
 *
 * Currently the only production caller of setModuleAttrString is
 * SetPythonPsutilProcPath (pkg/collector/python/helpers.go): psutil.PROCFS_PATH.
 */
void Three::_copyModuleAttrs()
{
    for (const auto &entry : _moduleAttrs) {
        // Import the module in this sub-interpreter's context.
        // PyImport_ImportModule returns a new reference on success.
        // https://docs.python.org/3/c-api/import.html#c.PyImport_ImportModule
        PyObject *py_module = PyImport_ImportModule(entry.module.c_str());
        if (py_module == NULL) {
            // Module not compatible with sub-interpreters (e.g., psutil). Clear and skip.
            PyErr_Clear();
            continue;
        }

        // Create a new Python string object in this sub-interpreter's context.
        // Each interpreter has its own allocator (use_main_obmalloc = 0) for
        // isolation, so objects can't be shared across interpreters. Must create a fresh one here.
        PyObject *py_value = PyUnicode_FromString(entry.value.c_str());
        if (py_value == NULL) {
            // String creation failed (OOM).
            PyErr_Clear();
            Py_DECREF(py_module);
            continue;
        }

        // Set the attribute on the module: module.attr = value (https://docs.python.org/3/c-api/object.html)
        if (PyObject_SetAttrString(py_module, entry.attr.c_str(), py_value) != 0) {
            // Failed to set attribute. Clear error and continue.
            PyErr_Clear();
        }

        Py_DECREF(py_value);
        Py_DECREF(py_module);
    }
}
