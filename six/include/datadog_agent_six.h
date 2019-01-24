#ifndef DATADOG_AGENT_SIX_H_INCLUDED
#define DATADOG_AGENT_SIX_H_INCLUDED

#ifndef DATADOG_AGENT_SIX_API
#  ifdef DATADOG_AGENT_SIX_TEST
#     define DATADOG_AGENT_SIX_API
#  elif _WIN32
#     define DATADOG_AGENT_SIX_API __declspec(dllexport)
#  else
#     if __GNUC__ >= 4
#         define DATADOG_AGENT_SIX_API __attribute__((visibility("default")))
#     else
#         define DATADOG_AGENT_SIX_API
#     endif
#  endif
#endif


#ifdef __cplusplus
extern "C" {
#endif

    struct six_s;
    typedef struct six_s six_t;

    struct six_pyobject_s;
    typedef struct six_pyobject_s six_pyobject_t;

    // FACTORIES
    DATADOG_AGENT_SIX_API six_t* make2();
    DATADOG_AGENT_SIX_API void destroy2(six_t*);
    DATADOG_AGENT_SIX_API six_t* make3();
    DATADOG_AGENT_SIX_API void destroy3(six_t*);

    // C API
    DATADOG_AGENT_SIX_API void init(six_t*, char*);
    DATADOG_AGENT_SIX_API void add_module_func_noargs(six_t*, char*, char*, void*);
    DATADOG_AGENT_SIX_API void add_module_func_args(six_t*, char*, char*, void*);
    DATADOG_AGENT_SIX_API void add_module_func_keywords(six_t*, char*, char*, void*);

    // C CONST API
    DATADOG_AGENT_SIX_API six_pyobject_t* get_none(const six_t*);
    DATADOG_AGENT_SIX_API const char* get_py_version(const six_t*);
    DATADOG_AGENT_SIX_API void run_any_file(const six_t*, const char* path);

#ifdef __cplusplus
}
#endif
#endif
