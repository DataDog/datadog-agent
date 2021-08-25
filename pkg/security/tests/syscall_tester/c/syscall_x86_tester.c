#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <string.h>

int chown_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to chown\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    syscall(SYS_chown, argv[1], uid, gid);

    return EXIT_SUCCESS;
}

int fchown_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to fchown\n");
        return EXIT_FAILURE;
    }

    FILE *f = fopen(argv[1], "r");
    if (!f) {
        perror("Failed to open provided file");
        return EXIT_FAILURE;
    }

    int fd = fileno(f);
    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);

    syscall(SYS_fchown, fd, uid, gid);

    fclose(f);

    return EXIT_SUCCESS;
}

int fchownat_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to fchownat\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    syscall(SYS_fchownat, 0, argv[1], uid, gid, 0x100);

    return EXIT_SUCCESS;
}

int lchown_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to lchown\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    syscall(SYS_lchown, argv[1], uid, gid);

    return EXIT_SUCCESS;
}

#ifdef SYS_chown32
int chown32_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to chown32\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    syscall(SYS_chown32, argv[1], uid, gid);

    return EXIT_SUCCESS;
}
#endif

#ifdef SYS_fchown32
int fchown32_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to fchown32\n");
        return EXIT_FAILURE;
    }

    FILE *f = fopen(argv[1], "r");
    if (!f) {
        perror("Failed to open provided file");
        return EXIT_FAILURE;
    }

    int fd = fileno(f);
    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);

    syscall(SYS_fchown32, fd, uid, gid);

    fclose(f);

    return EXIT_SUCCESS;
}
#endif

#ifdef SYS_lchown32
int lchown32_syscall(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please pass a file path, destination uid and destination gid to lchown32\n");
        return EXIT_FAILURE;
    }

    unsigned uid = atoi(argv[2]);
    unsigned gid = atoi(argv[3]);
    syscall(SYS_lchown32, argv[1], uid, gid);

    return EXIT_SUCCESS;
}
#endif

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    char *cmd = argv[1];

    if (strcmp(cmd, "check") == 0) {
        return EXIT_SUCCESS;
    } else if (strcmp(cmd, "chown") == 0) {
        return chown_syscall(argc - 1, argv + 1);
    } else if (strcmp(cmd, "fchown") == 0) {
        return fchown_syscall(argc - 1, argv + 1);
    } else if (strcmp(cmd, "fchownat") == 0) {
        return fchownat_syscall(argc - 1, argv + 1);
    } else if (strcmp(cmd, "lchown") == 0) {
        return lchown_syscall(argc - 1, argv + 1);
    } else if (strcmp(cmd, "chown32") == 0) {
#ifdef SYS_chown32
        return chown32_syscall(argc - 1, argv + 1);
#else
        fprintf(stderr, "chown32 syscall is not available");
        return EXIT_FAILURE;
#endif
    } else if (strcmp(cmd, "fchown32") == 0) {
#ifdef SYS_chown32
        return fchown32_syscall(argc - 1, argv + 1);
#else
        fprintf(stderr, "fchown32 syscall is not available");
        return EXIT_FAILURE;
#endif
    } else if (strcmp(cmd, "lchown32") == 0) {
#ifdef SYS_lchown32
        return lchown32_syscall(argc - 1, argv + 1);
#else
        fprintf(stderr, "lchown32 syscall is not available");
        return EXIT_FAILURE;
#endif
    } else {
        fprintf(stderr, "Unknown command `%s`\n", cmd);
        return EXIT_FAILURE;
    }
}
