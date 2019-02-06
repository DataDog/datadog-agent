// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_H
#define DATADOG_AGENT_SIX_THREE_H
#include <map>
#include <string>
#include <vector>

#include <Python.h>
#include <six.h>

class Three : public Six {
public:
    Three()
        : _modules()
        , _pythonHome(NULL) {};
    ~Three();

    void init(const char *pythonHome);
    int addModuleFunction(ExtensionModule module, MethType t, const char *funcName, void *func);

    // const API
    bool isInitialized() const;
    const char *getPyVersion() const;
    int runSimpleString(const char *path) const;
    SixPyObject *getNone() const { return reinterpret_cast<SixPyObject *>(Py_None); }

private:
    typedef std::vector<PyMethodDef> PyMethods;
    typedef std::map<ExtensionModule, PyMethods> PyModules;

    PyModules _modules;
    wchar_t *_pythonHome;
};

#ifdef __cplusplus
extern "C" {
#endif

extern Six *create() { return new Three(); }

extern void destroy(Six *p) { delete p; }

#ifdef __cplusplus
}
#endif
#endif
