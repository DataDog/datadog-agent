// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "two.h"

#include "constants.h"

Two::~Two() { Py_Finalize(); }

void Two::init(const char *pythonHome) {
    if (pythonHome != NULL) {
        _pythonHome = pythonHome;
    }

    Py_SetPythonHome(const_cast<char *>(_pythonHome));
    Py_Initialize();

    PyModules::iterator it;
    for (it = _modules.begin(); it != _modules.end(); ++it) {
        six_module_t module = it->first;
        PyObject *m = Py_InitModule(getExtensionModuleName(module), &_modules[module][0]);
        if (_module_constants.find(module) == _module_constants.end()) {
            std::vector<PyModuleConst>::iterator cit;
            for (cit = _module_constants[module].begin(); cit != _module_constants[module].begin(); ++cit) {
                PyModule_AddIntConstant(m, cit->first.c_str(), cit->second);
            }
        }
    }

    // In recent versions of Python3 this is called from Py_Initialize already,
    // for Python2 it has to be explicit.
    PyEval_InitThreads();
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
SixPyObject *Two::importFrom(const char *module, const char *name) {
    PyObject *obj_module, *obj_symbol;

    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        PyErr_Print();
        setError("Unable to import module");
        goto error;
    }

    obj_symbol = PyObject_GetAttrString(obj_module, name);
    if (obj_symbol == NULL) {
        setError("Unable to load symbol");
        goto error;
    }

    return reinterpret_cast<SixPyObject *>(obj_symbol);

error:
    Py_XDECREF(obj_module);
    Py_XDECREF(obj_symbol);
    return NULL;
}
