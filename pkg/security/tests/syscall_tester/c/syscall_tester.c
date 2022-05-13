#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/ptrace.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <string.h>
#include <stdint.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/fsuid.h>
#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <errno.h>

#define RPC_CMD 0xdeadc001
#define REGISTER_SPAN_TLS_OP 6

#ifndef SYS_gettid
#error "SYS_gettid unavailable on this system"
#endif

pid_t gettid(void) {
    pid_t tid = syscall(SYS_gettid);
    return tid;
}

struct span_tls_t {
    uint64_t format;
    uint64_t max_threads;
    void *base;
};

struct thread_opts {
    struct span_tls_t *tls;
    char **argv;
};

void *register_tls() {
    uint64_t max_threads = 100;
    uint64_t len = max_threads * sizeof(uint64_t) * 2;

    uint64_t *base = (uint64_t *)malloc(len);
    if (base == NULL)
        return NULL;
    bzero(base, len);

    struct span_tls_t *tls = (struct span_tls_t *)malloc(sizeof(struct span_tls_t));
    if (tls == NULL)
        return NULL;
    tls->max_threads = max_threads;
    tls->base = base;

    uint8_t request[257];
    bzero(request, sizeof(request));

    memcpy(&request[sizeof(uint8_t)], tls, sizeof(struct span_tls_t));
    request[0] = REGISTER_SPAN_TLS_OP;
    ioctl(0, RPC_CMD, &request);

    return tls;
}

void register_span(struct span_tls_t *tls, unsigned trace_id, unsigned span_id) {
    int offset = (gettid() % tls->max_threads) * 2;

    uint64_t *base = tls->base;
    base[offset] = span_id;
    base[offset + 1] = trace_id;
}

int span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Please pass a span Id and a trace Id to exec_span and a command\n");
        return EXIT_FAILURE;
    }

    struct span_tls_t *tls = register_tls();
    if (!tls) {
        fprintf(stderr, "Failed to register TLS\n");
        return EXIT_FAILURE;
    }

    unsigned trace_id = atoi(argv[1]);
    unsigned span_id = atoi(argv[2]);

    register_span(tls, trace_id, span_id);

    execv(argv[3], argv + 3);

    return EXIT_SUCCESS;
}

static void *thread_open(void *data) {
    struct thread_opts *opts = (struct thread_opts *)data;

    unsigned trace_id = atoi(opts->argv[1]);
    unsigned span_id = atoi(opts->argv[2]);

    register_span(opts->tls, trace_id, span_id);

    int fd = open(opts->argv[3], O_CREAT);
    if (fd < 0) {
        fprintf(stderr, "Unable to create file `%s`\n", opts->argv[3]);
        return NULL;
    }
    close(fd);

    return NULL;
}

int span_open(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Please pass a span Id and a trace Id to exec_span and a command\n");
        return EXIT_FAILURE;
    }

    struct span_tls_t *tls = register_tls();
    if (!tls) {
        fprintf(stderr, "Failed to register TLS\n");
        return EXIT_FAILURE;
    }

    struct thread_opts opts = {
        .argv = argv,
        .tls = tls,
    };

    pthread_t thread;
    if (pthread_create(&thread, NULL, thread_open, &opts) < 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    return EXIT_SUCCESS;
}

int ptrace_traceme() {
    int child = fork();
    if (child == 0) {
        ptrace(PTRACE_TRACEME, 0, NULL, NULL);
        raise(SIGSTOP);
    } else {
        wait(NULL);
        ptrace(PTRACE_CONT, child, 42, NULL);
    }
    return EXIT_SUCCESS;
}

int test_signal_sigusr(void) {
    int child = fork();
    if (child == 0) {
        sleep(5);
    } else {
        kill(child, SIGUSR1);
        sleep(1);
    }
    return EXIT_SUCCESS;
}

int test_signal_eperm(void) {
    int ppid = getpid();
    int child = fork();
    if (child == 0) {
        /* switch to user daemon */
        if (setuid(1)) {
            fprintf(stderr, "Failed to setuid 1 (%s)\n", strerror(errno));
            return EXIT_FAILURE;
        }
        kill(ppid, SIGKILL);
        sleep(1);
    } else {
        wait(NULL);
    }
    return EXIT_SUCCESS;
}

int test_signal(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "%s: Please pass a test case in: sigusr, eperm.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    if (!strcmp(argv[1], "sigusr"))
        return test_signal_sigusr();
    else if (!strcmp(argv[1], "eperm"))
        return test_signal_eperm();
    fprintf(stderr, "%s: Unknown argument: %s.\n", __FUNCTION__, argv[1]);
    return EXIT_FAILURE;
}

int test_splice() {
    const int fd = open("/tmp/splice_test", O_RDONLY | O_CREAT, 0700);
    if (fd < 0) {
        fprintf(stderr, "open failed");
        return EXIT_FAILURE;
    }

    int p[2];
    if (pipe(p)) {
        fprintf(stderr, "pipe failed");
        return EXIT_FAILURE;
    }

    loff_t offset = 1;
    splice(fd, 0, p[1], NULL, 1, 0);
    close(fd);
    sleep(5);
    remove("/tmp/splice_test");

    return EXIT_SUCCESS;
}

int test_mkdirat_error(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "%s: Please pass a path to mkdirat.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    if (setregid(1, 1) != 0) {
        fprintf(stderr, "setregid failed");
        return EXIT_FAILURE;
    }

    if (setreuid(1, 1) != 0) {
        fprintf(stderr, "setreuid failed");
        return EXIT_FAILURE;
    }

    if (mkdirat(0, argv[1], 0777) == 0) {
        fprintf(stderr, "mkdirat succeeded even though we expected it to fail");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_process_set(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "%s: Please pass a syscall name, real and effective id.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    int real_id = atoi(argv[2]);
    int effective_id = atoi(argv[3]);

    char *subcmd = argv[1];

    int res;
    if (strcmp(subcmd, "setuid") == 0) {
        res = setuid(real_id);
    } else if (strcmp(subcmd, "setreuid") == 0) {
        res = setreuid(real_id, effective_id);
    } else if (strcmp(subcmd, "setresuid") == 0) {
        res = setresuid(real_id, effective_id, 0);
    } else if (strcmp(subcmd, "setfsuid") == 0) {
        res = setfsuid(real_id);
    } else if (strcmp(subcmd, "setgid") == 0) {
        res = setgid(real_id);
    } else if (strcmp(subcmd, "setregid") == 0) {
        res = setregid(real_id, effective_id);
    } else if (strcmp(subcmd, "setresgid") == 0) {
        res = setresgid(real_id, effective_id, 0);
    } else if (strcmp(subcmd, "setfsgid") == 0) {
        res = setfsgid(real_id);
    } else {
        fprintf(stderr, "Unknown subcommand `%s`\n", subcmd);
        return EXIT_FAILURE;
    }

    if (res != 0) {
        fprintf(stderr, "%s failed", subcmd);
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int self_exec(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please pass a command name\n");
        return EXIT_FAILURE;
    }

    execv("/proc/self/exe", argv + 1);

    return EXIT_SUCCESS;
}

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    char *cmd = argv[1];

    if (strcmp(cmd, "check") == 0) {
        return EXIT_SUCCESS;
    } else if (strcmp(cmd, "span-exec") == 0) {
        return span_exec(argc - 1, argv + 1);
    } else if (strcmp(cmd, "ptrace-traceme") == 0) {
        return ptrace_traceme();
    } else if (strcmp(cmd, "span-open") == 0) {
        return span_open(argc - 1, argv + 1);
    } else if (strcmp(cmd, "signal") == 0) {
        return test_signal(argc - 1, argv + 1);
    } else if (strcmp(cmd, "splice") == 0) {
        return test_splice();
    } else if (strcmp(cmd, "mkdirat-error") == 0) {
        return test_mkdirat_error(argc - 1, argv + 1);
    } else if (strcmp(cmd, "process-credentials") == 0) {
        return test_process_set(argc - 1, argv + 1);
    } else if (strcmp(cmd, "self-exec") == 0) {
        return self_exec(argc - 1, argv + 1);
    } else {
        fprintf(stderr, "Unknown command `%s`\n", cmd);
        return EXIT_FAILURE;
    }
}
