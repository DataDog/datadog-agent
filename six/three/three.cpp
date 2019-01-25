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

}

int Three::runSimpleFile(const char* path) const
{
    FILE* fp = fopen(path, "r");
    if (!fp) {
        std::cerr << "error opening file: " << path << std::endl;
        return -1;
    }

    return PyRun_SimpleFileEx(fp, path, 1);  // automatically closes the file
}
