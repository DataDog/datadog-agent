#include <stdlib.h>
#include <stdbool.h>

typedef struct
{
    const char *data;
    int len;
    int cap;
} Result;

extern void *open_library(char *library, const char **error);
extern void close_library(void *handle, const char **error);
extern void run_agnostic_check(void *handle, const char *id, Result *result, const char **error);
extern void free_result(void *handle, Result *result, const char **error);
extern Result *allocate(void *handle, const char **error);