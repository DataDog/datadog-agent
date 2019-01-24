#include <iostream>
#include <dlfcn.h>

#include <datadog_agent_six.h>
#include <six.h>


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
    two = dlopen("libdatadog-agent-two.so", RTLD_LAZY);
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
    three = dlopen("libdatadog-agent-three.so", RTLD_LAZY);
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


const char *get_py_version(const six_t* six)
{
    return AS_CTYPE(Six, six)->getPyVersion();
}

void run_any_file(const six_t* six, const char* path)
{
    AS_CTYPE(Six, six)->runAnyFile(path);
}

six_pyobject_t* get_none(const six_t* six)
{
    return AS_TYPE(six_pyobject_t, AS_CTYPE(Six, six)->getNone());
}

void add_module_func_noargs(six_t* six, char* moduleName, char* funcName, void* func)
{
    AS_TYPE(Six, six)->addModuleFunction(moduleName, funcName, func, Six::NOARGS);
}

void add_module_func_args(six_t* six, char* moduleName, char* funcName, void* func)
{
    AS_TYPE(Six, six)->addModuleFunction(moduleName, funcName, func, Six::ARGS);
}

void add_module_func_keywords(six_t* six, char* moduleName, char* funcName, void* func)
{
    AS_TYPE(Six, six)->addModuleFunction(moduleName, funcName, func, Six::KEYWORDS);
}
