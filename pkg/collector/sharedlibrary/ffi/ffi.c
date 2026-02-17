#include "ffi.h"

#include <string.h>

#ifdef _WIN32
#    include <windows.h>
#    include <stdio.h>
#else
#    include <dlfcn.h>
#endif

#ifdef _WIN32
// windows specific implementations

void *open_lib(const char *lib_path, char **lib_error) {
    void *lib_handle = LoadLibraryA(lib_path);
    if (!lib_handle) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to open shared library, error code: %d", error_code);
		*lib_error = strdup(error_msg);
    }

    return lib_handle;
}

void *get_symbol(void *lib_handle, const char *symbol_name, char **lib_error) {
    void *symbol = GetProcAddress(lib_handle, symbol_name);
    if (!symbol) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Run' symbol, error code: %d", error_code);
		*lib_error = strdup(error_msg);
    }

    return symbol;
}

void close_lib(void *lib_handle, char **lib_error) {
    // close the library and check for errors (error_code == 0)
    int error_code = FreeLibrary(lib_handle);
    if (!error_code) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to close shared library, error code: %d", error_code);
		*lib_error = strdup(error_msg);
    }
}
#else
// non-windows specific implementations

void *open_lib(const char *lib_path, char **lib_error) {
    void *lib_handle;
    char *dlsym_error = NULL;

    // Calling `dlopen` again for the same shared library doesn't reopen it. The returned handle is the same.
    // (https://man7.org/linux/man-pages/man3/dlopen.3p.html)
    // This is great for running multiple instances in parallel but the global state of the shared library
    // remains the same for all the instances.
    lib_handle = dlopen(lib_path, RTLD_NOW | RTLD_LOCAL);

    // catch library opening error
    dlsym_error = dlerror();

    if (dlsym_error) {
		*lib_error = strdup(dlsym_error);
    }

    return lib_handle;
}

void *get_symbol(void *lib_handle, const char *symbol_name, char **lib_error) {
    void *symbol;
    char *dlsym_error = NULL;

    // get symbol pointer
    symbol = dlsym(lib_handle, symbol_name);

    // catch symbol errors and close the library if there are any
    dlsym_error = dlerror();
    if (dlsym_error) {
		*lib_error = strdup(dlsym_error);
    }

    return symbol;
}

void close_lib(void *lib_handle, char **lib_error) {
    // Calling `dlclose` for a shared library that has been opened multiple times doesn't close it.
    // (https://man7.org/linux/man-pages/man3/dlclose.3p.html)
    dlclose(lib_handle);

    // check for closing errors
    char *dlsym_error = dlerror();
    if (dlsym_error) {
		*lib_error = strdup(dlsym_error);
    }
}
#endif
// shared library interface functions

library_t load_shared_library(const char *lib_path, const char **error) {
    library_t lib;
    char *lib_error = NULL;

    // open the library
    lib.handle = open_lib(lib_path, &lib_error);
    if (lib_error) {
		*error = lib_error;
        return lib;
    }

    // get pointer of 'Run' symbol
    lib.run = (run_function_t *)get_symbol(lib.handle, "Run", &lib_error);
    if (lib_error) {
		*error = strdup("can't find 'Run' symbol");
        close_lib(lib.handle, &lib_error);
        return lib;
    }

    // get pointer of 'Version' symbol
    // it's not required, the pointer is set to NULL if the symbol wasn't found
    lib.version = (version_function_t *)get_symbol(lib.handle, "Version", &lib_error);
    if (lib_error) {
        lib.version = NULL;
    }

    return lib;
}

void close_shared_library(void *lib_handle, const char **error) {
    // verify library handle
	if (!lib_handle) {
        *error = strdup("pointer to shared library is NULL");
        return;
	}

    char *lib_error = NULL;

    close_lib(lib_handle, &lib_error);
    if (lib_error) {
		*error = lib_error;
    }
}

void run_shared_library(run_function_t *run_ptr, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator, const char **error) {
    // verify `Run` pointer
    if (!run_ptr) {
        *error = strdup("pointer to 'Run' symbol of the shared library is NULL");
        return;
    }

    // run the shared library check and put any errors string in the `error` variable
    (run_ptr)(check_id, init_config, instance_config, aggregator, error);
}

const char *get_version_shared_library(version_function_t *version_ptr, const char **error) {
    // verify `Version` pointer
    if (!version_ptr) {
        *error = strdup("pointer to 'Version' symbol of the shared library is NULL");
        return NULL;
    }

    // retrieve the version of the shared library check and put any errors string in the `error` variable
    return (version_ptr)(error);
}
