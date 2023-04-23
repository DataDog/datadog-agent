// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifdef _WIN32
#    include <Windows.h>
#else
#    include <dlfcn.h>
#endif

#ifndef _WIN32
// clang-format off
// handler stuff
#include <execinfo.h>
#include <csignal>
#include <cstring>
#include <sys/types.h>
#include <unistd.h>

// logging to cerr
#include <errno.h>
// clang-format on
#endif

#include <iostream>
#include <sstream>

#include "datadog_agent_rtloader.h"
#include "rtloader.h"
#include "rtloader_mem.h"

#if __linux__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.so"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif __APPLE__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.dylib"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.dylib"
#elif __FreeBSD__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.so"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
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
static HMODULE rtloader_backend = NULL;
#else
static void *rtloader_backend = NULL;
#endif

#ifdef _WIN32

/*! \fn create_t *loadAndCreate(const char *dll, const char *python_home, char **error)
    \brief Loads the Python backend DLL from the provided PYTHONHOME, and returns its
    creation routine.
    \param dll A C-string containing the expected backend DLL name.
    \param python_home A C-string containing the expected PYTHONHOME for said DLL.
    \param error A C-string pointer output parameter to return error messages.
    \return A create_t * function pointer that will allow us to create the relevant python
    backend. In case of failure NULL is returned and the error string is set on the output
    parameter.
    \sa create_t, make2, make3

    This function is windows only. Required by the backend "makers".
*/
create_t *loadAndCreate(const char *dll, const char *python_home, char **error)
{
    // first, add python home to the directory search path for loading DLLs
    SetDllDirectoryA(python_home);

    // load library
    rtloader_backend = LoadLibraryA(dll);
    if (!rtloader_backend) {
        // printing to stderr might reset the error, get it now
        int err = GetLastError();
        std::ostringstream err_msg;
        err_msg << "Unable to open library " << dll << ", error code: " << err;
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }

    // dlsym class factory
    create_t *create = (create_t *)GetProcAddress(rtloader_backend, "create");
    if (!create) {
        // printing to stderr might reset the error, get it now
        int err = GetLastError();
        std::ostringstream err_msg;
        err_msg << "Unable to open factory GPA: " << err;
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }
    return create;
}

rtloader_t *make2(const char *python_home, const char *python_exe, char **error)
{

    if (rtloader_backend != NULL) {
        *error = strdupe("RtLoader already initialized!");
        return NULL;
    }

    create_t *create = loadAndCreate(DATADOG_AGENT_TWO, python_home, error);
    if (!create) {
        return NULL;
    }
    return AS_TYPE(rtloader_t, create(python_home, python_exe, _get_memory_tracker_cb()));
}

rtloader_t *make3(const char *python_home, const char *python_exe, char **error)
{
    if (rtloader_backend != NULL) {
        *error = strdupe("RtLoader already initialized!");
        return NULL;
    }

    create_t *create_three = loadAndCreate(DATADOG_AGENT_THREE, python_home, error);
    if (!create_three) {
        return NULL;
    }
    return AS_TYPE(rtloader_t, create_three(python_home, python_exe, _get_memory_tracker_cb()));
}

/*! \fn void destroy(rtloader_t *rtloader)
    \brief Destructor function for the provided rtloader backend.
    \param rtloader_t A rtloader_t * pointer to the RtLoader instance we wish to destroy.
    \sa rtloader_t
*/
void destroy(rtloader_t *rtloader)
{
    if (rtloader_backend) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)GetProcAddress(rtloader_backend, "destroy");

        if (!destroy) {
            std::cerr << "Unable to open 'three' destructor: " << GetLastError() << std::endl;
            return;
        }
        destroy(AS_TYPE(RtLoader, rtloader));
        rtloader_backend = NULL;
    }
}

#else
rtloader_t *make2(const char *python_home, const char *python_exe, char **error)
{
    if (rtloader_backend != NULL) {
        std::string err_msg = "RtLoader already initialized!";
        *error = strdupe(err_msg.c_str());
        return NULL;
    }
    // load library
    rtloader_backend = dlopen(DATADOG_AGENT_TWO, RTLD_LAZY | RTLD_GLOBAL);
    if (!rtloader_backend) {
        std::ostringstream err_msg;
        err_msg << "Unable to open two library: " << dlerror();
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create = (create_t *)dlsym(rtloader_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::ostringstream err_msg;
        err_msg << "Unable to open two factory: " << dlsym_error;
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }

    return AS_TYPE(rtloader_t, create(python_home, python_exe, _get_memory_tracker_cb()));
}

rtloader_t *make3(const char *python_home, const char *python_exe, char **error)
{
    if (rtloader_backend != NULL) {
        std::string err_msg = "RtLoader already initialized!";
        *error = strdupe(err_msg.c_str());
        return NULL;
    }

    // load the library
    rtloader_backend = dlopen(DATADOG_AGENT_THREE, RTLD_LAZY | RTLD_GLOBAL);
    if (!rtloader_backend) {
        std::ostringstream err_msg;
        err_msg << "Unable to open three library: " << dlerror();
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create_three = (create_t *)dlsym(rtloader_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::ostringstream err_msg;
        err_msg << "Unable to open three factory: " << dlsym_error;
        *error = strdupe(err_msg.str().c_str());
        return NULL;
    }

    return AS_TYPE(rtloader_t, create_three(python_home, python_exe, _get_memory_tracker_cb()));
}

void destroy(rtloader_t *rtloader)
{
    if (rtloader_backend) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)dlsym(rtloader_backend, "destroy");
        const char *dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to dlopen backend destructor: " << dlsym_error;
            return;
        }
        destroy(AS_TYPE(RtLoader, rtloader));
        rtloader_backend = NULL;
    }
}
#endif

void set_memory_tracker_cb(cb_memory_tracker_t cb)
{
    _set_memory_tracker_cb(cb);
}

int init(rtloader_t *rtloader)
{
    return AS_TYPE(RtLoader, rtloader)->init() ? 1 : 0;
}

py_info_t *get_py_info(rtloader_t *rtloader)
{
    return AS_TYPE(RtLoader, rtloader)->getPyInfo();
}

void free_py_info(rtloader_t *rtloader, py_info_t *info)
{
    AS_TYPE(RtLoader, rtloader)->freePyInfo(info);
}

int run_simple_string(const rtloader_t *rtloader, const char *code)
{
    return AS_CTYPE(RtLoader, rtloader)->runSimpleString(code) ? 1 : 0;
}

rtloader_pyobject_t *get_none(const rtloader_t *rtloader)
{
    return AS_TYPE(rtloader_pyobject_t, AS_CTYPE(RtLoader, rtloader)->getNone());
}

int add_python_path(rtloader_t *rtloader, const char *path)
{
    return AS_TYPE(RtLoader, rtloader)->addPythonPath(path) ? 1 : 0;
}

rtloader_gilstate_t ensure_gil(rtloader_t *rtloader)
{
    return AS_TYPE(RtLoader, rtloader)->GILEnsure();
}

void release_gil(rtloader_t *rtloader, rtloader_gilstate_t state)
{
    AS_TYPE(RtLoader, rtloader)->GILRelease(state);
}

int get_class(rtloader_t *rtloader, const char *name, rtloader_pyobject_t **py_module, rtloader_pyobject_t **py_class)
{
    return AS_TYPE(RtLoader, rtloader)
               ->getClass(name, *AS_PTYPE(RtLoaderPyObject, py_module), *AS_PTYPE(RtLoaderPyObject, py_class))
        ? 1
        : 0;
}

int get_attr_string(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *attr_name, char **value)
{
    return AS_TYPE(RtLoader, rtloader)->getAttrString(AS_TYPE(RtLoaderPyObject, py_class), attr_name, *value);
}

int get_check(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config, const char *instance,
              const char *check_id, const char *check_name, rtloader_pyobject_t **check)
{
    return AS_TYPE(RtLoader, rtloader)
               ->getCheck(AS_TYPE(RtLoaderPyObject, py_class), init_config, instance, check_id, check_name, NULL,
                          *AS_PTYPE(RtLoaderPyObject, check))
        ? 1
        : 0;
}

int get_check_deprecated(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config,
                         const char *instance, const char *agent_config, const char *check_id, const char *check_name,
                         rtloader_pyobject_t **check)
{
    return AS_TYPE(RtLoader, rtloader)
               ->getCheck(AS_TYPE(RtLoaderPyObject, py_class), init_config, instance, check_id, check_name,
                          agent_config, *AS_PTYPE(RtLoaderPyObject, check))
        ? 1
        : 0;
}

char *run_check(rtloader_t *rtloader, rtloader_pyobject_t *check)
{
    return AS_TYPE(RtLoader, rtloader)->runCheck(AS_TYPE(RtLoaderPyObject, check));
}

void cancel_check(rtloader_t *rtloader, rtloader_pyobject_t *check)
{
    AS_TYPE(RtLoader, rtloader)->cancelCheck(AS_TYPE(RtLoaderPyObject, check));
}

char **get_checks_warnings(rtloader_t *rtloader, rtloader_pyobject_t *check)
{
    return AS_TYPE(RtLoader, rtloader)->getCheckWarnings(AS_TYPE(RtLoaderPyObject, check));
}

diagnoses_t *get_check_diagnoses(rtloader_t *rtloader, rtloader_pyobject_t *check)
{
    return AS_TYPE(RtLoader, rtloader)->getCheckDiagnoses(AS_TYPE(RtLoaderPyObject, check));
}

/*
 * error API
 */

int has_error(const rtloader_t *rtloader)
{
    return AS_CTYPE(RtLoader, rtloader)->hasError() ? 1 : 0;
}

const char *get_error(const rtloader_t *rtloader)
{
    return AS_CTYPE(RtLoader, rtloader)->getError();
}

void clear_error(rtloader_t *rtloader)
{
    AS_TYPE(RtLoader, rtloader)->clearError();
}

#ifndef WIN32
core_trigger_t core_dump = NULL;

static inline void core(int sig)
{
    signal(sig, SIG_DFL);
    kill(getpid(), sig);
}

//! signalHandler
/*!
  \brief Crash handler for UNIX OSes
  \param sig Integer representing the signal number that triggered the crash.
  \param Unused siginfo_t parameter.
  \param Unused void * pointer parameter.

  This crash handler intercepts crashes triggered in C-land, printing the stacktrace
  at the time of the crash to stderr - logging cannot be assumed to be working at this
  poinrt and hence the use of stderr. If the core dump has been enabled, we will also
  dump a core - of course the correct ulimits need to be set for the dump to be created.
  The idea of handling the crashes here is to allow us to collect the stacktrace, with
  all its C-context, before it unwinds as would be the case if we allowed the go runtime
  to handle it.
*/
#    define STACKTRACE_SIZE 500
void signalHandler(int sig, siginfo_t *, void *)
{
    void *buffer[STACKTRACE_SIZE];
    char **symbols;

    size_t nptrs = backtrace(buffer, STACKTRACE_SIZE);
    std::cerr << "HANDLER CAUGHT signal Error: signal " << sig << std::endl;
    symbols = backtrace_symbols(buffer, nptrs);
    if (symbols == NULL) {
        std::cerr << "Error getting backtrace symbols" << std::endl;
    } else {
        std::cerr << "C-LAND STACKTRACE: " << std::endl;
        for (int i = 0; i < nptrs; i++) {
            std::cerr << symbols[i] << std::endl;
        }

        _free(symbols);
    }

    // dump core if so configured
    __sync_synchronize();
    if (core_dump) {
        core_dump(sig);
    } else {
        kill(getpid(), SIGABRT);
    }
}

/*
 * C-land crash handling
 */
DATADOG_AGENT_RTLOADER_API int handle_crashes(const int enable, char **error)
{

    struct sigaction sa;
    sa.sa_flags = SA_SIGINFO;
    sa.sa_sigaction = signalHandler;

    // on segfault - what else?
    int err = sigaction(SIGSEGV, &sa, NULL);

    if (enable && err == 0) {
        __sync_synchronize();
        core_dump = core;
    }
    if (err) {
        std::ostringstream err_msg;
        err_msg << "unable to set crash handler: " << strerror(errno);
        *error = strdupe(err_msg.str().c_str());
    }

    return err == 0 ? 1 : 0;
}
#endif

/*
 * memory management
 */

void rtloader_free(rtloader_t *rtloader, void *ptr)
{
    AS_TYPE(RtLoader, rtloader)->free(ptr);
}

void rtloader_decref(rtloader_t *rtloader, rtloader_pyobject_t *obj)
{
    AS_TYPE(RtLoader, rtloader)->decref(AS_TYPE(RtLoaderPyObject, obj));
}

void rtloader_incref(rtloader_t *rtloader, rtloader_pyobject_t *obj)
{
    AS_TYPE(RtLoader, rtloader)->incref(AS_TYPE(RtLoaderPyObject, obj));
}

void set_module_attr_string(rtloader_t *rtloader, char *module, char *attr, char *value)
{
    AS_TYPE(RtLoader, rtloader)->setModuleAttrString(module, attr, value);
}

/*
 * aggregator API
 */

void set_submit_metric_cb(rtloader_t *rtloader, cb_submit_metric_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubmitMetricCb(cb);
}

void set_submit_service_check_cb(rtloader_t *rtloader, cb_submit_service_check_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubmitServiceCheckCb(cb);
}

void set_submit_event_cb(rtloader_t *rtloader, cb_submit_event_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubmitEventCb(cb);
}

void set_submit_histogram_bucket_cb(rtloader_t *rtloader, cb_submit_histogram_bucket_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubmitHistogramBucketCb(cb);
}

void set_submit_event_platform_event_cb(rtloader_t *rtloader, cb_submit_event_platform_event_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubmitEventPlatformEventCb(cb);
}

/*
 * datadog_agent API
 */

void set_get_version_cb(rtloader_t *rtloader, cb_get_version_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetVersionCb(cb);
}

void set_get_config_cb(rtloader_t *rtloader, cb_get_config_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetConfigCb(cb);
}

void set_headers_cb(rtloader_t *rtloader, cb_headers_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setHeadersCb(cb);
}

void set_get_hostname_cb(rtloader_t *rtloader, cb_get_hostname_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetHostnameCb(cb);
}

void set_get_clustername_cb(rtloader_t *rtloader, cb_get_clustername_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetClusternameCb(cb);
}

void set_tracemalloc_enabled_cb(rtloader_t *rtloader, cb_tracemalloc_enabled_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetTracemallocEnabledCb(cb);
}

void set_log_cb(rtloader_t *rtloader, cb_log_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setLogCb(cb);
}

void set_set_check_metadata_cb(rtloader_t *rtloader, cb_set_check_metadata_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSetCheckMetadataCb(cb);
}

void set_set_external_tags_cb(rtloader_t *rtloader, cb_set_external_tags_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSetExternalTagsCb(cb);
}

char *get_integration_list(rtloader_t *rtloader)
{
    return AS_TYPE(RtLoader, rtloader)->getIntegrationList();
}

char *get_interpreter_memory_usage(rtloader_t *rtloader)
{
    return AS_TYPE(RtLoader, rtloader)->getInterpreterMemoryUsage();
}

void set_write_persistent_cache_cb(rtloader_t *rtloader, cb_write_persistent_cache_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setWritePersistentCacheCb(cb);
}

void set_read_persistent_cache_cb(rtloader_t *rtloader, cb_read_persistent_cache_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setReadPersistentCacheCb(cb);
}

void set_obfuscate_sql_cb(rtloader_t *rtloader, cb_obfuscate_sql_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setObfuscateSqlCb(cb);
}

void set_obfuscate_sql_exec_plan_cb(rtloader_t *rtloader, cb_obfuscate_sql_exec_plan_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setObfuscateSqlExecPlanCb(cb);
}

void set_get_process_start_time_cb(rtloader_t *rtloader, cb_get_process_start_time_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetProcessStartTimeCb(cb);
}

/*
 * _util API
 */
void set_get_subprocess_output_cb(rtloader_t *rtloader, cb_get_subprocess_output_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSubprocessOutputCb(cb);
}

/*
 * CGO API
 */
void set_cgo_free_cb(rtloader_t *rtloader, cb_cgo_free_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setCGOFreeCb(cb);
}

/*
 * tagger API
 */
void set_tags_cb(rtloader_t *rtloader, cb_tags_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setTagsCb(cb);
}

/*
 * kubeutil API
 */
void set_get_connection_info_cb(rtloader_t *rtloader, cb_get_connection_info_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetConnectionInfoCb(cb);
}

/*
 * containers API
 */
void set_is_excluded_cb(rtloader_t *rtloader, cb_is_excluded_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setIsExcludedCb(cb);
}

/*
 * python allocator stats API
 */
void init_pymem_stats(rtloader_t *rtloader)
{
    AS_TYPE(RtLoader, rtloader)->initPymemStats();
}

void get_pymem_stats(rtloader_t *rtloader, pymem_stats_t *stats)
{
    if (stats == NULL) {
        return;
    }
    AS_TYPE(RtLoader, rtloader)->getPymemStats(*stats);
}
