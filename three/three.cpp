#include "three.h"
#include "constants.h"

#include <iostream>

static wchar_t *_pythonHome;


Three::~Three()
{
    PyMem_RawFree((void*)_pythonHome);
    Py_Finalize();
}

void Three::init(const char* pythonHome)
{
    Py_Initialize();

    if (pythonHome == NULL) {
        _pythonHome = Py_DecodeLocale(_defaultPythonHome, NULL);
    } else {
        _pythonHome = Py_DecodeLocale(pythonHome, NULL);
    }

    Py_SetPythonHome(_pythonHome);
}

const char* Three::getPyVersion() const
{
    return Py_GetVersion();
}

void Three::addModuleFunction(const char* module, const char* funcName,
                              void* func, Three::MethType t)
{

}
