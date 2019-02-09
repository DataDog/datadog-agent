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

static void *two, *three;

void init(six_t *six, char *pythonHome) { AS_TYPE(Six, six)->init(pythonHome); }

six_t *make2() {
    // load library
    two = dlopen(DATADOG_AGENT_TWO, RTLD_LAZY);
    if (!two) {
        std::cerr << "Unable to open 'two' library: " << dlerror() << std::endl;
        return 0;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create = (create_t *)dlsym(two, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'two' factory: " << dlsym_error << std::endl;
        return 0;
    }

    return AS_TYPE(six_t, create());
}

void destroy2(six_t *six) {
    if (two) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)dlsym(two, "destroy");
        const char *dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to open 'two' destructor: " << dlsym_error << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
    }
}

six_t *make3() {
    // load the library
    three = dlopen(DATADOG_AGENT_THREE, RTLD_LAZY);
    if (!three) {
        std::cerr << "Unable to open 'three' library: " << dlerror() << std::endl;
        return 0;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t *create_three = (create_t *)dlsym(three, "create");
    const char *dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'three' factory: " << dlsym_error << std::endl;
        return 0;
    }

    return AS_TYPE(six_t, create_three());
}

void destroy3(six_t *six) {
    if (three) {
        // dlsym object destructor
        destroy_t *destroy = (destroy_t *)dlsym(three, "destroy");
        const char *dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to open 'three' destructor: " << dlsym_error << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
    }
}

int is_initialized(six_t *six) { return AS_CTYPE(Six, six)->isInitialized(); }

const char *get_py_version(const six_t *six) { return AS_CTYPE(Six, six)->getPyVersion(); }

int run_simple_string(const six_t *six, const char *code) { return AS_CTYPE(Six, six)->runSimpleString(code); }

six_pyobject_t *get_none(const six_t *six) { return AS_TYPE(six_pyobject_t, AS_CTYPE(Six, six)->getNone()); }

int add_module_func(six_t *six, six_module_t module, six_module_func_t func_type, char *func_name, void *func) {
    return AS_TYPE(Six, six)->addModuleFunction(module, func_type, func_name, func);
}

int add_module_int_const(six_t *, six_module_t module, long value) {
    // TODO
    return 0;
}

six_gilstate_t ensure_gil(six_t *six) { return AS_TYPE(Six, six)->GILEnsure(); }

void release_gil(six_t *six, six_gilstate_t state) { AS_TYPE(Six, six)->GILRelease(state); }

six_pyobject_t *import_from(six_t *six, const char *module_name, const char *symbol_name) {
    return AS_TYPE(six_pyobject_t, AS_TYPE(Six, six)->importFrom(module_name, symbol_name));
}

int has_error(const six_t *six) {
    if (AS_CTYPE(Six, six)->hasError()) {
        return 1;
    }

    return 0;
}

const char *get_error(const six_t *six) { return AS_CTYPE(Six, six)->getError().c_str(); }
