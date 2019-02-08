// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"

#include "builtins.h"
#include "constants.h"

// we only populate the fields `m_base` and `m_name`, we don't need any of the rest since we're doing Single-phase
// initialization
static struct PyModuleDef def__util
    = { PyModuleDef_HEAD_INIT, builtins::module__util.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit__util(void) { return PyModule_Create(&def__util); }

static struct PyModuleDef def_aggregator
    = { PyModuleDef_HEAD_INIT, builtins::module_aggregator.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_aggregator(void) { return PyModule_Create(&def_aggregator); }

static struct PyModuleDef def_containers
    = { PyModuleDef_HEAD_INIT, builtins::module_containers.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_containers(void) { return PyModule_Create(&def_containers); }

static struct PyModuleDef def_datadog_agent
    = { PyModuleDef_HEAD_INIT, builtins::module_datadog_agent.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_datadog_agent(void) { return PyModule_Create(&def_datadog_agent); }

static struct PyModuleDef def_kubeutil
    = { PyModuleDef_HEAD_INIT, builtins::module_kubeutil.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_kubeutil(void) { return PyModule_Create(&def_kubeutil); }

static struct PyModuleDef def_tagger
    = { PyModuleDef_HEAD_INIT, builtins::module_tagger.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_tagger(void) { return PyModule_Create(&def_tagger); }

static struct PyModuleDef def_util
    = { PyModuleDef_HEAD_INIT, builtins::module_util.c_str(), NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_util(void) { return PyModule_Create(&def_util); }

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

    // _util
    def__util.m_methods = &_modules[builtins::MODULE__UTIL][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE__UTIL).c_str(), &PyInit__util);

    // aggregator
    def_aggregator.m_methods = &_modules[builtins::MODULE_AGGREGATOR][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_AGGREGATOR).c_str(), &PyInit_aggregator);

    // containers
    def_containers.m_methods = &_modules[builtins::MODULE_CONTAINERS][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_CONTAINERS).c_str(), &PyInit_containers);

    // datadog_agent
    def_datadog_agent.m_methods = &_modules[builtins::MODULE_DATADOG_AGENT][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_DATADOG_AGENT).c_str(),
                           &PyInit_datadog_agent);

    // kubeutil
    def_kubeutil.m_methods = &_modules[builtins::MODULE_KUBEUTIL][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_KUBEUTIL).c_str(), &PyInit_kubeutil);

    // tagger
    def_tagger.m_methods = &_modules[builtins::MODULE_TAGGER][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_TAGGER).c_str(), &PyInit_tagger);

    // util
    def_util.m_methods = &_modules[builtins::MODULE_UTIL][0];
    PyImport_AppendInittab(builtins::getExtensionModuleName(builtins::MODULE_UTIL).c_str(), &PyInit_util);

    Py_SetPythonHome(_pythonHome);
    Py_Initialize();
}

bool Three::isInitialized() const { return Py_IsInitialized(); }

const char *Three::getPyVersion() const { return Py_GetVersion(); }

int Three::addModuleFunction(builtins::ExtensionModule module, MethType t, const char *funcName, void *func) {
    if (builtins::getExtensionModuleName(module) == builtins::module_unknown) {
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
