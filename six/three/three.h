// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_H
#define DATADOG_AGENT_SIX_THREE_H
#include <vector>
#include <map>
#include <string>

#include <six.h>
#include <Python.h>

class Three: public Six {
public:
    Three(): _modules() {};
    ~Three();

    virtual void init(const char* pythonHome);
    virtual void addModuleFunction(const char* module, const char* funcName,
                                   void* func, MethType t);

    const char* getPyVersion() const;
    void runAnyFile(const char* path) const {}
    SixPyObject* getNone() const { return reinterpret_cast<SixPyObject*>(Py_None); }

private:
    typedef std::vector<PyMethodDef> PyMethods;
    typedef std::map<std::string, PyMethods> PyModules;

    PyModules _modules;
};

#ifdef __cplusplus
extern "C" {
#endif

    extern Six* create()
    {
        return new Three();
    }

    extern void destroy(Six* p)
    {
        delete p;
    }

#ifdef __cplusplus
}
#endif
#endif
