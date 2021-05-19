#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <string.h>

int chown_syscall(int argc, char **argv) {
    if (argc != 4) {
        printf("Please pass a file path, destination uid and destination gid to chown\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    chown(argv[1], uid, gid);

    return EXIT_SUCCESS;
}

int main(int argc, char **argv) {
    if (argc <= 1) {
        printf("Please pass a command\n");
        return EXIT_SUCCESS;
    }

    char* cmd = argv[1];

    if (strcmp(cmd, "chown") == 0) {
        return chown_syscall(argc - 1, argv + 1);
    } else {
        printf("Unknown command `%s`\n", cmd);
    }

    return EXIT_SUCCESS;
}
