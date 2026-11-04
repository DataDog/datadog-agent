#include <jemalloc/jemalloc.h>
#include <stdio.h>
#include <string.h>

int main(void) {
    const char *version;
    size_t len = sizeof(version);
    if (mallctl("version", &version, &len, NULL, 0) != 0) {
        fprintf(stderr, "mallctl(version) failed\n");
        return 1;
    }
    if (strncmp(version, "5.3.0-", 6) != 0) {
        fprintf(stderr, "unexpected version %s\n", version);
        return 1;
    }
    // sallocx only accepts jemalloc pointers, so this checks malloc is overridden.
    void *p = malloc(100);
    if (p == NULL || sallocx(p, 0) < 100) {
        fprintf(stderr, "malloc is not served by jemalloc\n");
        return 1;
    }
    free(p);
    return 0;
}
