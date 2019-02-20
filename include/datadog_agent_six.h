// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_H_INCLUDED
#define DATADOG_AGENT_SIX_H_INCLUDED
#include <six_types.h>


#ifdef __cplusplus
extern "C" {
#endif

struct six_s;
typedef struct six_s six_t;

struct six_pyobject_s;
typedef struct six_pyobject_s six_pyobject_t;

// FACTORIES
DATADOG_AGENT_SIX_API six_t *make2();
DATADOG_AGENT_SIX_API six_t *make3();

// C API
DATADOG_AGENT_SIX_API void destroy(six_t *);
DATADOG_AGENT_SIX_API int init(six_t *, char *);
DATADOG_AGENT_SIX_API int add_python_path(six_t *, const char *path);
DATADOG_AGENT_SIX_API int add_module_func(six_t *, six_module_t module, six_module_func_t func_type, char *func_name,
                                          void *func);
DATADOG_AGENT_SIX_API int add_module_int_const(six_t *, six_module_t module, const char *name, long value);
DATADOG_AGENT_SIX_API six_gilstate_t ensure_gil(six_t *);
DATADOG_AGENT_SIX_API void clear_error(six_t *);
DATADOG_AGENT_SIX_API void release_gil(six_t *, six_gilstate_t);
DATADOG_AGENT_SIX_API int get_check(six_t *, const char *name, const char *init_config, const char *instances,
                                    six_pyobject_t **check, char **version);
DATADOG_AGENT_SIX_API const char *run_check(six_t *, six_pyobject_t *check);

// C CONST API
DATADOG_AGENT_SIX_API int is_initialized(six_t *);
DATADOG_AGENT_SIX_API six_pyobject_t *get_none(const six_t *);
DATADOG_AGENT_SIX_API const char *get_py_version(const six_t *);
DATADOG_AGENT_SIX_API int run_simple_string(const six_t *, const char *code);
DATADOG_AGENT_SIX_API int has_error(const six_t *);
DATADOG_AGENT_SIX_API const char *get_error(const six_t *);

#ifdef __cplusplus
}
#endif
#endif
