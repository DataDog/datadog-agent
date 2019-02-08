// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"

#include "constants.h"

static struct PyModuleDef datadog_agent_def = {
    PyModuleDef_HEAD_INIT, // m_base
    "datadog_agent", // m_name
    NULL, // m_doc
    -1, // m_size
    NULL, // m_methods, will be filled later
    NULL,
    NULL,
    NULL,
    NULL // not needed, we're doing Single-phase initialization
};

PyMODINIT_FUNC PyInit_datadog_agent(void) { return PyModule_Create(&datadog_agent_def); }

Three::~Three() {
    if (_pythonHome) {
        PyMem_RawFree((void *)_pythonHome);
    }
    Py_Finalize();
}

void Three::init(const char *pythonHome) {
    if (pythonHome == NULL) {
        _pythonHome = Py_DecodeLocale(_defaultPythonHome, NULL);
    } else {
        if (_pythonHome) {
            PyMem_RawFree((void *)_pythonHome);
        }
        _pythonHome = Py_DecodeLocale(pythonHome, NULL);
    }

    // init builtin modules one by one
    datadog_agent_def.m_methods = &_modules[DATADOG_AGENT][0];
    PyImport_AppendInittab(getExtensionModuleName(DATADOG_AGENT).c_str(), &PyInit_datadog_agent);

    Py_SetPythonHome(_pythonHome);
    Py_Initialize();
}

bool Three::isInitialized() const { return Py_IsInitialized(); }

const char *Three::getPyVersion() const { return Py_GetVersion(); }

int Three::addModuleFunction(ExtensionModule module, MethType t, const char *funcName, void *func) {
    if (getExtensionModuleName(module) == getExtensionModuleUnknown()) {
        setError("Unknown ExtensionModule value");
        return -1;
    }

    int ml_flags;
    switch (t) {
    case Six::NOARGS:
        ml_flags = METH_NOARGS;
        break;
    case Six::ARGS:
        ml_flags = METH_VARARGS;
        break;
    case Six::KEYWORDS:
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
        PyMethodDef guard = { NULL, NULL, 0, NULL };
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);

    return 1;
}

int Three::runSimpleString(const char *code) const { return PyRun_SimpleString(code); }

Six::GILState Three::GILEnsure() {
    PyGILState_STATE state = PyGILState_Ensure();
    if (state == PyGILState_LOCKED) {
        return Six::GIL_LOCKED;
    }
    return Six::GIL_UNLOCKED;
}

void Three::GILRelease(Six::GILState state) {
    if (state == Six::GIL_LOCKED) {
        PyGILState_Release(PyGILState_LOCKED);
    } else {
        PyGILState_Release(PyGILState_UNLOCKED);
    }
}
