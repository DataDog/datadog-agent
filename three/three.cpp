// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"

#include "aggregator.h"
#include "constants.h"

#include <algorithm>
#include <sstream>

extern "C" DATADOG_AGENT_SIX_API Six *create() {
    return new Three();
}

extern "C" DATADOG_AGENT_SIX_API void destroy(Six *p) {
    delete p;
}

PyModuleConstants Three::ModuleConstants;

// we only populate the fields `m_base` and `m_name`, we don't need any of the
// rest since we're doing Single-phase initialization
//
// INIT_PYTHON_MODULE creates the def_<moduleName> (a PyModuleDef struct) and
// the needed PyInit_<moduleName> callback.
#define INIT_PYTHON_MODULE(moduleID, moduleName)                                                                       \
    static struct PyModuleDef def_##moduleName                                                                         \
        = { PyModuleDef_HEAD_INIT, datadog_agent_six_##moduleName, NULL, -1, NULL, NULL, NULL, NULL, NULL };           \
    PyMODINIT_FUNC PyInit_##moduleName(void) {                                                                         \
        PyObject *m = PyModule_Create(&def_##moduleName);                                                              \
        PyModuleConstants::iterator it = Three::ModuleConstants.find(moduleID);                                        \
        if (it != Three::ModuleConstants.end()) {                                                                      \
            std::vector<PyModuleConst>::iterator cit;                                                                  \
            for (cit = it->second.begin(); cit != it->second.end(); ++cit) {                                           \
                PyModule_AddIntConstant(m, cit->first.c_str(), cit->second);                                           \
            }                                                                                                          \
        }                                                                                                              \
        return m;                                                                                                      \
    }

// APPEND_TO_PYTHON_INITTAB set the module methods and call
// PyImport_AppendInittab with it, allowing Python to import it
#define APPEND_TO_PYTHON_INITTAB(moduleID, moduleName)                                                                 \
    {                                                                                                                  \
        if (_modules[moduleID].size() > 0) {                                                                           \
            def_##moduleName.m_methods = &_modules[moduleID][0];                                                       \
            if (PyImport_AppendInittab(getExtensionModuleName(moduleID), &PyInit_##moduleName) == -1) {                \
                setError("PyImport_AppendInittab failed to append " #moduleName);                                      \
                return false;                                                                                          \
            }                                                                                                          \
        }                                                                                                              \
    }

// initializing all Python C module
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX_DATADOG_AGENT, datadog_agent)
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX__UTIL, _util)
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX_UTIL, util)
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX_CONTAINERS, containers)
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX_KUBEUTIL, kubeutil)
INIT_PYTHON_MODULE(DATADOG_AGENT_SIX_TAGGER, tagger)

Three::~Three() {
    if (_pythonHome) {
        PyMem_RawFree((void *)_pythonHome);
    }
    Py_XDECREF(_baseClass);
    Py_Finalize();
    ModuleConstants.clear();
}

bool Three::init(const char *pythonHome) {
    // insert module to Python inittab one by one
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX_DATADOG_AGENT, datadog_agent)
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX__UTIL, _util)
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX_UTIL, util)
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX_CONTAINERS, containers)
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX_KUBEUTIL, kubeutil)
    APPEND_TO_PYTHON_INITTAB(DATADOG_AGENT_SIX_TAGGER, tagger)

    PyImport_AppendInittab("aggregator", PyInit_aggregator);

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
    _baseClass = _importFrom("datadog_checks.base.checks", "AgentCheck");

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

bool Three::addModuleFunction(six_module_t module, six_module_func_t t, const char *funcName, void *func) {
    if (getExtensionModuleName(module) == getUnknownModuleName()) {
        setError("Unknown ExtensionModule value");
        return false;
    }

    int ml_flags;
    switch (t) {
    case DATADOG_AGENT_SIX_NOARGS:
        ml_flags = METH_NOARGS;
        break;
    case DATADOG_AGENT_SIX_ARGS:
        ml_flags = METH_VARARGS;
        break;
    case DATADOG_AGENT_SIX_KEYWORDS:
        ml_flags = METH_VARARGS | METH_KEYWORDS;
        break;
    default:
        setError("Unknown MethType value");
        return false;
    }

    PyMethodDef def = { funcName, (PyCFunction)func, ml_flags, "" };

    if (_modules.find(module) == _modules.end()) {
        _modules[module] = PyMethods();
        // add the guard
        PyMethodDef guard = { NULL, NULL, 0, NULL };
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);

    return true;
}

bool Three::addModuleIntConst(six_module_t moduleID, const char *name, long value) {
    if (ModuleConstants.find(moduleID) == ModuleConstants.end()) {
        ModuleConstants[moduleID] = std::vector<PyModuleConst>();
    }

    ModuleConstants[moduleID].push_back(std::make_pair(std::string(name), value));
    return true;
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

bool Three::getCheck(const char *module, const char *init_config_str, const char *instances_str, SixPyObject *&pycheck,
                     char *&version) {
    PyObject *obj_module = NULL;
    PyObject *klass = NULL;
    PyObject *init_config = NULL;
    PyObject *instances = NULL;
    PyObject *check = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;

    char load_config[] = "load_config";
    char format[] = "(s)";

    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        std::ostringstream err;
        err << "unable to import module '" << module << "': " + _fetchPythonError();
        setError(err.str());
        goto done;
    }

    // find a subclass of the base check
    klass = _findSubclassOf(_baseClass, obj_module);
    if (klass == NULL) {
        std::ostringstream err;
        err << "unable to find a subclass of the base check in module '" << module << "': " << _fetchPythonError();
        setError(err.str());
        goto done;
    }

    // try to get Check version
    version = _getCheckVersion(obj_module);

    // call `AgentCheck.load_config(init_config)`
    init_config = PyObject_CallMethod(klass, load_config, format, init_config_str);
    if (init_config == NULL) {
        setError("error parsing init_config: " + _fetchPythonError());
        goto done;
    }

    // call `AgentCheck.load_config(instances)`
    instances = PyObject_CallMethod(klass, load_config, format, instances_str);
    if (instances == NULL) {
        setError("error parsing instances: " + _fetchPythonError());
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

done:
    Py_XDECREF(obj_module);
    Py_XDECREF(klass);
    Py_XDECREF(init_config);
    Py_XDECREF(instances);
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

char *Three::_getCheckVersion(PyObject *module) const {
    if (module == NULL) {
        return NULL;
    }

    char *ret = NULL;
    PyObject *py_version = NULL;
    PyObject *py_version_bytes = NULL;
    char version_field[] = "__version__";

    // try getting module.__version__
    py_version = PyObject_GetAttrString(module, version_field);
    if (py_version != NULL && PyUnicode_Check(py_version)) {
        py_version_bytes = PyUnicode_AsEncodedString(py_version, "UTF-8", "strict");
        if (py_version_bytes == NULL) {
            setError("error converting __version__ to string: " + _fetchPythonError());
            ret = NULL;
            goto done;
        }
        ret = _strdup(PyBytes_AsString(py_version_bytes));
        goto done;
    } else {
        // we expect __version__ might not be there, don't clutter the error stream
        PyErr_Clear();
    }

done:
    Py_XDECREF(py_version);
    Py_XDECREF(py_version_bytes);
    return ret;
}

void Three::setSubmitMetricCb(cb_submit_metric_t cb) {
    _set_submit_metric_cb(cb);
}
