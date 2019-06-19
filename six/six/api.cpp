// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifdef _WIN32
#    include <Windows.h>
#else
#    include <dlfcn.h>
#endif

#include <iostream>
#include <sstream>

#include <datadog_agent_six.h>
#include <six.h>

#if __linux__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.so"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif __APPLE__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.dylib"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.dylib"
#elif _WIN32
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.dll"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.dll"
#else
#    error Platform not supported
#endif

#define AS_TYPE(Type, Obj) reinterpret_cast<Type *>(Obj)
#define AS_PTYPE(Type, Obj) reinterpret_cast<Type **>(Obj)
#define AS_CTYPE(Type, Obj) reinterpret_cast<const Type *>(Obj)

#ifdef _WIN32
static HMODULE six_backend = NULL;
#else
static void *six_backend = NULL;
#endif

#ifdef _WIN32

create_t *loadAndCreate(const char *dll, const char *python_home, char **error)
{
    // first, add python home to the directory search path for loading DLLs
    SetDllDirectoryA(python_home);

    // load library
    six_backend = LoadLibraryA(dll);
    if (!six_backend) {
        // printing to stderr might reset the error, get it now
        int err = GetLastError();
        std::ostringstream err_msg;
        err_msg << "Unable to open library " << dll << ", error code: " << err;
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }

    // dlsym class factory
    create_t *create = (create_t *)GetProcAddress(six_backend, "create");
    if (!create) {
        // printing to stderr might reset the error, get it now
        int err = GetLastError();
        std::ostringstream err_msg;
        err_msg << "Unable to open factory GPA: " << err;
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }
    return create;
}
six_t *make2(const char *python_home, char **error)
{
    create_t *create = loadAndCreate(DATADOG_AGENT_TWO, python_home, error);
    if (!create) {
        return NULL;
    }
    return AS_TYPE(six_t, create(python_home));
}

six_t *make3(const char *python_home, char **error)
{
    create_t *create_three = loadAndCreate(DATADOG_AGENT_THREE, python_home, error);
    if (!create_three) {
        return NULL;
    }
    return AS_TYPE(six_t, create_three(python_home));
}

void destroy(six_t *six)
{
    if (six_backend) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)GetProcAddress(six_backend, "destroy");

        if (!destroy) {
            std::cerr << "Unable to open 'three' destructor: " << GetLastError() << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
        six_backend = NULL;
    }
}

#else
six_t *make2(const char *python_home, char **error)
{
    if (six_backend != NULL) {
        std::string err_msg = "Six already initialized!";
        *error = strdup(err_msg.c_str());
        return NULL;
    }
    // load library
    six_backend = dlopen(DATADOG_AGENT_TWO, RTLD_LAZY | RTLD_GLOBAL);
    if (!six_backend) {
        std::ostringstream err_msg;
        err_msg << "Unable to open two library: " << dlerror();
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create = (create_t *)dlsym(six_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::ostringstream err_msg;
        err_msg << "Unable to open two factory: " << dlsym_error;
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }

    return AS_TYPE(six_t, create(python_home));
}

six_t *make3(const char *python_home, char **error)
{
    if (six_backend != NULL) {
        std::string err_msg = "Six already initialized!";
        *error = strdup(err_msg.c_str());
        return NULL;
    }

    // load the library
    six_backend = dlopen(DATADOG_AGENT_THREE, RTLD_LAZY | RTLD_GLOBAL);
    if (!six_backend) {
        std::ostringstream err_msg;
        err_msg << "Unable to open three library: " << dlerror();
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create_three = (create_t *)dlsym(six_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::ostringstream err_msg;
        err_msg << "Unable to open three factory: " << dlsym_error;
        *error = strdup(err_msg.str().c_str());
        return NULL;
    }

    return AS_TYPE(six_t, create_three(python_home));
}

void destroy(six_t *six)
{
    if (six_backend) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)dlsym(six_backend, "destroy");
        const char *dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to dlopen backend destructor: " << dlsym_error;
            return;
        }
        destroy(AS_TYPE(Six, six));
        six_backend = NULL;
    }
}
#endif

int init(six_t *six)
{
    return AS_TYPE(Six, six)->init() ? 1 : 0;
}

py_info_t *get_py_info(six_t *six)
{
    return AS_TYPE(Six, six)->getPyInfo();
}

int run_simple_string(const six_t *six, const char *code)
{
    return AS_CTYPE(Six, six)->runSimpleString(code) ? 1 : 0;
}

six_pyobject_t *get_none(const six_t *six)
{
    return AS_TYPE(six_pyobject_t, AS_CTYPE(Six, six)->getNone());
}

int add_python_path(six_t *six, const char *path)
{
    return AS_TYPE(Six, six)->addPythonPath(path) ? 1 : 0;
}

six_gilstate_t ensure_gil(six_t *six)
{
    return AS_TYPE(Six, six)->GILEnsure();
}

void release_gil(six_t *six, six_gilstate_t state)
{
    AS_TYPE(Six, six)->GILRelease(state);
}

int get_class(six_t *six, const char *name, six_pyobject_t **py_module, six_pyobject_t **py_class)
{
    return AS_TYPE(Six, six)->getClass(name, *AS_PTYPE(SixPyObject, py_module), *AS_PTYPE(SixPyObject, py_class)) ? 1
                                                                                                                  : 0;
}

int get_attr_string(six_t *six, six_pyobject_t *py_class, const char *attr_name, char **value)
{
    return AS_TYPE(Six, six)->getAttrString(AS_TYPE(SixPyObject, py_class), attr_name, *value);
}

int get_check(six_t *six, six_pyobject_t *py_class, const char *init_config, const char *instance, const char *check_id,
              const char *check_name, six_pyobject_t **check)
{
    return AS_TYPE(Six, six)->getCheck(AS_TYPE(SixPyObject, py_class), init_config, instance, check_id, check_name,
                                       NULL, *AS_PTYPE(SixPyObject, check))
        ? 1
        : 0;
}

int get_check_deprecated(six_t *six, six_pyobject_t *py_class, const char *init_config, const char *instance,
                         const char *agent_config, const char *check_id, const char *check_name, six_pyobject_t **check)
{
    return AS_TYPE(Six, six)->getCheck(AS_TYPE(SixPyObject, py_class), init_config, instance, check_id, check_name,
                                       agent_config, *AS_PTYPE(SixPyObject, check))
        ? 1
        : 0;
}

const char *run_check(six_t *six, six_pyobject_t *check)
{
    return AS_TYPE(Six, six)->runCheck(AS_TYPE(SixPyObject, check));
}

char **get_checks_warnings(six_t *six, six_pyobject_t *check)
{
    return AS_TYPE(Six, six)->getCheckWarnings(AS_TYPE(SixPyObject, check));
}

/*
 * error API
 */

int has_error(const six_t *six)
{
    return AS_CTYPE(Six, six)->hasError() ? 1 : 0;
}

const char *get_error(const six_t *six)
{
    return AS_CTYPE(Six, six)->getError();
}

void clear_error(six_t *six)
{
    AS_TYPE(Six, six)->clearError();
}

#ifndef WIN32
/*
 * C-land crash handling
 */

DATADOG_AGENT_SIX_API int handle_crashes(const six_t *six, const int enable)
{
    // enable implicit cast to bool
    return AS_CTYPE(Six, six)->handleCrashes(enable) ? 1 : 0;
}
#endif

/*
 * memory management
 */

void six_free(six_t *six, void *ptr)
{
    AS_TYPE(Six, six)->free(ptr);
}

void six_decref(six_t *six, six_pyobject_t *obj)
{
    AS_TYPE(Six, six)->decref(AS_TYPE(SixPyObject, obj));
}

void six_incref(six_t *six, six_pyobject_t *obj)
{
    AS_TYPE(Six, six)->incref(AS_TYPE(SixPyObject, obj));
}

void set_module_attr_string(six_t *six, char *module, char *attr, char *value)
{
    AS_TYPE(Six, six)->set_module_attr_string(module, attr, value);
}

/*
 * aggregator API
 */

void set_submit_metric_cb(six_t *six, cb_submit_metric_t cb)
{
    AS_TYPE(Six, six)->setSubmitMetricCb(cb);
}

void set_submit_service_check_cb(six_t *six, cb_submit_service_check_t cb)
{
    AS_TYPE(Six, six)->setSubmitServiceCheckCb(cb);
}

void set_submit_event_cb(six_t *six, cb_submit_event_t cb)
{
    AS_TYPE(Six, six)->setSubmitEventCb(cb);
}

/*
 * datadog_agent API
 */

void set_get_version_cb(six_t *six, cb_get_version_t cb)
{
    AS_TYPE(Six, six)->setGetVersionCb(cb);
}

void set_get_config_cb(six_t *six, cb_get_config_t cb)
{
    AS_TYPE(Six, six)->setGetConfigCb(cb);
}

void set_headers_cb(six_t *six, cb_headers_t cb)
{
    AS_TYPE(Six, six)->setHeadersCb(cb);
}

void set_get_hostname_cb(six_t *six, cb_get_hostname_t cb)
{
    AS_TYPE(Six, six)->setGetHostnameCb(cb);
}

void set_get_clustername_cb(six_t *six, cb_get_clustername_t cb)
{
    AS_TYPE(Six, six)->setGetClusternameCb(cb);
}

void set_log_cb(six_t *six, cb_log_t cb)
{
    AS_TYPE(Six, six)->setLogCb(cb);
}

void set_set_external_tags_cb(six_t *six, cb_set_external_tags_t cb)
{
    AS_TYPE(Six, six)->setSetExternalTagsCb(cb);
}

char *get_integration_list(six_t *six)
{
    return AS_TYPE(Six, six)->getIntegrationList();
}

/*
 * stringutils API
 */

DATADOG_AGENT_SIX_API int init_stringutils(const six_t *six)
{
    // enable implicit cast to bool
    return AS_CTYPE(Six, six)->initStringUtils() ? 1 : 0;
}

/*
 * _util API
 */
void set_get_subprocess_output_cb(six_t *six, cb_get_subprocess_output_t cb)
{
    AS_TYPE(Six, six)->setSubprocessOutputCb(cb);
}

/*
 * CGO API
 */
void set_cgo_free_cb(six_t *six, cb_cgo_free_t cb)
{
    AS_TYPE(Six, six)->setCGOFreeCb(cb);
}

/*
 * tagger API
 */
void set_tags_cb(six_t *six, cb_tags_t cb)
{
    AS_TYPE(Six, six)->setTagsCb(cb);
}

/*
 * kubeutil API
 */
void set_get_connection_info_cb(six_t *six, cb_get_connection_info_t cb)
{
    AS_TYPE(Six, six)->setGetConnectionInfoCb(cb);
}

/*
 * containers API
 */
void set_is_excluded_cb(six_t *six, cb_is_excluded_t cb)
{
    AS_TYPE(Six, six)->setIsExcludedCb(cb);
}
