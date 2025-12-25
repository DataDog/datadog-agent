#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char *argv[]) {
    if (argc < 3 || strcmp(argv[1], "--") != 0) {
        fprintf(stderr, "usage: %s <system-probe command line>\n", argv[0]);
        return 1;
    }

    fprintf(stdout, "system-probe-discovery: Executing system-probe");

    execv(argv[2], &argv[2]);
    perror("execv failed");

    return 1;
}
