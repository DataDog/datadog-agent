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

#    ifdef HAS_BACKTRACE_LIB
#  include <execinfo.h>
#else
#  warning "<execinfo.h> not found, C backtrace will not be available"
#    endif
#include <csignal>
#include <cstring>
#include <sys/mman.h>
#include <sys/resource.h>
#include <sys/types.h>
#include <unistd.h>

// macOS and AIX name the anonymous mapping flag differently
#if !defined(MAP_ANONYMOUS) && defined(MAP_ANON)
#    define MAP_ANONYMOUS MAP_ANON
#    endif
// MAP_STACK is a hint accepted by Linux and FreeBSD only; other platforms
// (macOS, AIX) do not define it at all, so fall back to no flag.
#if !defined(MAP_STACK)
#    define MAP_STACK 0
#endif

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
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif __APPLE__
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.dylib"
#elif __FreeBSD__
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif _AIX
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif _WIN32
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
    \sa create_t, make3

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
    return AS_TYPE(rtloader_t, create_three(python_home, python_exe, _get_tracked_malloc(), _get_tracked_free()));
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

    return AS_TYPE(rtloader_t, create_three(python_home, python_exe, _get_tracked_malloc(), _get_tracked_free()));
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

void enable_memory_tracker(void)
{
    _enable_memory_tracker();
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

int get_attr_bool(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *attr_name, bool *value)
{
    return AS_TYPE(RtLoader, rtloader)->getAttrBool(AS_TYPE(RtLoaderPyObject, py_class), attr_name, *value);
}

int get_check(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config, const char *instance,
              const char *check_id, const char *check_name, const char *provider, rtloader_pyobject_t **check)
{
    return AS_TYPE(RtLoader, rtloader)
               ->getCheck(AS_TYPE(RtLoaderPyObject, py_class), init_config, instance, check_id, check_name, NULL,
                          provider, *AS_PTYPE(RtLoaderPyObject, check))
        ? 1
        : 0;
}

int get_check_deprecated(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config,
                         const char *instance, const char *agent_config, const char *check_id, const char *check_name,
                         const char *provider, rtloader_pyobject_t **check)
{
    return AS_TYPE(RtLoader, rtloader)
               ->getCheck(AS_TYPE(RtLoaderPyObject, py_class), init_config, instance, check_id, check_name,
                          agent_config, provider, *AS_PTYPE(RtLoaderPyObject, check))
        ? 1
        : 0;
}

char *discover_config(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *service_json)
{
    return AS_TYPE(RtLoader, rtloader)->discoverConfig(AS_TYPE(RtLoaderPyObject, py_class), service_json);
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

char *get_check_diagnoses(rtloader_t *rtloader, rtloader_pyobject_t *check)
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

// Storage for the previous signal handler and for the alternate stack
// installed by handle_crashes.
static struct sigaction old_sigsegv_handler;
static void *installed_alt_stack = nullptr;

//! signalHandler
/*!
  \brief Crash handler for UNIX OSes
  \param sig Integer representing the signal number that triggered the crash.
  \param info siginfo_t pointer with signal information.
  \param context void pointer to the signal context.

  This crash handler intercepts crashes triggered in C-land, printing the stacktrace
  at the time of the crash to stderr - logging cannot be assumed to be working at this
  point and hence the use of stderr. The handler only uses async-signal-safe
  functions (write(2) with hand-formatted strings): the heap may be corrupted and
  libc locks may be held by the faulting thread, so raw addresses are printed and
  are meant to be symbolized offline from the core dump. After printing the trace,
  this handler re-delivers the fault to the previously installed signal handler
  (typically the Go runtime's handler) instead of calling it directly: it restores
  the previous handler and returns, so the faulting instruction re-executes and
  the kernel delivers the fault to the previous handler with a fresh delivery,
  letting it perform its own crash handling and generate a goroutine dump. Signals that cannot re-occur (raised with
  kill/tgkill) are chained to the previous handler by direct call.
*/
#    define STACKTRACE_SIZE 500

// Async-signal-safe output helpers: no iostreams (locks), no stdio formatting
// (locale/locks), no malloc (backtrace_symbols allocates its result).
static size_t safe_append(char *dst, const char *s)
{
    size_t len = strlen(s);
    memcpy(dst, s, len);
    return len;
}

static size_t safe_append_uint(char *dst, size_t value)
{
    static const char digits[] = "0123456789";
    char tmp[20];
    size_t n = 0;
    do {
        tmp[n++] = digits[value % 10];
        value /= 10;
    } while (value != 0);
    size_t len = 0;
    while (n > 0) {
        dst[len++] = tmp[--n];
    }
    return len;
}

static size_t safe_append_hex(char *dst, uintptr_t value)
{
    static const char digits[] = "0123456789abcdef";
    char tmp[2 * sizeof(uintptr_t)];
    size_t n = 0;
    do {
        tmp[n++] = digits[value & 0xf];
        value >>= 4;
    } while (value != 0);
    size_t len = 0;
    dst[len++] = '0';
    dst[len++] = 'x';
    while (n > 0) {
        dst[len++] = tmp[--n];
    }
    return len;
}

static void safe_write(const char *buf, size_t len)
{
    // write(2) is async-signal-safe; loop over partial writes and EINTR.
    while (len > 0) {
        ssize_t written = write(STDERR_FILENO, buf, len);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            return;
        }
        if (written == 0) {
            return;
        }
        buf += written;
        len -= (size_t)written;
    }
}

static void safe_writeln(const char *s)
{
    safe_write(s, strlen(s));
    safe_write("\n", 1);
}

void signalHandler(int sig, siginfo_t *info, void *context)
{
#    ifdef HAS_BACKTRACE_LIB
    void *buffer[STACKTRACE_SIZE];
    size_t nptrs = backtrace(buffer, STACKTRACE_SIZE);
#    endif

    char header[64];
    size_t len = safe_append(header, "HANDLER CAUGHT signal ");
    len += safe_append_uint(header + len, (size_t)sig);
    header[len++] = '\n';
    safe_write(header, len);

#    ifdef HAS_BACKTRACE_LIB
    safe_writeln("C-LAND STACKTRACE (raw addresses; symbolize offline):");
    for (size_t i = 0; i < nptrs; i++) {
        char line[2 + 2 + 2 * sizeof(uintptr_t) + 2];
        size_t n = safe_append(line, "  ");
        n += safe_append_hex(line + n, (uintptr_t)buffer[i]);
        line[n++] = '\n';
        safe_write(line, n);
    }
#    endif

    // Re-deliver the fault to the previous signal handler (typically the Go
    // runtime's) instead of calling it directly: a direct call would run the
    // previous handler on the remains of this alternate stack. Restoring the
    // previous handler and returning lets the faulting instruction re-execute,
    // so the kernel delivers the fault to the previous handler with a fresh
    // delivery. The alternate stack stays installed for the re-delivered fault:
    // it cannot be swapped out while we are running on it (sigaltstack returns
    // EPERM), and it is sized for the previous handler's needs. This only works
    // for hardware faults - they re-occur when the handler returns - and it
    // leaves the C collector uninstalled: from then on, SIGSEGV is handled by
    // the previous handler alone.
    const bool old_handler_is_ign
        = (old_sigsegv_handler.sa_flags & SA_SIGINFO) == 0 && old_sigsegv_handler.sa_handler == SIG_IGN;
    if (info != nullptr && info->si_code > 0 && !old_handler_is_ign) {
        if (sigaction(SIGSEGV, &old_sigsegv_handler, nullptr) == 0) {
            return; // the fault re-occurs and is delivered to the previous handler
        }
        safe_writeln("unable to restore the previous handler, chaining directly");
    }

    // Fallback for signals that do not re-occur (raised with kill/tgkill):
    // chain to the previous signal handler by direct call.
    if (old_sigsegv_handler.sa_flags & SA_SIGINFO) {
        // Old handler uses the three-argument form
        if (old_sigsegv_handler.sa_sigaction != NULL) {
            old_sigsegv_handler.sa_sigaction(sig, info, context);
        }
    } else {
        // Old handler uses the simple one-argument form
        if (old_sigsegv_handler.sa_handler != SIG_DFL && old_sigsegv_handler.sa_handler != SIG_IGN) {
            old_sigsegv_handler.sa_handler(sig);
        } else {
            // No previous handler or it was default/ignore, so just abort
            safe_writeln("Received SIGSEGV and no handler to chain to. Aborting.");
            kill(getpid(), SIGABRT);
        }
    }
}

/*
 * C-land crash handling
 */
DATADOG_AGENT_RTLOADER_API int handle_crashes(const int enable_coredump, const int enable_stacktrace, char **error)
{
    if (!enable_coredump && !enable_stacktrace) {
        // Nothing to do
        return 1;
    }
    if (enable_coredump) {
        // All we have to do is set the RLIMIT_CORE to unlimited
        // This should not require any special privileges, but
        // if it returns EPERM then perhaps there is a system policy
        // that prevents it.
        struct rlimit rlim;
        rlim.rlim_cur = RLIM_INFINITY;
        rlim.rlim_max = RLIM_INFINITY;
        if (setrlimit(RLIMIT_CORE, &rlim) != 0) {
            std::ostringstream err_msg;
            err_msg << "unable to enable core dump: " << strerror(errno);
            *error = strdupe(err_msg.str().c_str());
            return 0;
        }
    }

    if (enable_stacktrace) {
        // Establish an alternate stack, as go stacks are too shallow and might crash.
        // SIGSTKSZ (8 KiB on Linux) is far too small for this handler: it puts a
        // 4000-byte backtrace buffer on the stack, runs the unwinder and then the
        // previously installed handler (the Go runtime's, which prints a full
        // goroutine dump) on what is left of it. Use a dedicated mmap'd region
        // instead of a heap block, sized for the whole chain, with a PROT_NONE
        // guard page below it: MAP_STACK is only a hint on Linux and creates no
        // guard page of its own, so add one explicitly. If the stack is ever
        // exhausted, the overflow faults on the guard page instead of silently
        // corrupting whatever is mapped next to it.
        static constexpr size_t ALT_STACK_SIZE = 64 * 1024;

        __sync_synchronize();
        if (installed_alt_stack == nullptr) {
            const long page_size = sysconf(_SC_PAGESIZE);
            if (page_size <= 0) {
                std::ostringstream err_msg;
                err_msg << "unable to determine the page size: " << strerror(errno);
                *error = strdupe(err_msg.str().c_str());
                return 0;
            }
            // One PROT_NONE page below the usable stack region
            const size_t total_size = ALT_STACK_SIZE + (size_t)page_size;
            void *mem
                = mmap(nullptr, total_size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_STACK, -1, 0);
            if (mem == MAP_FAILED) {
                std::ostringstream err_msg;
                err_msg << "unable to allocate alternate stack: " << strerror(errno);
                *error = strdupe(err_msg.str().c_str());
                return 0;
            }
            if (mprotect(mem, (size_t)page_size, PROT_NONE) != 0) {
                std::ostringstream err_msg;
                err_msg << "unable to set up the alternate stack guard page: " << strerror(errno);
                *error = strdupe(err_msg.str().c_str());
                munmap(mem, total_size);
                return 0;
            }
            // Note: this memory is never freed, but it is necessary for the duration of the program
            installed_alt_stack = (char *)mem + page_size;
            stack_t new_stack;
            memset(&new_stack, 0, sizeof(new_stack));
            new_stack.ss_sp = (decltype(new_stack.ss_sp))installed_alt_stack;
            new_stack.ss_size = ALT_STACK_SIZE;
            new_stack.ss_flags = 0;
            int ret = sigaltstack(&new_stack, nullptr);
            if (ret != 0) {
                std::ostringstream err_msg;
                err_msg << "unable to set alternate stack: " << strerror(errno);
                *error = strdupe(err_msg.str().c_str());
                return 0;
            }
        }

        struct sigaction sa;
        sigemptyset(&sa.sa_mask);
        sa.sa_flags = SA_SIGINFO | SA_ONSTACK;
        sa.sa_sigaction = signalHandler;

#    ifdef HAS_BACKTRACE_LIB
        // glibc's backtrace() dlopen()s libgcc_s.so.1 on first use. Doing that
        // from the signal handler would take the loader and malloc locks while
        // the faulting thread may hold them, so run backtrace() once here, in
        // a sane context, to force the library to be loaded up front.
        void *warmup[1];
        backtrace(warmup, 1);
#    endif

        // Gather stacktrace on segfault and save the old handler
        int err = sigaction(SIGSEGV, &sa, &old_sigsegv_handler);

        if (err) {
            std::ostringstream err_msg;
            err_msg << "unable to set crash handler: " << strerror(errno);
            *error = strdupe(err_msg.str().c_str());
            return 0;
        }
    }

    return 1;
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

void set_get_host_tags_cb(rtloader_t *rtloader, cb_get_host_tags_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setGetHostTagsCb(cb);
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

void set_send_log_cb(rtloader_t *rtloader, cb_send_log_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setSendLogCb(cb);
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

void set_obfuscate_mongodb_string_cb(rtloader_t *rtloader, cb_obfuscate_mongodb_string_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setObfuscateMongoDBStringCb(cb);
}

void set_emit_agent_telemetry_cb(rtloader_t *rtloader, cb_emit_agent_telemetry_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setEmitAgentTelemetryCb(cb);
}

void set_report_issue_cb(rtloader_t *rtloader, cb_report_issue_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setReportIssueCb(cb);
}

void set_resolve_issue_cb(rtloader_t *rtloader, cb_resolve_issue_t cb)
{
    AS_TYPE(RtLoader, rtloader)->setResolveIssueCb(cb);
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
