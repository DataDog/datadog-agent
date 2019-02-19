// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "two.h"

#include "constants.h"

#include <sstream>

Two::~Two() { Py_Finalize(); }

bool Two::init(const char *pythonHome) {
    if (pythonHome != NULL) {
        _pythonHome = pythonHome;
    }

    Py_SetPythonHome(const_cast<char *>(_pythonHome));
    Py_Initialize();

    PyModules::iterator it;
    for (it = _modules.begin(); it != _modules.end(); ++it) {
        six_module_t module = it->first;
        PyObject *m = Py_InitModule(getExtensionModuleName(module), &_modules[module][0]);
        if (_module_constants.find(module) != _module_constants.end()) {
            std::vector<PyModuleConst>::iterator cit;
            for (cit = _module_constants[module].begin(); cit != _module_constants[module].end(); ++cit) {
                PyModule_AddIntConstant(m, cit->first.c_str(), cit->second);
            }
        }
    }

    // In recent versions of Python3 this is called from Py_Initialize already,
    // for Python2 it has to be explicit.
    PyEval_InitThreads();
    return true;
}

bool Two::isInitialized() const { return Py_IsInitialized(); }

const char *Two::getPyVersion() const { return Py_GetVersion(); }

int Two::runSimpleString(const char *code) const { return PyRun_SimpleString(code); }

int Two::addModuleFunction(six_module_t module, six_module_func_t t, const char *funcName, void *func) {
    if (getExtensionModuleName(module) == getUnknownModuleName()) {
        setError("Unknown ExtensionModule value");
        return -1;
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
        return -1;
    }

    PyMethodDef def = { funcName, (PyCFunction)func, ml_flags, "" };

    if (_modules.find(module) == _modules.end()) {
        _modules[module] = PyMethods();
        // add the guard
        PyMethodDef guard = { NULL, NULL };
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);

    // success
    return 0;
}

int Two::addModuleIntConst(six_module_t module, const char *name, long value) {
    if (_module_constants.find(module) == _module_constants.end()) {
        _module_constants[module] = std::vector<PyModuleConst>();
    }

    _module_constants[module].push_back(std::make_pair(std::string(name), value));
    return 1; // ok
}

six_gilstate_t Two::GILEnsure() {
    PyGILState_STATE state = PyGILState_Ensure();
    if (state == PyGILState_LOCKED) {
        return DATADOG_AGENT_SIX_GIL_LOCKED;
    }
    return DATADOG_AGENT_SIX_GIL_UNLOCKED;
}

void Two::GILRelease(six_gilstate_t state) {
    if (state == DATADOG_AGENT_SIX_GIL_LOCKED) {
        PyGILState_Release(PyGILState_LOCKED);
    } else {
        PyGILState_Release(PyGILState_UNLOCKED);
    }
}

// return new reference
PyObject *Two::_importFrom(const char *module, const char *name) {
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

SixPyObject *Two::getCheckClass(const char *module) {
    PyObject *base = NULL;
    PyObject *obj_module = NULL;
    PyObject *klass = NULL;

    // import the base class
    base = _importFrom("datadog_checks.base.checks", "AgentCheck");
    if (base == NULL) {
        std::string old_err = getError();
        setError("Unable to import the base class: " + old_err);
        goto done;
    }

    // try to import python module containing the check
    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        std::ostringstream err;
        err << "unable to import module '" << module << "': " + _fetchPythonError();
        setError(err.str());
        goto done;
    }

    // find a subclass of the base check
    klass = _findSubclassOf(base, obj_module);
    if (klass == NULL) {
        std::ostringstream err;
        err << "unable to find a subclass of the base check in module '" << module << "': " << _fetchPythonError();
        setError(err.str());
        goto done;
    }

done:
    Py_XDECREF(base);
    Py_XDECREF(obj_module);
    Py_XDECREF(klass);

    if (klass == NULL) {
        return NULL;
    }

    return reinterpret_cast<SixPyObject *>(klass);
}

SixPyObject *Two::getCheck(const char *module, const char *init_config_str, const char *instances_str) {
    PyObject *klass = NULL;
    PyObject *init_config = NULL;
    PyObject *instances = NULL;
    PyObject *check = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;

    char load_config[] = "load_config";
    char format[] = "(s)"; // use parentheses to force Tuple creation

    // Gets Check class from module
    klass = reinterpret_cast<PyObject *>(getCheckClass(module));
    if (klass == NULL) {
        // Error is already set by getCheckClass if class is not found
        goto done;
    }

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
    Py_XDECREF(klass);
    Py_XDECREF(init_config);
    Py_XDECREF(instances);
    Py_XDECREF(args);
    Py_XDECREF(kwargs);

    if (check == NULL) {
        return NULL;
    }

    return reinterpret_cast<SixPyObject *>(check);
}

PyObject *Two::_findSubclassOf(PyObject *base, PyObject *module) {
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
        setError("there was an error calling dir() on module object");
        return NULL;
    }

    PyObject *klass = NULL;
    for (int i = 0; i < PyList_GET_SIZE(dir); i++) {
        // get symbol name
        char *symbol_name;
        PyObject *symbol = PyList_GetItem(dir, i);
        if (symbol != NULL) {
            symbol_name = PyString_AsString(symbol);
        }

        // get symbol instance. It's a new ref but in case of success we don't DecRef since we return it and the caller
        // will be owner
        klass = PyObject_GetAttrString(module, symbol_name);
        if (klass == NULL) {
            continue;
        }

        // not a class, ignore
        if (!PyType_Check(klass)) {
            Py_XDECREF(klass);
            continue;
        }

        // this is an unrelated class, ignore
        if (!PyType_IsSubtype((PyTypeObject *)klass, (PyTypeObject *)base)) {
            Py_XDECREF(klass);
            continue;
        }

        // `klass` is actually `base` itself, ignore
        if (PyObject_RichCompareBool(klass, base, Py_EQ)) {
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
    Py_XDECREF(dir);
    return klass;
}

const char *Two::runCheck(SixPyObject *check) {
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret, *ret_copy = NULL;
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

    ret_copy = strdup(ret);

done:
    Py_XDECREF(result);
    return ret_copy;
}

std::string Two::_fetchPythonError() {
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
                        ret_val += PyString_AsString(PyList_GetItem(fmt_exc, i));
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
