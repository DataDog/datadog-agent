// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <iostream>
#include <dlfcn.h>

#include <datadog_agent_six.h>
#include <six.h>

#if __linux__
#define DATADOG_AGENT_TWO "libdatadog-agent-two.so"
#define DATADOG_AGENT_THREE "libdatadog-agent-three.so"
#elif __APPLE__
#define DATADOG_AGENT_TWO "libdatadog-agent-two.dylib"
#define DATADOG_AGENT_THREE "libdatadog-agent-three.dylib"
#elif _WIN32
#else
#error Platform not supported
#endif

#define AS_TYPE(Type, Obj) reinterpret_cast<Type *>(Obj)
#define AS_CTYPE(Type, Obj) reinterpret_cast<const Type *>(Obj)


static void *two, *three;


void init(six_t* six, char* pythonHome)
{
    AS_TYPE(Six, six)->init(pythonHome);
}

six_t *make2()
{
    // load library
    two = dlopen(DATADOG_AGENT_TWO, RTLD_LAZY);
    if (!two) {
        std::cerr << "Unable to open 'two' library: " << dlerror() << std::endl;
        return 0;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t* create = (create_t*) dlsym(two, "create");
    const char* dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'two' factory: " << dlsym_error << std::endl;
        return 0;
    }

    return AS_TYPE(six_t, create());
}

void destroy2(six_t* six)
{
    if (two) {
        // dlsym object destructor
        destroy_t* destroy = (destroy_t*) dlsym(two, "destroy");
        const char* dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to open 'two' destructor: " << dlsym_error << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
    }
}

six_t *make3()
{
    // load the library
    three = dlopen(DATADOG_AGENT_THREE, RTLD_LAZY);
    if (!three) {
        std::cerr << "Unable to open 'three' library: " << dlerror() << std::endl;
        return 0;
    }

    // reset dl errors
    dlerror();

    // dlsym class factory
    create_t* create_three = (create_t*) dlsym(three, "create");
    const char* dlsym_error = dlerror();
    if (dlsym_error) {
        std::cerr << "Unable to open 'three' factory: " << dlsym_error << std::endl;
        return 0;
    }

    return AS_TYPE(six_t, create_three());
}

void destroy3(six_t* six)
{
    if (three) {
        // dlsym object destructor
        destroy_t* destroy = (destroy_t*) dlsym(three, "destroy");
        const char* dlsym_error = dlerror();
        if (dlsym_error) {
            std::cerr << "Unable to open 'three' destructor: " << dlsym_error << std::endl;
            return;
        }
        destroy(AS_TYPE(Six, six));
    }
}

int is_initialized(six_t* six)
{
    return AS_CTYPE(Six, six)->isInitialized();
}

const char *get_py_version(const six_t* six)
{
    return AS_CTYPE(Six, six)->getPyVersion();
}

int run_simple_string(const six_t* six, const char* code)
{
    return AS_CTYPE(Six, six)->runSimpleString(code);
}

six_pyobject_t* get_none(const six_t* six)
{
    return AS_TYPE(six_pyobject_t, AS_CTYPE(Six, six)->getNone());
}

int add_module_func(six_t* six, six_module_t module, six_module_func_t func_type,
                     char *func_name, void *func)
{
    Six::ExtensionModule six_module;
    switch(module) {
        case DATADOG_AGENT_SIX_DATADOG_AGENT:
            six_module = Six::DATADOG_AGENT;
            break;
        default:
            std::cerr << "Unknown six_module_t value" << std::endl;
            return -1;
    }

    Six::MethType six_func_type;
    switch(func_type) {
        case DATADOG_AGENT_SIX_NOARGS:
            six_func_type = Six::NOARGS;
            break;
        case DATADOG_AGENT_SIX_ARGS:
            six_func_type = Six::ARGS;
            break;
        case DATADOG_AGENT_SIX_KEYWORDS:
            six_func_type = Six::KEYWORDS;
            break;
        default:
            std::cerr << "Unknown six_module_func_t value" << std::endl;
            return -1;
    }

    return AS_TYPE(Six, six)->addModuleFunction(six_module, six_func_type, func_name, func);
}
