// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"

#include "constants.h"

#include <aggregator.h>
#include <datadog_agent.h>

#include <algorithm>
#include <sstream>

extern "C" DATADOG_AGENT_SIX_API Six *create() {
    return new Three();
}

extern "C" DATADOG_AGENT_SIX_API void destroy(Six *p) {
    delete p;
}

Three::~Three() {
    PyEval_RestoreThread(_threadState);
    if (_pythonHome) {
        PyMem_RawFree((void *)_pythonHome);
    }
    Py_XDECREF(_baseClass);
    Py_Finalize();
}

bool Three::init(const char *pythonHome) {
    // add custom builtins init funcs to Python inittab, one by one
    PyImport_AppendInittab("aggregator", PyInit_aggregator);
    PyImport_AppendInittab("datadog_agent", PyInit_datadog_agent);

    if (pythonHome == NULL) {
        _pythonHome = Py_DecodeLocale(_defaultPythonHome, NULL);
    } else {
        if (_pythonHome) {
            PyMem_RawFree((void *)_pythonHome);
        }
        _pythonHome = Py_DecodeLocale(pythonHome, NULL);
    }

    Py_SetPythonHome(_pythonHome);
    Py_Initialize();

    // Set PYTHONPATH
    if (_pythonPaths.size()) {
        char pathchr[] = "path";
        PyObject *path = PySys_GetObject(pathchr); // borrowed
        for (PyPaths::iterator pit = _pythonPaths.begin(); pit != _pythonPaths.end(); ++pit) {
            PyObject *p = PyUnicode_FromString((*pit).c_str());
            PyList_Append(path, p);
            Py_XDECREF(p);
        }
    }

    // load the base class
    _baseClass = _importFrom("datadog_checks.checks", "AgentCheck");

    // save tread state and release the GIL
    _threadState = PyEval_SaveThread();
    return _baseClass != NULL;
}

bool Three::isInitialized() const {
    return Py_IsInitialized();
}

const char *Three::getPyVersion() const {
    return Py_GetVersion();
}

bool Three::runSimpleString(const char *code) const {
    return PyRun_SimpleString(code) == 0;
}

bool Three::addPythonPath(const char *path) {
    if (std::find(_pythonPaths.begin(), _pythonPaths.end(), path) == _pythonPaths.end()) {
        _pythonPaths.push_back(path);
        return true;
    }
    return false;
}

six_gilstate_t Three::GILEnsure() {
    PyGILState_STATE state = PyGILState_Ensure();
    if (state == PyGILState_LOCKED) {
        return DATADOG_AGENT_SIX_GIL_LOCKED;
    }
    return DATADOG_AGENT_SIX_GIL_UNLOCKED;
}

void Three::GILRelease(six_gilstate_t state) {
    if (state == DATADOG_AGENT_SIX_GIL_LOCKED) {
        PyGILState_Release(PyGILState_LOCKED);
    } else {
        PyGILState_Release(PyGILState_UNLOCKED);
    }
}

bool Three::getClass(const char *module, SixPyObject *&pyModule, SixPyObject *&pyClass) {
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
        std::ostringstream err;
        err << "unable to find a subclass of the base check in module '" << module << "': " << _fetchPythonError();
        setError(err.str());
        Py_XDECREF(obj_module);
        return false;
    }

    pyModule = reinterpret_cast<SixPyObject *>(obj_module);
    pyClass = reinterpret_cast<SixPyObject *>(obj_class);
    return true;
}

bool Three::getCheck(SixPyObject *py_class, const char *init_config_str, const char *instance_str,
                     const char *agent_config_str, const char *check_id, SixPyObject *&pycheck) {

    PyObject *klass = reinterpret_cast<PyObject *>(py_class);
    PyObject *init_config = NULL;
    PyObject *instance = NULL;
    PyObject *instances = NULL;
    PyObject *check = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;
    PyObject *py_check_id = NULL;

    char load_config[] = "load_config";
    char format[] = "(s)";

    // call `AgentCheck.load_config(init_config)`
    init_config = PyObject_CallMethod(klass, load_config, format, init_config_str);
    if (init_config == NULL) {
        setError("error parsing init_config: " + _fetchPythonError());
        goto done;
    }

    // call `AgentCheck.load_config(instance)`
    printf("instance: %s\n", instance_str);
    instance = PyObject_CallMethod(klass, load_config, format, instance_str);
    if (instance == NULL) {
        setError("error parsing instance: " + _fetchPythonError());
        goto done;
    }

    instances = PyTuple_New(1);
    if (PyTuple_SetItem(instances, 0, instance) != 0) {
        setError("Could not create Tuple for instances: " + _fetchPythonError());
        goto done;
    }

    // create `args` and `kwargs` to invoke `AgentCheck` constructor
    args = PyTuple_New(0);
    kwargs = PyDict_New();
    PyDict_SetItemString(kwargs, "init_config", init_config);
    PyDict_SetItemString(kwargs, "instances", instances);

    // call `AgentCheck` constructor
    check = PyObject_Call(klass, args, kwargs);
    if (check == NULL) {
        setError("error creating check instance: " + _fetchPythonError());
        goto done;
    }

    if (check_id != NULL && strlen(check_id) != 0) {
        py_check_id = PyUnicode_FromString(check_id);
        if (py_check_id == NULL) {
            std::ostringstream err;
            err << "error could not set check_id: " << check_id;
            setError(err.str());
            Py_XDECREF(check);
            check = NULL;
            goto done;
        }

        if (PyObject_SetAttrString(check, "check_id", py_check_id) != 0) {
            setError("error could not set 'check_id' attr: " + _fetchPythonError());
            Py_XDECREF(check);
            check = NULL;
            goto done;
        }
    }

done:
    Py_XDECREF(py_check_id);
    Py_XDECREF(init_config);
    Py_XDECREF(instance);
    Py_XDECREF(args);
    Py_XDECREF(kwargs);

    if (check == NULL) {
        return false;
    }

    pycheck = reinterpret_cast<SixPyObject *>(check);
    return true;
}

//
const char *Three::runCheck(SixPyObject *check) {
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret, *ret_copy = NULL;
    char run[] = "run";
    PyObject *result = NULL, *bytes = NULL;

    result = PyObject_CallMethod(py_check, run, NULL);
    if (result == NULL || !PyUnicode_Check(result)) {
        setError("error invoking 'run' method: " + _fetchPythonError());
        goto done;
    }

    bytes = PyUnicode_AsEncodedString(result, "UTF-8", "strict");
    if (bytes == NULL) {
        setError("error converting result to string: " + _fetchPythonError());
        goto done;
    }

    // `ret` points to the Python string internal storage and will be eventually
    // deallocated along with the corresponding Python object.
    ret = PyBytes_AsString(bytes);
    ret_copy = _strdup(ret);
    Py_XDECREF(bytes);

done:
    Py_XDECREF(result);
    return ret_copy;
}

// return new reference
PyObject *Three::_importFrom(const char *module, const char *name) {
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

PyObject *Three::_findSubclassOf(PyObject *base, PyObject *module) {
    if (base == NULL || !PyType_Check(base)) {
        setError("base class is not of type 'Class'");
        return NULL;
    }

    if (module == NULL || !PyModule_Check(module)) {
        setError("module is not of type 'Module'");
        return NULL;
    }

    PyObject *dir = PyObject_Dir(module);
    if (dir == NULL) {
        setError("there was an error calling dir() on module object");
        return NULL;
    }

    PyObject *klass = NULL;
    for (int i = 0; i < PyList_GET_SIZE(dir); i++) {
        // get symbol name
        std::string symbol_name;
        PyObject *symbol = PyList_GetItem(dir, i);
        if (symbol != NULL) {
            PyObject *bytes = PyUnicode_AsEncodedString(symbol, "UTF-8", "strict");

            if (bytes != NULL) {
                symbol_name = PyBytes_AsString(bytes);
                Py_XDECREF(bytes);
            } else {
                continue;
            }
        } else {
            // Gets exception reason
            PyObject *reason = PyUnicodeDecodeError_GetReason(PyExc_IndexError);

            // Clears exception and sets error
            PyException_SetTraceback(PyExc_IndexError, Py_None);
            setError((const char *)PyBytes_AsString(reason));
            goto done;
        }

        klass = PyObject_GetAttrString(module, symbol_name.c_str());
        if (klass == NULL) {
            continue;
        }

        // Not a class, ignore
        if (!PyType_Check(klass)) {
            Py_XDECREF(klass);
            continue;
        }

        // Unrelated class, ignore
        if (!PyType_IsSubtype((PyTypeObject *)klass, (PyTypeObject *)base)) {
            Py_XDECREF(klass);
            continue;
        }

        // `klass` is actually `base` itself, ignore
        if (PyObject_RichCompareBool(klass, base, Py_EQ) == 1) {
            Py_XDECREF(klass);
            continue;
        }

        // does `klass` have subclasses?
        char func_name[] = "__subclasses__";
        PyObject *children = PyObject_CallMethod(klass, func_name, NULL);
        if (children == NULL) {
            Py_XDECREF(klass);
            continue;
        }

        // how many?
        int children_count = PyList_GET_SIZE(children);
        Py_XDECREF(children);

        // Agent integrations are supposed to have no subclasses
        if (children_count > 0) {
            Py_XDECREF(klass);
            continue;
        }

        // got it, return the check class
        goto done;
    }

    setError("cannot find a subclass");
    klass = NULL;

done:
    Py_DECREF(dir);
    return klass;
}

std::string Three::_fetchPythonError() const {
    std::string ret_val = "";

    if (PyErr_Occurred() == NULL) {
        return ret_val;
    }

    PyObject *ptype = NULL;
    PyObject *pvalue = NULL;
    PyObject *ptraceback = NULL;

    // Fetch error and make sure exception values are normalized, as per python C API docs.
    PyErr_Fetch(&ptype, &pvalue, &ptraceback);
    PyErr_NormalizeException(&ptype, &pvalue, &ptraceback);

    // There's a traceback, try to format it nicely
    if (ptraceback != NULL) {
        PyObject *traceback = PyImport_ImportModule("traceback");
        if (traceback != NULL) {
            char fname[] = "format_exception";
            PyObject *format_exception = PyObject_GetAttrString(traceback, fname);
            if (format_exception != NULL) {
                PyObject *fmt_exc = PyObject_CallFunctionObjArgs(format_exception, ptype, pvalue, ptraceback, NULL);
                if (fmt_exc != NULL) {
                    // "format_exception" returns a list of strings (one per line)
                    for (int i = 0; i < PyList_Size(fmt_exc); i++) {
                        PyObject *temp_bytes = PyUnicode_AsEncodedString(PyList_GetItem(fmt_exc, i), "UTF-8", "strict");
                        ret_val += PyBytes_AS_STRING(temp_bytes);
                        Py_XDECREF(temp_bytes);
                    }
                }
                Py_XDECREF(fmt_exc);
                Py_XDECREF(format_exception);
            }
            Py_XDECREF(traceback);
        } else {
            // If we reach this point, there was an error while formatting the exception
            ret_val = "can't format exception";
        }
    }
    // we sometimes do not get a traceback but an error in pvalue
    else if (pvalue != NULL) {
        PyObject *pvalue_obj = PyObject_Str(pvalue);
        if (pvalue_obj != NULL) {
            PyObject *temp_bytes = PyUnicode_AsEncodedString(pvalue_obj, "UTF-8", "strict");
            ret_val = PyBytes_AS_STRING(temp_bytes);
            Py_XDECREF(pvalue_obj);
            Py_XDECREF(temp_bytes);
        }
    } else if (ptype != NULL) {
        PyObject *ptype_obj = PyObject_Str(ptype);
        if (ptype_obj != NULL) {
            PyObject *temp_bytes = PyUnicode_AsEncodedString(ptype_obj, "UTF-8", "strict");
            ret_val = PyBytes_AS_STRING(temp_bytes);
            Py_XDECREF(ptype_obj);
            Py_XDECREF(temp_bytes);
        }
    }

    if (ret_val == "") {
        ret_val = "unknown error";
    }

    // clean up and return the string
    PyErr_Clear();
    Py_XDECREF(ptype);
    Py_XDECREF(pvalue);
    Py_XDECREF(ptraceback);
    return ret_val;
}

bool Three::getAttrString(SixPyObject *obj, const char *attributeName, char *&value) const {
    if (obj == NULL) {
        return false;
    }

    bool res = false;
    PyObject *py_attr = NULL;
    PyObject *py_attr_bytes = NULL;
    PyObject *py_obj = reinterpret_cast<PyObject *>(obj);

    py_attr = PyObject_GetAttrString(py_obj, attributeName);
    if (py_attr != NULL && PyUnicode_Check(py_attr)) {
        py_attr_bytes = PyUnicode_AsEncodedString(py_attr, "UTF-8", "strict");
        if (py_attr_bytes == NULL) {
            setError("error converting attribute " + std::string(attributeName) + " to string: " + _fetchPythonError());
        } else {
            value = _strdup(PyBytes_AsString(py_attr_bytes));
            res = true;
        }
    } else if (py_attr != NULL && !PyUnicode_Check(py_attr)) {
        setError("error attribute " + std::string(attributeName) + " is has a different type than unicode");
        PyErr_Clear();
    } else {
        PyErr_Clear();
    }

    Py_XDECREF(py_attr_bytes);
    Py_XDECREF(py_attr);
    return res;
}

void Three::decref(SixPyObject *obj) {
    Py_XDECREF(reinterpret_cast<PyObject *>(obj));
}

void Three::setSubmitMetricCb(cb_submit_metric_t cb) {
    _set_submit_metric_cb(cb);
}

void Three::setSubmitServiceCheckCb(cb_submit_service_check_t cb) {
    _set_submit_service_check_cb(cb);
}

void Three::setSubmitEventCb(cb_submit_event_t cb) {
    _set_submit_event_cb(cb);
}

void Three::setGetVersionCb(cb_get_version_t cb) {
    _set_get_version_cb(cb);
}

void Three::setGetConfigCb(cb_get_config_t cb) {
    _set_get_config_cb(cb);
}

void Three::setHeadersCb(cb_headers_t cb) {
    _set_headers_cb(cb);
}

void Three::setGetHostnameCb(cb_get_hostname_t cb) {
    _set_get_hostname_cb(cb);
}

void Three::setGetClusternameCb(cb_get_clustername_t cb) {
    _set_get_clustername_cb(cb);
}

void Three::setLogCb(cb_log_t cb) {
    _set_log_cb(cb);
}
