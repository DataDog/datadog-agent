// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_SIX_H
#define DATADOG_AGENT_SIX_SIX_H
#include <mutex>
#include <string>

#include "types.h"

// Opaque type to wrap PyObject
struct SixPyObject {};

class Six {
public:
    enum MethType { NOARGS = 0, ARGS, KEYWORDS };
    enum GILState { GIL_LOCKED = 0, GIL_UNLOCKED };

    Six() {};
    virtual ~Six() {};

    // Public API
    virtual void init(const char *pythonHome) = 0;
    virtual int addModuleFunction(six_module_t module, MethType t, const char *funcName, void *func) = 0;
    void setError(const std::string &msg);
    void clearError();
    virtual GILState GILEnsure() = 0;
    virtual void GILRelease(GILState) = 0;
    virtual SixPyObject *importFrom(const char *module, const char *name) = 0;

    // Public Const API
    virtual bool isInitialized() const = 0;
    virtual const char *getPyVersion() const = 0;
    virtual int runSimpleString(const char *code) const = 0;
    virtual SixPyObject *getNone() const = 0;
    std::string getError() const;
    bool hasError() const;

protected:
    const char *getExtensionModuleName(six_module_t m);
    const char *getUnknownModuleName();

private:
    std::string _error;
    mutable std::mutex _error_mtx;
};

typedef Six *create_t();
typedef void destroy_t(Six *);

#endif
