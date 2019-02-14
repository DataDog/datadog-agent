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
    if (_modules[DATADOG_AGENT_SIX__UTIL].size() > 0) {
        def__util.m_methods = &_modules[DATADOG_AGENT_SIX__UTIL][0];
        PyImport_AppendInittab(datadog_agent_six__util, &PyInit__util);
    }

    // aggregator
    if (_modules[DATADOG_AGENT_SIX_AGGREGATOR].size() > 0) {
        def_aggregator.m_methods = &_modules[DATADOG_AGENT_SIX_AGGREGATOR][0];
        PyImport_AppendInittab(datadog_agent_six_aggregator, &PyInit_aggregator);
    }

    // containers
    if (_modules[DATADOG_AGENT_SIX_CONTAINERS].size() > 0) {
        def_containers.m_methods = &_modules[DATADOG_AGENT_SIX_CONTAINERS][0];
        PyImport_AppendInittab(datadog_agent_six_containers, &PyInit_containers);
    }

    // datadog_agent
    if (_modules[DATADOG_AGENT_SIX_DATADOG_AGENT].size() > 0) {
        def_datadog_agent.m_methods = &_modules[DATADOG_AGENT_SIX_DATADOG_AGENT][0];
        PyImport_AppendInittab(datadog_agent_six_datadog_agent, &PyInit_datadog_agent);
    }

    // kubeutil
    if (_modules[DATADOG_AGENT_SIX_KUBEUTIL].size() > 0) {
        def_kubeutil.m_methods = &_modules[DATADOG_AGENT_SIX_KUBEUTIL][0];
        PyImport_AppendInittab(datadog_agent_six_kubeutil, &PyInit_kubeutil);
    }

    // tagger
    if (_modules[DATADOG_AGENT_SIX_TAGGER].size() > 0) {
        def_tagger.m_methods = &_modules[DATADOG_AGENT_SIX_TAGGER][0];
        PyImport_AppendInittab(datadog_agent_six_tagger, &PyInit_tagger);
    }
    // util
    if (_modules[DATADOG_AGENT_SIX_UTIL].size() > 0) {
        def_util.m_methods = &_modules[DATADOG_AGENT_SIX_UTIL][0];
        PyImport_AppendInittab(datadog_agent_six_util, &PyInit_util);
    }

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

SixPyObject *Three::getCheck(const char *module, const char *init_config_str, const char *instances_str) {
    PyObject *base = NULL;
    PyObject *obj_module = NULL;
    PyObject *klass = NULL;
    PyObject *init_config = NULL;
    PyObject *instances = NULL;
    PyObject *check = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;

    char load_config[] = "load_config";
    char format[] = "(s)";

    base = _importFrom("datadog_checks.base.checks", "AgentCheck");
    if (base == NULL) {
        setError("Unable to import base class");
        goto done;
    }

    obj_module = PyImport_ImportModule(module);
    if (obj_module == NULL) {
        PyErr_Print();
        setError("Unable to import module");
        goto done;
    }

    // find a subclass of the base check
    klass = _findSubclassOf(base, obj_module);
    if (klass == NULL) {
        PyErr_Print();
        goto done;
    }

    // call `AgentCheck.load_config(init_config)`
    init_config = PyObject_CallMethod(klass, load_config, format, init_config_str);
    if (init_config == NULL) {
        PyErr_Print();
        goto done;
    }

    // call `AgentCheck.load_config(instances)`
    instances = PyObject_CallMethod(klass, load_config, format, instances_str);
    if (instances == NULL) {
        PyErr_Print();
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
        PyErr_Print();
        goto done;
    }

done:
    Py_XDECREF(base);
    Py_XDECREF(obj_module);
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

const char *Three::runCheck(SixPyObject *check) {
    if (check == NULL) {
        return NULL;
    }

    PyObject *py_check = reinterpret_cast<PyObject *>(check);

    // result will be eventually returned as a copy and the corresponding Python
    // string decref'ed, caller will be responsible for memory deallocation.
    char *ret, *ret_copy = NULL;
    char run[] = "run";
    PyObject *result, *bytes = NULL;

    result = PyObject_CallMethod(py_check, run, NULL);
    if (result == NULL || !PyUnicode_Check(result)) {
        PyErr_Print();
        goto done;
    }

    bytes = PyUnicode_AsEncodedString(result, "UTF-8", "strict");
    if (bytes == NULL) {
        PyErr_Print();
        goto done;
    }

    // `ret` points to the Python string internal storage and will be eventually
    // deallocated along with the corresponding Python object.
    ret = PyBytes_AsString(bytes);
    ret_copy = strdup(ret);
    Py_XDECREF(bytes);

done:
    Py_XDECREF(result);
    return ret_copy;
}

// return new reference
PyObject *Three::_importFrom(const char *module, const char *name) {
    PyObject *obj_module, *obj_symbol;

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

    return obj_symbol;

error:
    Py_XDECREF(obj_module);
    Py_XDECREF(obj_symbol);
    return NULL;
}

PyObject *Three::_findSubclassOf(PyObject *base, PyObject *module) {
    if (base == NULL || !PyType_Check(base)) {
        setError("base class is not of type 'Class'");
        return NULL;
    }

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
        if (symbol != NULL || !PyUnicode_Check(symbol)) {
            PyObject *bytes = PyUnicode_AsEncodedString(symbol, "UTF-8", "strict");

            if (bytes != NULL) {
                symbol_name = strdup(PyBytes_AsString(bytes));
                Py_XDECREF(bytes);
            } else {
                continue;
            }
        }

        klass = PyObject_GetAttrString(module, symbol_name);
        if (klass == NULL) {
            continue;
        }

        // Not a class, ignore
        if (!PyType_Check(klass)) {
            Py_XDECREF(klass);
            continue;
        }

        // Unrelated class, ignore
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
    Py_DECREF(dir);
    return klass;
}

std::string Three::_fetchPythonError() {
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
                        PyObject *temp_bytes = PyUnicode_AsEncodedString(PyList_GetItem(fmt_exc, i), "UTF-8", "strict");
                        ret_val += PyBytes_AS_STRING(temp_bytes);
                        Py_XDECREF(temp_bytes);
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
            PyObject *temp_bytes = PyUnicode_AsEncodedString(pvalue_obj, "UTF-8", "strict");
            ret_val = PyBytes_AS_STRING(temp_bytes);
            Py_XDECREF(pvalue_obj);
            Py_XDECREF(temp_bytes);
        }
    } else if (ptype != NULL) {
        PyObject *ptype_obj = PyObject_Str(ptype);
        if (ptype_obj != NULL) {
            PyObject *temp_bytes = PyUnicode_AsEncodedString(ptype_obj, "UTF-8", "strict");
            ret_val = PyBytes_AS_STRING(temp_bytes);
            Py_XDECREF(ptype_obj);
            Py_XDECREF(temp_bytes);
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
