// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <dlfcn.h>
#include <iostream>

#include <datadog_agent_six.h>
#include <six.h>

#if __linux__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.so"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif __APPLE__
#    define DATADOG_AGENT_TWO "libdatadog-agent-two.dylib"
#    define DATADOG_AGENT_THREE "libdatadog-agent-three.dylib"
#elif _WIN32
#else
#    error Platform not supported
#endif

#define AS_TYPE(Type, Obj) reinterpret_cast<Type *>(Obj)
#define AS_CTYPE(Type, Obj) reinterpret_cast<const Type *>(Obj)

static void *six_backend = NULL;
// public API will return a const char* pointing to the storage of this string
static std::string last_error;

six_t *make2() {
    if (six_backend != NULL) {
        std::cerr << "Six alrady initialized!" << std::endl;
        return NULL;
    }

    // load library
    six_backend = dlopen(DATADOG_AGENT_TWO, RTLD_LAZY | RTLD_GLOBAL);
    if (!six_backend) {
        std::cerr << "Unable to open 'two' library: " << dlerror() << std::endl;
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create = (create_t *)dlsym(six_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'two' factory: " << dlsym_error << std::endl;
        return NULL;
    }

    return AS_TYPE(six_t, create());
}

six_t *make3() {
    if (six_backend != NULL) {
        std::cerr << "Six alrady initialized!" << std::endl;
        return NULL;
    }

    // load the library
    six_backend = dlopen(DATADOG_AGENT_THREE, RTLD_LAZY | RTLD_GLOBAL);
    if (!six_backend) {
        std::cerr << "Unable to open 'three' library: " << dlerror() << std::endl;
        return NULL;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create_three = (create_t *)dlsym(six_backend, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'three' factory: " << dlsym_error << std::endl;
        return NULL;
    }

    return AS_TYPE(six_t, create_three());
}

void destroy(six_t *six) {
    if (six_backend) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)dlsym(six_backend, "destroy");
        const char *dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to dlopen backend destructor: " << dlsym_error << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
    }
}

int init(six_t *six, char *pythonHome) {
    bool ret = AS_TYPE(Six, six)->init(pythonHome);
    return ret ? 1 : 0;
}

int is_initialized(six_t *six) { return AS_CTYPE(Six, six)->isInitialized(); }

const char *get_py_version(const six_t *six) { return AS_CTYPE(Six, six)->getPyVersion(); }

int run_simple_string(const six_t *six, const char *code) { return AS_CTYPE(Six, six)->runSimpleString(code); }

six_pyobject_t *get_none(const six_t *six) { return AS_TYPE(six_pyobject_t, AS_CTYPE(Six, six)->getNone()); }

int add_module_func(six_t *six, six_module_t module, six_module_func_t func_type, char *func_name, void *func) {
    return AS_TYPE(Six, six)->addModuleFunction(module, func_type, func_name, func);
}

int add_module_int_const(six_t *six, six_module_t module, const char *name, long value) {
    return AS_TYPE(Six, six)->addModuleIntConst(module, name, value);
}

six_gilstate_t ensure_gil(six_t *six) { return AS_TYPE(Six, six)->GILEnsure(); }

void release_gil(six_t *six, six_gilstate_t state) { AS_TYPE(Six, six)->GILRelease(state); }

six_pyobject_t *get_check_class(six_t *six, const char *name) {
    return AS_TYPE(six_pyobject_t, AS_TYPE(Six, six)->getCheckClass(name));
}

six_pyobject_t *get_check(six_t *six, const char *name, const char *init_config, const char *instances) {
    return AS_TYPE(six_pyobject_t, AS_TYPE(Six, six)->getCheck(name, init_config, instances));
}

const char *run_check(six_t *six, six_pyobject_t *check) {
    return AS_TYPE(Six, six)->runCheck(AS_TYPE(SixPyObject, check));
}

int has_error(const six_t *six) {
    if (AS_CTYPE(Six, six)->hasError()) {
        return 1;
    }

    return 0;
}

const char *get_error(const six_t *six) {
    last_error = AS_CTYPE(Six, six)->getError();
    return last_error.c_str();
}
