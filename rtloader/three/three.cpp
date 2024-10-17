// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "three.h"

#include "constants.h"

#include "_util.h"
#include "aggregator.h"
#include "cgo_free.h"
#include "containers.h"
#include "datadog_agent.h"
#include "kubeutil.h"
#include "rtloader_mem.h"
#include "stringutils.h"
#include "tagger.h"
#include "util.h"

#include <algorithm>
#include <sstream>

extern "C" DATADOG_AGENT_RTLOADER_API RtLoader *create(const char *python_home, const char *python_exe,
                                                       cb_memory_tracker_t memtrack_cb)
{
    return new Three(python_home, python_exe, memtrack_cb);
}

extern "C" DATADOG_AGENT_RTLOADER_API void destroy(RtLoader *p)
{
    delete p;
}

Three::Three(const char *python_home, const char *python_exe, cb_memory_tracker_t memtrack_cb)
    : RtLoader(memtrack_cb)
    , _pythonHome(NULL)
    , _pythonExe(NULL)
    , _baseClass(NULL)
    , _pythonPaths()
    , _pymallocPrev{ 0 }
    , _pymemInuse(0)
    , _pymemAlloc(0)
{
    initPythonHome(python_home);

    // If not empty, set our Python interpreter path
    if (python_exe && strlen(python_exe) > 0) {
        initPythonExe(python_exe);
    }
}

Three::~Three()
{
    // For more information on why Py_Finalize() isn't called here please
    // refer to the header file or the doxygen documentation.
    PyEval_RestoreThread(_threadState);
    Py_XDECREF(_baseClass);
}

void Three::initPythonHome(const char *pythonHome)
{
    // Py_SetPythonHome stores a pointer to the string we pass to it, so we must keep it in memory
    wchar_t *oldPythonHome = _pythonHome;
    if (pythonHome == NULL || strlen(pythonHome) == 0) {
        _pythonHome = Py_DecodeLocale(_defaultPythonHome, NULL);
    } else {
        _pythonHome = Py_DecodeLocale(pythonHome, NULL);
    }

    Py_SetPythonHome(_pythonHome);
    PyMem_RawFree((void *)oldPythonHome);
}

void Three::initPythonExe(const char *python_exe)
{
    // Py_SetProgramName stores a pointer to the string we pass to it, so we must keep it in memory
    wchar_t *oldPythonExe = _pythonExe;
    _pythonExe = Py_DecodeLocale(python_exe, NULL);

    Py_SetProgramName(_pythonExe);

    // HACK: This extra internal API invocation is due to the workaround for an upstream bug on
    // Windows (https://bugs.python.org/issue34725) where just using `Py_SetProgramName` is
    // ineffective. The workaround API call will be removed at some point in the future (Python
    // 3.12+) so we should convert this initialization to the new`PyConfig API`
    // (https://docs.python.org/3.11/c-api/init_config.html#c.PyConfig) before then.
    _Py_SetProgramFullPath(_pythonExe);

    PyMem_RawFree((void *)oldPythonExe);
}

bool Three::init()
{
    // we want the checks to be runned with the standard encoding utf-8
    // setting this var to 1 forces the UTF8 mode for CPython >= 3.7
    // See:
    //	- PEP UTF8 mode https://www.python.org/dev/peps/pep-0540/
    //	- about this var https://github.com/python/cpython/pull/12589
    // This has to be set before the Py_Initialize() call.
    Py_UTF8Mode = 1;

    // add custom builtins init funcs to Python inittab, one by one
    // Unlinke its py2 counterpart, these need to be called before Py_Initialize
    PyImport_AppendInittab(AGGREGATOR_MODULE_NAME, PyInit_aggregator);
    PyImport_AppendInittab(DATADOG_AGENT_MODULE_NAME, PyInit_datadog_agent);
    PyImport_AppendInittab(UTIL_MODULE_NAME, PyInit_util);
    PyImport_AppendInittab(_UTIL_MODULE_NAME, PyInit__util);
    PyImport_AppendInittab(TAGGER_MODULE_NAME, PyInit_tagger);
    PyImport_AppendInittab(KUBEUTIL_MODULE_NAME, PyInit_kubeutil);
    PyImport_AppendInittab(CONTAINERS_MODULE_NAME, PyInit_containers);

    Py_Initialize();

    if (!Py_IsInitialized()) {
        setError("Python not initialized");
        return false;
    }

    // Set PYTHONPATH
    if (!_pythonPaths.empty()) {
        char pathchr[] = "path";
        PyObject *path = PySys_GetObject(pathchr); // borrowed
        if (path == NULL) {
            // sys.path doesn't exist, which should never happen.
            // No exception is set on the interpreter, so no need to handle any.
            setError("could not access sys.path");
            goto done;
        }
        for (PyPaths::iterator pit = _pythonPaths.begin(); pit != _pythonPaths.end(); ++pit) {
            PyObject *p = PyUnicode_FromString(pit->c_str());
            if (p == NULL) {
                setError("could not set pythonPath: " + _fetchPythonError());
                goto done;
            }
            int retval = PyList_Append(path, p);
            Py_XDECREF(p);
            if (retval == -1) {
                setError("could not append path to pythonPath: " + _fetchPythonError());
                goto done;
            }
        }
    }

    if (init_stringutils() != EXIT_SUCCESS) {
        setError("error initializing string utils: " + _fetchPythonError());
        goto done;
    }

    // import the base class
    _baseClass = _importFrom("datadog_checks.checks", "AgentCheck");
    if (_baseClass == NULL) {
        setError("could not import base class: " + std::string(getError()));
    }

done:
    // save thread state and release the GIL
    _threadState = PyEval_SaveThread();

    return _baseClass != NULL;
}

/**
 * getPyInfo()
 */
py_info_t *Three::getPyInfo()
{
    PyObject *sys = NULL;
    PyObject *path = NULL;
    PyObject *str_path = NULL;

    py_info_t *info = (py_info_t *)_malloc(sizeof(*info));
    if (!info) {
        setError("could not allocate a py_info_t struct");
        return NULL;
    }
    info->version = Py_GetVersion();
    info->path = NULL;

    sys = PyImport_ImportModule("sys");
    if (!sys) {
        setError("could not import module 'sys': " + _fetchPythonError());
        goto done;
    }

    path = PyObject_GetAttrString(sys, "path");
    if (!path) {
        setError("could not get 'sys.path': " + _fetchPythonError());
        goto done;
    }

    str_path = PyObject_Repr(path);
    if (!str_path) {
        setError("could not compute a string representation of 'sys.path': " + _fetchPythonError());
        goto done;
    }

    info->path = as_string(str_path);

done:
    Py_XDECREF(sys);
    Py_XDECREF(path);
    Py_XDECREF(str_path);
    return info;
}
/**
 * freePyInfo()
 */

void Three::freePyInfo(py_info_t *info)
{
    info->version = NULL;
    if (info->path) {
        _free(info->path);
        info->version = NULL;
    }
    _free(info);
    return;
}
bool Three::runSimpleString(const char *code) const
{
    return PyRun_SimpleString(code) == 0;
}

bool Three::addPythonPath(const char *path)
{
    if (std::find(_pythonPaths.begin(), _pythonPaths.end(), path) == _pythonPaths.end()) {
        _pythonPaths.push_back(path);
        return true;
    }
    return false;
}

rtloader_gilstate_t Three::GILEnsure()
{
    PyGILState_STATE state = PyGILState_Ensure();
    if (state == PyGILState_LOCKED) {
        return DATADOG_AGENT_RTLOADER_GIL_LOCKED;
    }
    return DATADOG_AGENT_RTLOADER_GIL_UNLOCKED;
}

void Three::GILRelease(rtloader_gilstate_t state)
{
    if (state == DATADOG_AGENT_RTLOADER_GIL_LOCKED) {
        PyGILState_Release(PyGILState_LOCKED);
    } else {
        PyGILState_Release(PyGILState_UNLOCKED);
    }
}

bool Three::getClass(const char *module, RtLoaderPyObject *&pyModule, RtLoaderPyObject *&pyClass)
{
    PyObject *obj_module = NULL;
    PyObject *obj_class = NULL;

    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        std::ostringstream err;
        err << "unable to import module '" << module << "': " + _fetchPythonError();
        setError(err.str());
        return false;
    }

    obj_class = _findSubclassOf(_baseClass, obj_module);
    if (obj_class == NULL) {
        // `_findSubclassOf` does not set the interpreter's error flag, but leaves an error on rtloader
        std::ostringstream err;
        err << "unable to find a subclass of the base check in module '" << module << "': " << getError();
        setError(err.str());
        Py_XDECREF(obj_module);
        return false;
    }

    pyModule = reinterpret_cast<RtLoaderPyObject *>(obj_module);
    pyClass = reinterpret_cast<RtLoaderPyObject *>(obj_class);
    return true;
}

bool Three::getCheck(RtLoaderPyObject *py_class, const char *init_config_str, const char *instance_str,
                     const char *check_id_str, const char *check_name, const char *agent_config_str,
                     RtLoaderPyObject *&check)
{

    PyObject *klass = reinterpret_cast<PyObject *>(py_class);
    PyObject *agent_config = NULL;
    PyObject *init_config = NULL;
    PyObject *instance = NULL;
    PyObject *instances = NULL;
    PyObject *py_check = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;
    PyObject *check_id = NULL;
    PyObject *name = NULL;

    char load_config[] = "load_config";
    char format[] = "(s)"; // use parentheses to force Tuple creation

    // call `AgentCheck.load_config(init_config)`
    init_config = PyObject_CallMethod(klass, load_config, format, init_config_str);
    if (init_config == NULL) {
        setError("error parsing init_config: " + _fetchPythonError());
        goto done;
    }
    // replace an empty init_config by  a empty dict
    if (init_config == Py_None) {
        Py_XDECREF(init_config);
        init_config = PyDict_New();
        if (init_config == NULL) {
            setError("error 'init_config' can't be initialized to an empty dict: " + _fetchPythonError());
            goto done;
        }
    } else if (!PyDict_Check(init_config)) {
        setError("error 'init_config' is not a dict");
        goto done;
    }

    // call `AgentCheck.load_config(instance)`
    instance = PyObject_CallMethod(klass, load_config, format, instance_str);
    if (instance == NULL) {
        setError("error parsing instance: " + _fetchPythonError());
        goto done;
    } else if (!PyDict_Check(instance)) {
        setError("error instance is not a dict");
        Py_XDECREF(instance); // we still own the reference to instance, so we need to decref it here
        goto done;
    }

    instances = PyTuple_New(1);
    if (instances == NULL) {
        setError("could not create tuple for instances: " + _fetchPythonError());
        Py_XDECREF(instance); // we still own the reference to instance, so we need to decref it here
        goto done;
    }
    // As stated in the Python C-API documentation
    // https://github.com/python/cpython/blob/2.7/Doc/c-api/intro.rst#reference-count-details, PyTuple_SetItem takes
    // over ownership of the given item (instance in this case). This means that we should NOT DECREF it
    if (PyTuple_SetItem(instances, 0, instance) != 0) {
        setError("could not set instance item on instances: " + _fetchPythonError());
        goto done;
    }

    // create `args` and `kwargs` to invoke `AgentCheck` constructor
    args = PyTuple_New(0);
    if (args == NULL) {
        setError("error 'args' can't be initialized to an empty tuple: " + _fetchPythonError());
        goto done;
    }
    kwargs = PyDict_New();
    if (kwargs == NULL) {
        setError("error 'kwargs' can't be initialized to an empty dict: " + _fetchPythonError());
        goto done;
    }
    name = PyUnicode_FromString(check_name);
    if (name == NULL) {
        setError("error 'name' can't be initialized: " + _fetchPythonError());
        goto done;
    }
    if (PyDict_SetItemString(kwargs, "name", name) == -1) {
        setError("error 'name' key can't be set: " + _fetchPythonError());
        goto done;
    }
    if (PyDict_SetItemString(kwargs, "init_config", init_config) == -1) {
        setError("error 'init_config' key can't be set: " + _fetchPythonError());
        goto done;
    }
    if (PyDict_SetItemString(kwargs, "instances", instances) == -1) {
        setError("error 'instances' key can't be set: " + _fetchPythonError());
        goto done;
    }

    if (agent_config_str != NULL) {
        agent_config = PyObject_CallMethod(klass, load_config, format, agent_config_str);
        if (agent_config == NULL) {
            setError("error parsing agent_config: " + _fetchPythonError());
            goto done;
        } else if (!PyDict_Check(agent_config)) {
            setError("error agent_config is not a dict");
            goto done;
        }

        if (PyDict_SetItemString(kwargs, "agentConfig", agent_config) == -1) {
            setError("error 'agentConfig' key can't be set: " + _fetchPythonError());
            goto done;
        }
    }

    // call `AgentCheck` constructor
    py_check = PyObject_Call(klass, args, kwargs);
    if (py_check == NULL) {
        setError(_fetchPythonError());
        goto done;
    }

    if (check_id_str != NULL && strlen(check_id_str) != 0) {
        check_id = PyUnicode_FromString(check_id_str);
        if (check_id == NULL) {
            std::ostringstream err;
            err << "error could not set check_id: " << check_id_str;
            setError(err.str());
            Py_XDECREF(py_check);
            py_check = NULL;
            goto done;
        }

        if (PyObject_SetAttrString(py_check, "check_id", check_id) != 0) {
            setError("error could not set 'check_id' attr: " + _fetchPythonError());
            Py_XDECREF(py_check);
            py_check = NULL;
            goto done;
        }
    }

done:
    // We purposefully avoid calling Py_XDECREF on instance because we lost ownership earlier by
    // calling PyTuple_SetItem. More details are available in the comment above this PyTuple_SetItem
    // call
    Py_XDECREF(name);
    Py_XDECREF(check_id);
    Py_XDECREF(init_config);
    Py_XDECREF(instances);
    Py_XDECREF(agent_config);
    Py_XDECREF(args);
    Py_XDECREF(kwargs);

    if (py_check == NULL) {
        return false;
    }

    check = reinterpret_cast<RtLoaderPyObject *>(py_check);
    return true;
}

char *Three::runCheck(RtLoaderPyObject *check)
{
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret = NULL;
    char run[] = "run";
    PyObject *result = NULL;

    result = PyObject_CallMethod(py_check, run, NULL);
    if (result == NULL || !PyUnicode_Check(result)) {
        setError("error invoking 'run' method: " + _fetchPythonError());
        goto done;
    }

    ret = as_string(result);
    if (ret == NULL) {
        // as_string clears the error, so we can't fetch it here
        setError("error converting 'run' result to string");
        goto done;
    }

done:
    Py_XDECREF(result);
    return ret;
}

void Three::cancelCheck(RtLoaderPyObject *check)
{
    if (check == NULL) {
        return;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    char cancel[] = "cancel";
    PyObject *result = NULL;

    result = PyObject_CallMethod(py_check, cancel, NULL);
    // at least None should be returned
    if (result == NULL) {
        setError("error invoking 'cancel' method: " + _fetchPythonError());
    }
    Py_XDECREF(result);
}

char **Three::getCheckWarnings(RtLoaderPyObject *check)
{
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);
    char **warnings = NULL;

    char func_name[] = "get_warnings";
    Py_ssize_t numWarnings;

    PyObject *warns_list = PyObject_CallMethod(py_check, func_name, NULL);
    if (warns_list == NULL) {
        setError("error invoking 'get_warnings' method: " + _fetchPythonError());
        goto done;
    }

    numWarnings = PyList_Size(warns_list);
    // docs are not clear but `PyList_Size` can actually fail and in case it would
    // return -1, see https://github.com/python/cpython/blob/3.8/Objects/listobject.c#L223
    if (numWarnings == -1) {
        setError("error computing 'len(warnings)': " + _fetchPythonError());
        goto done;
    }

    warnings = (char **)_malloc(sizeof(*warnings) * (numWarnings + 1));
    if (!warnings) {
        setError("could not allocate memory to store warnings");
        goto done;
    }
    warnings[numWarnings] = NULL;

    for (Py_ssize_t idx = 0; idx < numWarnings; idx++) {
        PyObject *warn = PyList_GetItem(warns_list, idx); // borrowed ref
        if (warn == NULL) {
            setError("there was an error browsing 'warnings' list: " + _fetchPythonError());

            for (int jdx = 0; jdx < idx; jdx++) {
                _free(warnings[jdx]);
            }
            _free(warnings);

            warnings = NULL;
            goto done;
        }
        warnings[idx] = as_string(warn);
    }

done:
    Py_XDECREF(warns_list);
    return warnings;
}

char *Three::getCheckDiagnoses(RtLoaderPyObject *check)
{
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret = NULL;
    char func_name[] = "get_diagnoses";
    PyObject *result = NULL;

    result = PyObject_CallMethod(py_check, func_name, NULL);
    if (result == NULL || !PyUnicode_Check(result)) {
        ret = _createInternalErrorDiagnoses(_fetchPythonError().c_str());
        goto done;
    }

    ret = as_string(result);
    if (ret == NULL) {
        // as_string clears the error, so we can't fetch it here
        ret = _createInternalErrorDiagnoses("error converting 'get_diagnoses' result to string");
        goto done;
    }

done:
    Py_XDECREF(result);
    return ret;
}

// return new reference
PyObject *Three::_importFrom(const char *module, const char *name)
{
    PyObject *obj_module = NULL;
    PyObject *obj_symbol = NULL;

    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        setError(_fetchPythonError());
        goto error;
    }

    obj_symbol = PyObject_GetAttrString(obj_module, name);
    if (obj_symbol == NULL) {
        setError(_fetchPythonError());
        goto error;
    }

    return obj_symbol;

error:
    Py_XDECREF(obj_module);
    Py_XDECREF(obj_symbol);
    return NULL;
}

PyObject *Three::_findSubclassOf(PyObject *base, PyObject *module)
{
    // baseClass is not a Class type
    if (base == NULL || !PyType_Check(base)) {
        setError("base class is not of type 'Class'");
        return NULL;
    }

    // module is not a Module object
    if (module == NULL || !PyModule_Check(module)) {
        setError("module is not of type 'Module'");
        return NULL;
    }

    PyObject *dir = PyObject_Dir(module);
    if (dir == NULL) {
        PyErr_Clear();
        setError("there was an error calling dir() on module object");
        return NULL;
    }

    PyObject *klass = NULL;
    for (int i = 0; i < PyList_GET_SIZE(dir); i++) {
        // Reset `klass` at every iteration so its state is always clean when we
        // continue the loop or return early. Reset at first iteration is useless
        // but it keeps the code readable.
        Py_XDECREF(klass);
        klass = NULL;

        // get the symbol in current list item
        PyObject *symbol = PyList_GetItem(dir, i);
        if (symbol == NULL) {
            // This should never happen as it means we're out of bounds
            PyErr_Clear();
            setError("there was an error browsing dir() output");
            goto done;
        }

        // get symbol name, as_string returns a new object that we have to free
        char *symbol_name = as_string(symbol);
        if (symbol_name == NULL) {
            // as_string returns NULL if `symbol` is not a string object
            // and raises TypeError. Let's clear the error and keep going.
            PyErr_Clear();
            continue;
        }

        // get symbol instance. It's a new ref but in case of success we don't
        // DecRef since we return it and the caller will be owner
        klass = PyObject_GetAttrString(module, symbol_name);
        ::_free(symbol_name);
        if (klass == NULL) {
            PyErr_Clear();
            continue;
        }

        // Not a class, ignore
        if (!PyType_Check(klass)) {
            continue;
        }

        // Unrelated class, ignore
        if (!PyType_IsSubtype((PyTypeObject *)klass, (PyTypeObject *)base)) {
            continue;
        }

        // check whether `klass` is actually `base` itself
        int retval = PyObject_RichCompareBool(klass, base, Py_EQ);
        if (retval == 1) {
            // `klass` is indeed `base`, continue
            continue;
        } else if (retval == -1) {
            // an error occurred calling __eq__, clear and continue
            PyErr_Clear();
            continue;
        }

        // does `klass` have subclasses?
        char func_name[] = "__subclasses__";
        PyObject *children = PyObject_CallMethod(klass, func_name, NULL);
        if (children == NULL) {
            PyErr_Clear();
            continue;
        }

        // how many?
        int children_count = PyList_GET_SIZE(children);
        Py_XDECREF(children);

        // Agent integrations are supposed to have no subclasses
        if (children_count > 0) {
            continue;
        }

        // got it, return the check class
        goto done;
    }

    // we couldn't find any good subclass, set an error and reset
    // `klass` state for one last time before moving to `done`.
    setError("cannot find a subclass");
    Py_XDECREF(klass);
    klass = NULL;

done:
    Py_XDECREF(dir);
    return klass;
}

std::string Three::_fetchPythonError() const
{
    std::string ret_val = "";

    if (PyErr_Occurred() == NULL) {
        return ret_val;
    }

    PyObject *ptype = NULL;
    PyObject *pvalue = NULL;
    PyObject *ptraceback = NULL;
    PyObject *format_exception = NULL;
    PyObject *traceback = NULL;
    PyObject *fmt_exc = NULL;

    // Fetch error and make sure exception values are normalized, as per python C
    // API docs.
    // PyErr_Fetch returns void, no need to check its return value
    PyErr_Fetch(&ptype, &pvalue, &ptraceback);
    // PyErr_NormalizeException returns void, no need to check its return value
    PyErr_NormalizeException(&ptype, &pvalue, &ptraceback);

    // There's a traceback, try to format it nicely
    if (ptraceback != NULL) {
        traceback = PyImport_ImportModule("traceback");
        if (traceback != NULL) {
            char fname[] = "format_exception";
            format_exception = PyObject_GetAttrString(traceback, fname);
            if (format_exception != NULL) {
                fmt_exc = PyObject_CallFunctionObjArgs(format_exception, ptype, pvalue, ptraceback, NULL);
                if (fmt_exc != NULL) {
                    Py_ssize_t len = PyList_Size(fmt_exc);
                    // docs are not clear but `PyList_Size` can actually fail and in case it would
                    // return -1, see https://github.com/python/cpython/blob/2.7/Objects/listobject.c#L170
                    if (len == -1) {
                        // don't fetch the actual error or the caller might think it was the root cause,
                        // while it's not. Setting `ret_val` empty will make the function return "unknown error".
                        // PyErr_Clear() will be called before returning.
                        ret_val = "";
                        goto done;
                    }

                    // "format_exception" returns a list of strings (one per line)
                    for (int i = 0; i < len; i++) {
                        PyObject *s = PyList_GetItem(fmt_exc, i); // borrowed ref
                        if (s == NULL || !PyUnicode_Check(s)) {
                            // unlikely to happen but same as above, do not propagate this error upstream
                            // to avoid confusing the caller. PyErr_Clear() will be called before returning.
                            ret_val = "";
                            goto done;
                        }
                        char *item = as_string(s);
                        // traceback.format_exception returns a list of strings, each ending in a *newline*
                        // and some containing internal newlines. No need to add any CRLF/newlines.
                        ret_val += item;
                        ::_free(item);
                    }
                }
            }
        } else {
            // If we reach this point, there was an error while formatting the
            // exception
            ret_val = "can't format exception";
        }
    }
    // we sometimes do not get a traceback but an error in pvalue
    else if (pvalue != NULL) {
        PyObject *pvalue_obj = PyObject_Str(pvalue);
        if (pvalue_obj != NULL) {
            // we know pvalue_obj is a string (we just casted it), no need to PyUnicode_Check()
            char *ret = as_string(pvalue_obj);
            ret_val += ret;
            ::_free(ret);
            Py_XDECREF(pvalue_obj);
        }
    } else if (ptype != NULL) {
        PyObject *ptype_obj = PyObject_Str(ptype);
        if (ptype_obj != NULL) {
            // we know ptype_obj is a string (we just casted it), no need to PyUnicode_Check()
            char *ret = as_string(ptype_obj);
            ret_val += ret;
            ::_free(ret);
            Py_XDECREF(ptype_obj);
        }
    }

done:
    if (ret_val == "") {
        ret_val = "unknown error";
    }

    // something might've gone wrong while fetching the error, ensure
    // the error state is clean before returning
    PyErr_Clear();

    Py_XDECREF(traceback);
    Py_XDECREF(format_exception);
    Py_XDECREF(fmt_exc);
    Py_XDECREF(ptype);
    Py_XDECREF(pvalue);
    Py_XDECREF(ptraceback);
    return ret_val;
}

bool Three::getAttrString(RtLoaderPyObject *obj, const char *attributeName, char *&value) const
{
    if (obj == NULL) {
        return false;
    }

    bool res = false;
    PyObject *py_attr = NULL;
    PyObject *py_obj = reinterpret_cast<PyObject *>(obj);

    py_attr = PyObject_GetAttrString(py_obj, attributeName);
    if (py_attr != NULL && PyUnicode_Check(py_attr)) {
        value = as_string(py_attr);
        if (value == NULL) {
            // as_string clears the error, so we can't fetch it here
            setError("error converting attribute " + std::string(attributeName) + " to string");
        } else {
            res = true;
        }
    } else if (py_attr != NULL && !PyUnicode_Check(py_attr)) {
        setError("error attribute " + std::string(attributeName) + " has a different type than unicode");
        PyErr_Clear();
    } else {
        PyErr_Clear();
    }

    Py_XDECREF(py_attr);
    return res;
}

void Three::decref(RtLoaderPyObject *obj)
{
    Py_XDECREF(reinterpret_cast<PyObject *>(obj));
}

void Three::incref(RtLoaderPyObject *obj)
{
    Py_XINCREF(reinterpret_cast<PyObject *>(obj));
}

void Three::setModuleAttrString(char *module, char *attr, char *value)
{
    PyObject *py_module = PyImport_ImportModule(module);
    if (!py_module) {
        setError("error importing python '" + std::string(module) + "' module: " + _fetchPythonError());
        return;
    }

    PyObject *py_value = PyUnicode_FromString(value);
    if (PyObject_SetAttrString(py_module, attr, py_value) != 0) {
        setError("error setting the '" + std::string(module) + "." + std::string(attr)
                 + "' attribute: " + _fetchPythonError());
    }

    Py_XDECREF(py_module);
    Py_XDECREF(py_value);
}

void Three::setSubmitMetricCb(cb_submit_metric_t cb)
{
    _set_submit_metric_cb(cb);
}

void Three::setSubmitServiceCheckCb(cb_submit_service_check_t cb)
{
    _set_submit_service_check_cb(cb);
}

void Three::setSubmitEventCb(cb_submit_event_t cb)
{
    _set_submit_event_cb(cb);
}

void Three::setSubmitHistogramBucketCb(cb_submit_histogram_bucket_t cb)
{
    _set_submit_histogram_bucket_cb(cb);
}

void Three::setSubmitEventPlatformEventCb(cb_submit_event_platform_event_t cb)
{
    _set_submit_event_platform_event_cb(cb);
}

void Three::setGetVersionCb(cb_get_version_t cb)
{
    _set_get_version_cb(cb);
}

void Three::setGetConfigCb(cb_get_config_t cb)
{
    _set_get_config_cb(cb);
}

void Three::setHeadersCb(cb_headers_t cb)
{
    _set_headers_cb(cb);
}

void Three::setGetHostnameCb(cb_get_hostname_t cb)
{
    _set_get_hostname_cb(cb);
}

void Three::setGetHostTagsCb(cb_get_host_tags_t cb)
{
    _set_get_host_tags_cb(cb);
}

void Three::setGetClusternameCb(cb_get_clustername_t cb)
{
    _set_get_clustername_cb(cb);
}

void Three::setGetTracemallocEnabledCb(cb_tracemalloc_enabled_t cb)
{
    _set_tracemalloc_enabled_cb(cb);
}

void Three::setLogCb(cb_log_t cb)
{
    _set_log_cb(cb);
}

void Three::setSendLogCb(cb_send_log_t cb)
{
    _set_send_log_cb(cb);
}

void Three::setSetCheckMetadataCb(cb_set_check_metadata_t cb)
{
    _set_set_check_metadata_cb(cb);
}

void Three::setSetExternalTagsCb(cb_set_external_tags_t cb)
{
    _set_set_external_tags_cb(cb);
}

void Three::setSubprocessOutputCb(cb_get_subprocess_output_t cb)
{
    _set_get_subprocess_output_cb(cb);
}

void Three::setCGOFreeCb(cb_cgo_free_t cb)
{
    _set_cgo_free_cb(cb);
}

void Three::setTagsCb(cb_tags_t cb)
{
    _set_tags_cb(cb);
}

void Three::setGetConnectionInfoCb(cb_get_connection_info_t cb)
{
    _set_get_connection_info_cb(cb);
}

void Three::setIsExcludedCb(cb_is_excluded_t cb)
{
    _set_is_excluded_cb(cb);
}

void Three::setWritePersistentCacheCb(cb_write_persistent_cache_t cb)
{
    _set_write_persistent_cache_cb(cb);
}

void Three::setReadPersistentCacheCb(cb_read_persistent_cache_t cb)
{
    _set_read_persistent_cache_cb(cb);
}

void Three::setObfuscateSqlCb(cb_obfuscate_sql_t cb)
{
    _set_obfuscate_sql_cb(cb);
}

void Three::setObfuscateSqlExecPlanCb(cb_obfuscate_sql_exec_plan_t cb)
{
    _set_obfuscate_sql_exec_plan_cb(cb);
}

void Three::setGetProcessStartTimeCb(cb_get_process_start_time_t cb)
{
    _set_get_process_start_time_cb(cb);
}

void Three::setObfuscateMongoDBStringCb(cb_obfuscate_mongodb_string_t cb)
{
    _set_obfuscate_mongodb_string_cb(cb);
}

void Three::setEmitAgentTelemetryCb(cb_emit_agent_telemetry_t cb)
{
    _set_emit_agent_telemetry_cb(cb);
}

// Python Helpers

// get_integration_list return a list of every datadog's wheels installed.
char *Three::getIntegrationList()
{
    PyObject *pyPackages = NULL;
    PyObject *pkgLister = NULL;
    PyObject *args = NULL;
    PyObject *packages = NULL;
    char *wheels = NULL;

    rtloader_gilstate_t state = GILEnsure();

    pyPackages = PyImport_ImportModule("datadog_checks.base.utils.agent.packages");
    if (pyPackages == NULL) {
        setError("could not import datadog_checks.base.utils.agent.packages: " + _fetchPythonError());
        goto done;
    }

    pkgLister = PyObject_GetAttrString(pyPackages, "get_datadog_wheels");
    if (pkgLister == NULL) {
        setError("could not fetch get_datadog_wheels attr: " + _fetchPythonError());
        goto done;
    }

    args = PyTuple_New(0);
    if (args == NULL) {
        setError("could not initialize args to empty tuple: " + _fetchPythonError());
        goto done;
    }

    packages = PyObject_Call(pkgLister, args, NULL);
    if (packages == NULL) {
        setError("error fetching wheels list: " + _fetchPythonError());
        goto done;
    }

    if (PyList_Check(packages) == 0) {
        setError("'get_datadog_wheels' did not return a list");
        goto done;
    }

    wheels = as_yaml(packages);
    if (wheels == NULL) {
        setError("'packages' could not be serialized to yaml: " + _fetchPythonError());
        goto done;
    }

done:
    Py_XDECREF(pyPackages);
    Py_XDECREF(pkgLister);
    Py_XDECREF(args);
    Py_XDECREF(packages);
    GILRelease(state);

    return wheels;
}

// getInterpreterMemoryUsage return a dict with the python interpreters memory
// usage snapshot. The returned dict must be freed by calling....
char *Three::getInterpreterMemoryUsage()
{
    PyObject *pyMemory = NULL;
    PyObject *memSummary = NULL;
    PyObject *args = NULL;
    PyObject *summary = NULL;
    char *memUsage = NULL;

    rtloader_gilstate_t state = GILEnsure();

    pyMemory = PyImport_ImportModule(_PY_MEM_MODULE);
    if (pyMemory == NULL) {
        setError("could not import " _PY_MEM_MODULE ": " + _fetchPythonError());
        goto done;
    }

    memSummary = PyObject_GetAttrString(pyMemory, _PY_MEM_SUMMARY_FUNC);
    if (memSummary == NULL) {
        setError("could not fetch " _PY_MEM_SUMMARY_FUNC " attr: " + _fetchPythonError());
        goto done;
    }

    args = PyTuple_New(0);
    summary = PyObject_Call(memSummary, args, NULL);
    if (summary == NULL) {
        setError("error fetching interpreter memory usage: " + _fetchPythonError());
        goto done;
    }

    if (PyDict_Check(summary) == 0) {
        setError("'" _PY_MEM_SUMMARY_FUNC "' did not return a dictionary");
        goto done;
    }

    memUsage = as_yaml(summary);

done:
    Py_XDECREF(summary);
    Py_XDECREF(args);
    Py_XDECREF(memSummary);
    Py_XDECREF(pyMemory);
    GILRelease(state);

    return memUsage;
}
