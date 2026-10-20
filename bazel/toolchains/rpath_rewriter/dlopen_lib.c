#include <dlfcn.h>

void *load(const char *name) { return dlopen(name, RTLD_NOW); }
