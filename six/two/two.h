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
        , _module_constants()
        , _baseClass(NULL)
        , _pythonPaths(){};
    ~Two();

    bool init(const char *pythonHome);
    bool addModuleFunction(six_module_t module, six_module_func_t t, const char *funcName, void *func);
    bool addModuleIntConst(six_module_t module, const char *name, long value);
    bool addPythonPath(const char *path);
    six_gilstate_t GILEnsure();
    void GILRelease(six_gilstate_t);
    SixPyObject *getCheckClass(const char *module);
    bool getCheck(const char *module, const char *init_config, const char *instances, SixPyObject *&check,
                  char *&version);
    const char *runCheck(SixPyObject *check);
    void decref(SixPyObject *);

    // const API
    bool isInitialized() const;
    const char *getPyVersion() const;
    bool runSimpleString(const char *code) const;
    SixPyObject *getNone() const {
        return reinterpret_cast<SixPyObject *>(Py_None);
    }

    // Aggregator API
    void setSubmitMetricCb(cb_submit_metric_t);
    void setSubmitServiceCheckCb(cb_submit_service_check_t);

private:
    PyObject *_importFrom(const char *module, const char *name);
    PyObject *_findSubclassOf(PyObject *base, PyObject *moduleName);
    PyObject *_getClass(const char *module, const char *base);
    std::string _fetchPythonError();
    char *_getCheckVersion(PyObject *module) const;

    typedef std::vector<PyMethodDef> PyMethods;
    typedef std::map<six_module_t, PyMethods> PyModules;
    typedef std::vector<std::string> PyPaths;

    PyModules _modules;
    PyModuleConstants _module_constants;
    PyObject *_baseClass;
    PyPaths _pythonPaths;
    PyThreadState *_state;
};

#endif
