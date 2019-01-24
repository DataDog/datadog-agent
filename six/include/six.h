// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_SIX_H
#define DATADOG_AGENT_SIX_SIX_H
#include <string>

struct SixPyObject {};

class Six {
public:
    enum MethType {
        NOARGS=0,
        ARGS,
        KEYWORDS
    };

    Six() {};
    virtual ~Six() {};

    // Public API
    virtual void init(const char* pythonHome) = 0;
    virtual void addModuleFunction(const char* module, const char* funcName,
                                   void* func, MethType t) = 0;

    // Public Const API
    virtual const char* getPyVersion() const = 0;
    virtual void runAnyFile(const char* path) const = 0;
    virtual SixPyObject* getNone() const = 0;
};

typedef Six* create_t();
typedef void destroy_t(Six*);

#endif
