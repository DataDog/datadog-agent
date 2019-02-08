// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_H_INCLUDED
#define DATADOG_AGENT_SIX_H_INCLUDED

#ifndef DATADOG_AGENT_SIX_API
#    ifdef DATADOG_AGENT_SIX_TEST
#        define DATADOG_AGENT_SIX_API
#    elif _WIN32
#        define DATADOG_AGENT_SIX_API __declspec(dllexport)
#    else
#        if __GNUC__ >= 4
#            define DATADOG_AGENT_SIX_API __attribute__((visibility("default")))
#        else
#            define DATADOG_AGENT_SIX_API
#        endif
#    endif
#endif

#ifdef __cplusplus
extern "C" {
#endif

struct six_s;
typedef struct six_s six_t;

struct six_pyobject_s;
typedef struct six_pyobject_s six_pyobject_t;

typedef enum six_gilstate_e { DATADOG_AGENT_SIX_GIL_LOCKED, DATADOG_AGENT_SIX_GIL_UNLOCKED } six_gilstate_t;

typedef enum six_module_e { DATADOG_AGENT_SIX_DATADOG_AGENT } six_module_t;

typedef enum six_module_func_e {
    DATADOG_AGENT_SIX_NOARGS,
    DATADOG_AGENT_SIX_ARGS,
    DATADOG_AGENT_SIX_KEYWORDS
} six_module_func_t;

// FACTORIES
DATADOG_AGENT_SIX_API six_t *make2();
DATADOG_AGENT_SIX_API void destroy2(six_t *);
DATADOG_AGENT_SIX_API six_t *make3();
DATADOG_AGENT_SIX_API void destroy3(six_t *);

// C API
DATADOG_AGENT_SIX_API void init(six_t *, char *);
DATADOG_AGENT_SIX_API int add_module_func(six_t *, six_module_t module, six_module_func_t func_type, char *func_name,
                                          void *func);
DATADOG_AGENT_SIX_API six_gilstate_t ensure_gil(six_t *);
DATADOG_AGENT_SIX_API void release_gil(six_t *, six_gilstate_t);

// C CONST API
DATADOG_AGENT_SIX_API int is_initialized(six_t *);
DATADOG_AGENT_SIX_API six_pyobject_t *get_none(const six_t *);
DATADOG_AGENT_SIX_API const char *get_py_version(const six_t *);
DATADOG_AGENT_SIX_API int run_simple_string(const six_t *, const char *path);
DATADOG_AGENT_SIX_API six_pyobject_t *import_from(const six_t *, const char *module, const char *name);

#ifdef __cplusplus
}
#endif
#endif
