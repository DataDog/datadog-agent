#ifndef SHARED_LIBRARY_TYPES_H
#define SHARED_LIBRARY_TYPES_H

// (run_function_cb)
typedef void(run_shared_library_check_t)(char *);

// library and symbols pointers
typedef struct shared_library_handle_s {
    void *lib; // handle to the shared library
    run_shared_library_check_t *run; // handle to the run function
} shared_library_handle_t;

#endif