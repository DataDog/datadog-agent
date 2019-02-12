// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_TWO_H
#define DATADOG_AGENT_SIX_TWO_H
#include <map>
#include <string>
#include <vector>

#include <Python.h>
#include <six.h>

class Two : public Six {
public:
    Two()
        : Six()
        , _modules()
        , _module_constants() {};
    ~Two();

    void init(const char *pythonHome);
    int addModuleFunction(six_module_t module, six_module_func_t t, const char *funcName, void *func);
    int addModuleIntConst(six_module_t module, const char *name, long value);
    six_gilstate_t GILEnsure();
    void GILRelease(six_gilstate_t);
    SixPyObject *importFrom(const char *module, const char *name);
    SixPyObject *getCheckClass(const char *module) { return NULL; }
    SixPyObject *getCheck(const char *module, const char *init_config, const char *instances);

    // const API
    bool isInitialized() const;
    const char *getPyVersion() const;
    int runSimpleString(const char *code) const;
    SixPyObject *getNone() const { return reinterpret_cast<SixPyObject *>(Py_None); }

private:
    PyObject *_importFrom(const char *module, const char *name);
    PyObject *_findSubclassOf(PyObject *base, PyObject *moduleName);
    PyObject *_getClass(const char *module, const char *base);

    typedef std::vector<PyMethodDef> PyMethods;
    typedef std::map<six_module_t, PyMethods> PyModules;
    typedef std::pair<std::string, long> PyModuleConst;
    typedef std::map<six_module_t, std::vector<PyModuleConst> > PyModuleConstants;

    PyModules _modules;
    PyModuleConstants _module_constants;
};

#ifdef __cplusplus
extern "C" {
#endif

Six *create() { return new Two(); }

void destroy(Six *p) { delete p; }

#ifdef __cplusplus
}
#endif
#endif
