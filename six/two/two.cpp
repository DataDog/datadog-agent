// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "two.h"
#include "constants.h"

#include <iostream>

Two::~Two() {
    Py_Finalize();
}

void Two::init(const char *pythonHome) {
    if (pythonHome != NULL) {
        _pythonHome = pythonHome;
    }

    Py_SetPythonHome(const_cast<char *>(_pythonHome));
    Py_Initialize();

    PyModules::iterator it;
    for (it = _modules.begin(); it != _modules.end(); ++it) {
        Py_InitModule(getExtensionModuleName(it->first), &_modules[it->first][0]);
    }

    // In Python3 this is called from Py_Initialize already
    PyEval_InitThreads();
}

bool Two::isInitialized() const {
    return Py_IsInitialized();
}

const char *Two::getPyVersion() const {
    return Py_GetVersion();
}

int Two::runSimpleString(const char *code) const {
    return PyRun_SimpleString(code);
}

int Two::addModuleFunction(ExtensionModule module, MethType t,
                           const char *funcName, void *func) {
    if (getExtensionModuleName(module) == "") {
        std::cerr << "Unknown ExtensionModule value" << std::endl;
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
        std::cerr << "Unknown MethType value" << std::endl;
        return -1;
    }

    PyMethodDef def = {
        funcName,
        (PyCFunction)func,
        ml_flags,
        ""
    };

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
