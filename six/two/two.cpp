// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "two.h"

#include "constants.h"

#include <_util.h>
#include <aggregator.h>
#include <cgo_free.h>
#include <containers.h>
#include <datadog_agent.h>
#include <kubeutil.h>
#include <six_types.h>
#include <stringutils.h>
#include <tagger.h>
#include <util.h>

#include <algorithm>
#include <sstream>

extern "C" DATADOG_AGENT_SIX_API Six *create(const char *pythonHome)
{
    return new Two(pythonHome);
}

extern "C" DATADOG_AGENT_SIX_API void destroy(Six *p)
{
    delete p;
}

Two::Two(const char *python_home)
    : Six()
    , _baseClass(NULL)
    , _pythonPaths()
{
    initPythonHome(python_home);
}

Two::~Two()
{
    PyEval_RestoreThread(_threadState);
    Py_XDECREF(_baseClass);
    Py_Finalize();
}

void Two::initPythonHome(const char *pythonHome)
{
    if (pythonHome != NULL && strlen(pythonHome) != 0) {
        _pythonHome = pythonHome;
    }

    Py_SetPythonHome(const_cast<char *>(_pythonHome));
}

bool Two::init()
{

    Py_Initialize();

    // In recent versions of Python3 this is called from Py_Initialize already,
    // for Python2 it has to be explicit.
    PyEval_InitThreads();

    // init custom builtins
    Py2_init_aggregator();
    Py2_init_datadog_agent();
    Py2_init_util();
    Py2_init__util();
    Py2_init_tagger();
    Py2_init_kubeutil();
    Py2_init_containers();

    // Set PYTHONPATH
    if (_pythonPaths.size()) {
        char pathchr[] = "path";
        PyObject *path = PySys_GetObject(pathchr); // borrowed
        for (PyPaths::iterator pit = _pythonPaths.begin(); pit != _pythonPaths.end(); ++pit) {
            PyObject *p = PyString_FromString((*pit).c_str());
            PyList_Append(path, p);
            Py_XDECREF(p);
        }
    }

    // import the base class
    _baseClass = _importFrom("datadog_checks.checks", "AgentCheck");

    // save tread state and release the GIL
    _threadState = PyEval_SaveThread();

    return _baseClass != NULL;
}

bool Two::isInitialized() const
{
    return Py_IsInitialized();
}

py_info_t *Two::getPyInfo()
{
    PyObject *sys = NULL;
    PyObject *path = NULL;
    PyObject *str_path = NULL;

    py_info_t *info = (py_info_t *)malloc(sizeof(*info));
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

bool Two::runSimpleString(const char *code) const
{
    return PyRun_SimpleString(code) == 0;
}

bool Two::addPythonPath(const char *path)
{
    if (std::find(_pythonPaths.begin(), _pythonPaths.end(), path) == _pythonPaths.end()) {
        _pythonPaths.push_back(path);
        return true;
    }
    return false;
}

six_gilstate_t Two::GILEnsure()
{
    PyGILState_STATE state = PyGILState_Ensure();
    if (state == PyGILState_LOCKED) {
        return DATADOG_AGENT_SIX_GIL_LOCKED;
    }
    return DATADOG_AGENT_SIX_GIL_UNLOCKED;
}

void Two::GILRelease(six_gilstate_t state)
{
    if (state == DATADOG_AGENT_SIX_GIL_LOCKED) {
        PyGILState_Release(PyGILState_LOCKED);
    } else {
        PyGILState_Release(PyGILState_UNLOCKED);
    }
}

// return new reference
PyObject *Two::_importFrom(const char *module, const char *name)
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

    Py_XDECREF(obj_module);
    return obj_symbol;

error:
    Py_XDECREF(obj_module);
    Py_XDECREF(obj_symbol);
    return NULL;
}

bool Two::getClass(const char *module, SixPyObject *&pyModule, SixPyObject *&pyClass)
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

bool Two::getCheck(SixPyObject *py_class, const char *init_config_str, const char *instance_str,
                   const char *check_id_str, const char *check_name, const char *agent_config_str, SixPyObject *&check)
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
    name = PyString_FromString(check_name);
    PyDict_SetItemString(kwargs, "name", name);
    PyDict_SetItemString(kwargs, "init_config", init_config);
    PyDict_SetItemString(kwargs, "instances", instances);

    if (agent_config_str != NULL) {
        agent_config = PyObject_CallMethod(klass, load_config, format, agent_config_str);
        if (agent_config == NULL) {
            setError("error parsing agent_config: " + _fetchPythonError());
            goto done;
        } else if (!PyDict_Check(agent_config)) {
            setError("error agent_config is not a dict");
            goto done;
        }

        PyDict_SetItemString(kwargs, "agentConfig", agent_config);
    }

    // call `AgentCheck` constructor
    py_check = PyObject_Call(klass, args, kwargs);
    if (py_check == NULL) {
        setError(_fetchPythonError());
        goto done;
    }

    if (check_id_str != NULL && strlen(check_id_str) != 0) {
        check_id = PyString_FromString(check_id_str);
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
    Py_XDECREF(name);
    Py_XDECREF(check_id);
    Py_XDECREF(init_config);
    Py_XDECREF(instance);
    Py_XDECREF(agent_config);
    Py_XDECREF(args);
    Py_XDECREF(kwargs);

    if (py_check == NULL) {
        return false;
    }

    check = reinterpret_cast<SixPyObject *>(py_check);
    return true;
}

PyObject *Two::_findSubclassOf(PyObject *base, PyObject *module)
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

        // get symbol name
        char *symbol_name = PyString_AsString(symbol);
        if (symbol_name == NULL) {
            // PyString_AsString returns NULL if `symbol` is not a string object
            // and raises TypeError. Let's clear the error and keep going.
            PyErr_Clear();
            continue;
        }

        // get symbol instance. It's a new ref but in case of success we don't
        // DecRef since we return it and the caller will be owner
        klass = PyObject_GetAttrString(module, symbol_name);
        if (klass == NULL) {
            PyErr_Clear();
            continue;
        }

        // not a class, ignore
        if (!PyType_Check(klass)) {
            continue;
        }

        // this is an unrelated class, ignore
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

const char *Two::runCheck(SixPyObject *check)
{
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret = NULL;
    char *ret_copy = NULL;
    char run[] = "run";
    PyObject *result = NULL;

    result = PyObject_CallMethod(py_check, run, NULL);
    if (result == NULL) {
        setError("error invoking 'run' method: " + _fetchPythonError());
        goto done;
    }

    // `ret` points to the Python string internal storage and will be eventually
    // deallocated along with the corresponding Python object.
    ret = PyString_AsString(result);
    if (ret == NULL) {
        setError("error converting result to string: " + _fetchPythonError());
        goto done;
    }

    ret_copy = _strdup(ret);

done:
    Py_XDECREF(result);
    return ret_copy;
}

char **Two::getCheckWarnings(SixPyObject *check)
{
    if (check == NULL)
        return NULL;
    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    char func_name[] = "get_warnings";
    PyObject *warns_list = PyObject_CallMethod(py_check, func_name, NULL);
    if (warns_list == NULL) {
        setError("error invoking 'get_warnings' method: " + _fetchPythonError());
        return NULL;
    }

    Py_ssize_t numWarnings = PyList_Size(warns_list);
    char **warnings = (char **)malloc(sizeof(*warnings) * (numWarnings + 1));
    if (!warnings) {
        Py_XDECREF(warns_list);
        setError("could not allocate memory to get warnings: ");
        return NULL;
    }
    warnings[numWarnings] = NULL;

    for (Py_ssize_t idx = 0; idx < numWarnings; idx++) {
        PyObject *warn = PyList_GetItem(warns_list, idx); // borrowed ref
        warnings[idx] = as_string(warn);
    }
    Py_XDECREF(warns_list);
    return warnings;
}

std::string Two::_fetchPythonError()
{
    std::string ret_val = "";

    if (PyErr_Occurred() == NULL) {
        return ret_val;
    }

    PyObject *ptype = NULL;
    PyObject *pvalue = NULL;
    PyObject *ptraceback = NULL;

    // Fetch error and make sure exception values are normalized, as per python C
    // API docs.
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
                        ret_val += PyString_AsString(PyList_GetItem(fmt_exc, i));
                    }
                }
                Py_XDECREF(fmt_exc);
                Py_XDECREF(format_exception);
            }
            Py_XDECREF(traceback);
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
            ret_val = PyString_AsString(pvalue_obj);
            Py_XDECREF(pvalue_obj);
        }
    } else if (ptype != NULL) {
        PyObject *ptype_obj = PyObject_Str(ptype);
        if (ptype_obj != NULL) {
            ret_val = PyString_AsString(ptype_obj);
            Py_XDECREF(ptype_obj);
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

bool Two::getAttrString(SixPyObject *obj, const char *attributeName, char *&value) const
{
    if (obj == NULL) {
        return false;
    }

    bool res = false;
    PyObject *py_attr = NULL;
    PyObject *py_obj = reinterpret_cast<PyObject *>(obj);

    py_attr = PyObject_GetAttrString(py_obj, attributeName);
    if (py_attr != NULL && PyString_Check(py_attr)) {
        value = as_string(py_attr);
        res = true;
    } else if (py_attr != NULL && !PyString_Check(py_attr)) {
        setError("error attribute " + std::string(attributeName) + " is has a different type than string");
        PyErr_Clear();
    } else {
        PyErr_Clear();
    }

    Py_XDECREF(py_attr);
    return res;
}

void Two::decref(SixPyObject *obj)
{
    Py_XDECREF(reinterpret_cast<PyObject *>(obj));
}

void Two::incref(SixPyObject *obj)
{
    Py_XINCREF(reinterpret_cast<SixPyObject *>(obj));
}

void Two::set_module_attr_string(char *module, char *attr, char *value)
{
    PyObject *py_module = PyImport_ImportModule(module);
    if (!py_module) {
        setError("error importing python '" + std::string(module) + "' module: " + _fetchPythonError());
        return;
    }

    PyObject *py_value = PyStringFromCString(value);
    if (PyObject_SetAttrString(py_module, attr, py_value) != 0)
        setError("error setting the '" + std::string(module) + "." + std::string(attr)
                 + "' attribute: " + _fetchPythonError());

    Py_XDECREF(py_module);
    Py_XDECREF(py_value);
}

void Two::setSubmitMetricCb(cb_submit_metric_t cb)
{
    _set_submit_metric_cb(cb);
}

void Two::setSubmitServiceCheckCb(cb_submit_service_check_t cb)
{
    _set_submit_service_check_cb(cb);
}

void Two::setSubmitEventCb(cb_submit_event_t cb)
{
    _set_submit_event_cb(cb);
}

void Two::setGetVersionCb(cb_get_version_t cb)
{
    _set_get_version_cb(cb);
}

void Two::setGetConfigCb(cb_get_config_t cb)
{
    _set_get_config_cb(cb);
}

void Two::setHeadersCb(cb_headers_t cb)
{
    _set_headers_cb(cb);
}

void Two::setGetHostnameCb(cb_get_hostname_t cb)
{
    _set_get_hostname_cb(cb);
}

void Two::setGetClusternameCb(cb_get_clustername_t cb)
{
    _set_get_clustername_cb(cb);
}

void Two::setLogCb(cb_log_t cb)
{
    _set_log_cb(cb);
}

void Two::setSetExternalTagsCb(cb_set_external_tags_t cb)
{
    _set_set_external_tags_cb(cb);
}

void Two::setSubprocessOutputCb(cb_get_subprocess_output_t cb)
{
    _set_get_subprocess_output_cb(cb);
}

void Two::setCGOFreeCb(cb_cgo_free_t cb)
{
    _set_cgo_free_cb(cb);
}

void Two::setTagsCb(cb_tags_t cb)
{
    _set_tags_cb(cb);
}

void Two::setGetConnectionInfoCb(cb_get_connection_info_t cb)
{
    _set_get_connection_info_cb(cb);
}

void Two::setIsExcludedCb(cb_is_excluded_t cb)
{
    _set_is_excluded_cb(cb);
}

// Python Helpers

// get_integration_list return a list of every datadog's wheels installed. The
// returned list must be free by calling free_integration_list.
char *Two::getIntegrationList()
{
    PyObject *pyPackages = NULL;
    PyObject *pkgLister = NULL;
    PyObject *args = NULL;
    PyObject *packages = NULL;
    char *wheels = NULL;

    six_gilstate_t state = GILEnsure();

    pyPackages = PyImport_ImportModule("datadog_checks.base.utils.agent.packages");
    if (pyPackages == NULL) {
        setError("could not import datadog_checks.base.utils.agent.packages: " + _fetchPythonError());
        goto done;
    }

    pkgLister = PyObject_GetAttrString(pyPackages, "get_datadog_wheels");
    if (pyPackages == NULL) {
        setError("could not fetch get_datadog_wheels attr: " + _fetchPythonError());
        goto done;
    }

    args = PyTuple_New(0);
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

done:
    Py_XDECREF(pyPackages);
    Py_XDECREF(pkgLister);
    Py_XDECREF(args);
    Py_XDECREF(packages);
    GILRelease(state);

    return wheels;
}
