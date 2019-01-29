// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"
#include "constants.h"

#include <iostream>


Three::~Three()
{
    if (_pythonHome) {
        PyMem_RawFree((void*)_pythonHome);
    }
    Py_Finalize();
}

void Three::init(const char* pythonHome)
{
    if (pythonHome == NULL) {
        _pythonHome = Py_DecodeLocale(_defaultPythonHome, NULL);
    } else {
        if (_pythonHome) {
           PyMem_RawFree((void*)_pythonHome);
        }
        _pythonHome = Py_DecodeLocale(pythonHome, NULL);
    }

    Py_SetPythonHome(_pythonHome);
    Py_Initialize();

    PyModules::iterator it;
    for (it = _modules.begin(); it != _modules.end(); ++it) {
        PyModuleDef mod_def = {
            PyModuleDef_HEAD_INIT, // m_base
            it->first.c_str(), // m_name
            NULL, // m_doc
            -1, // m_size
            &_modules[it->first][0], // m_methods
            NULL, NULL, NULL, NULL // not needed, we're doing Single-phase initialization
        };
        PyObject *module = PyModule_Create(&mod_def);
        PyState_AddModule(module, &mod_def);
    }
}

bool Three::isInitialized() const
{
    return Py_IsInitialized();
}

const char* Three::getPyVersion() const
{
    return Py_GetVersion();
}

void Three::addModuleFunction(const char* module, const char* funcName,
                              void* func, Three::MethType t)
{
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
        PyMethodDef guard = {NULL, NULL, 0, NULL};
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);
}

int Three::runSimpleString(const char* code) const
{
    return PyRun_SimpleString(code);
}
