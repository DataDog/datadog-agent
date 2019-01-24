#include "two.h"
#include "constants.h"

#include <iostream>


Two::~Two()
{
    Py_Finalize();
}

void Two::init(const char* pythonHome) {
    if (pythonHome != NULL) {
        _pythonHome = pythonHome;
    }

    Py_SetPythonHome(const_cast<char *>(_pythonHome));
    Py_InitializeEx(0);

    PyModules::iterator it;
    for (it = _modules.begin(); it != _modules.end(); ++it) {
        Py_InitModule(it->first.c_str(), &_modules[it->first][0]);
    }
}

const char* Two::getPyVersion() const
{
    return Py_GetVersion();
}

void Two::runAnyFile(const char* path) const
{
    FILE* fp = fopen(path, "r");
    if (!fp) {
        std::cerr << "error opening file: " << path << std::endl;
        return;
    }

    PyRun_AnyFile(fp, path);

    fclose(fp);
}

void Two::addModuleFunction(const char* module, const char* funcName,
                            void* func, MethType t)
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

    PyMethodDef guard = {NULL, NULL};

    if (_modules.find(module) == _modules.end()) {
        _modules[module] = PyMethods();
        // add the guard
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);
}
