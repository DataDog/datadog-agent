// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "three.h"

#include "constants.h"

// we only populate the fields `m_base` and `m_name`, we don't need any of the rest since we're doing Single-phase
// initialization
static struct PyModuleDef def__util
    = { PyModuleDef_HEAD_INIT, datadog_agent_six__util, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit__util(void) { return PyModule_Create(&def__util); }

static struct PyModuleDef def_aggregator
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_aggregator, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_aggregator(void) { return PyModule_Create(&def_aggregator); }

static struct PyModuleDef def_containers
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_containers, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_containers(void) { return PyModule_Create(&def_containers); }

static struct PyModuleDef def_datadog_agent
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_datadog_agent, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_datadog_agent(void) { return PyModule_Create(&def_datadog_agent); }

static struct PyModuleDef def_kubeutil
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_kubeutil, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_kubeutil(void) { return PyModule_Create(&def_kubeutil); }

static struct PyModuleDef def_tagger
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_tagger, NULL, -1, NULL, NULL, NULL, NULL, NULL };
PyMODINIT_FUNC PyInit_tagger(void) { return PyModule_Create(&def_tagger); }

static struct PyModuleDef def_util
    = { PyModuleDef_HEAD_INIT, datadog_agent_six_util, NULL, -1, NULL, NULL, NULL, NULL, NULL };
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
    def__util.m_methods = &_modules[DATADOG_AGENT_SIX__UTIL][0];
    PyImport_AppendInittab(datadog_agent_six__util, &PyInit__util);

    // aggregator
    def_aggregator.m_methods = &_modules[DATADOG_AGENT_SIX_AGGREGATOR][0];
    PyImport_AppendInittab(datadog_agent_six_aggregator, &PyInit_aggregator);

    // containers
    def_containers.m_methods = &_modules[DATADOG_AGENT_SIX_CONTAINERS][0];
    PyImport_AppendInittab(datadog_agent_six_containers, &PyInit_containers);

    // datadog_agent
    def_datadog_agent.m_methods = &_modules[DATADOG_AGENT_SIX_DATADOG_AGENT][0];
    PyImport_AppendInittab(datadog_agent_six_datadog_agent, &PyInit_datadog_agent);

    // kubeutil
    def_kubeutil.m_methods = &_modules[DATADOG_AGENT_SIX_KUBEUTIL][0];
    PyImport_AppendInittab(datadog_agent_six_kubeutil, &PyInit_kubeutil);

    // tagger
    def_tagger.m_methods = &_modules[DATADOG_AGENT_SIX_TAGGER][0];
    PyImport_AppendInittab(datadog_agent_six_tagger, &PyInit_tagger);

    // util
    def_util.m_methods = &_modules[DATADOG_AGENT_SIX_UTIL][0];
    PyImport_AppendInittab(datadog_agent_six_util, &PyInit_util);

    Py_SetPythonHome(_pythonHome);
    Py_Initialize();
}

bool Three::isInitialized() const { return Py_IsInitialized(); }

const char *Three::getPyVersion() const { return Py_GetVersion(); }

int Three::addModuleFunction(six_module_t module, six_module_func_t t, const char *funcName, void *func) {
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
        PyMethodDef guard = { NULL, NULL, 0, NULL };
        _modules[module].push_back(guard);
    }

    // insert at beginning so we keep guard at the end
    _modules[module].insert(_modules[module].begin(), def);

    return 1;
}

int Three::runSimpleString(const char *code) const { return PyRun_SimpleString(code); }

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

// return new reference
PyObject *Three::_importFrom(const char *module, const char *name) {
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

    return obj_symbol;

error:
    Py_XDECREF(obj_module);
    Py_XDECREF(obj_symbol);
    return NULL;
}
