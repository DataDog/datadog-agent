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
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/fsuid.h>
#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <errno.h>
#include <arpa/inet.h>
#include <linux/un.h>

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

int test_bind_af_inet(int argc, char** argv) {
    int s = socket(PF_INET, SOCK_STREAM, 0);
    if (s < 0) {
        perror("socker");
        return EXIT_FAILURE;
    }

    if (argc != 2) {
        fprintf(stderr, "Please speficy an option in the list: any, custom_ip\n");
        return EXIT_FAILURE;
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        addr.sin_addr.s_addr = htonl(INADDR_ANY);
    } else if (!strcmp(ip, "custom_ip")) {
        int ip32 = 0;
        if (inet_pton(AF_INET, "127.0.0.1", &ip32) != 1) {
            perror("inet_pton");
            return EXIT_FAILURE;
        }
        addr.sin_addr.s_addr = htonl(ip32);
    } else {
        fprintf(stderr, "Please speficy an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin_port = htons(4242);
    bind(s, (struct sockaddr*)&addr, sizeof(addr));

    close (s);
    return EXIT_SUCCESS;
}

int test_bind_af_inet6(int argc, char** argv) {
    int s = socket(AF_INET6, SOCK_STREAM, 0);
    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    if (argc != 2) {
        fprintf(stderr, "Please speficy an option in the list: any, custom_ip\n");
        return EXIT_FAILURE;
    }

    struct sockaddr_in6 addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin6_family = AF_INET6;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        inet_pton(AF_INET6, "::", &addr.sin6_addr);
    } else if (!strcmp(ip, "custom_ip")) {
        inet_pton(AF_INET6, "1234:5678:90ab:cdef:0000:0000:1a1a:1337", &addr.sin6_addr);
    } else {
        fprintf(stderr, "Please speficy an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin6_port = htons(4242);
    bind(s, (struct sockaddr*)&addr, sizeof(addr));

    close(s);
    return EXIT_SUCCESS;
}

#define TEST_BIND_AF_UNIX_SERVER_PATH "/tmp/test_bind_af_unix"
int test_bind_af_unix(void) {
    int s = socket(AF_UNIX, SOCK_STREAM, 0);
    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    unlink(TEST_BIND_AF_UNIX_SERVER_PATH);
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, TEST_BIND_AF_UNIX_SERVER_PATH, strlen(TEST_BIND_AF_UNIX_SERVER_PATH));
    int ret = bind(s, (struct sockaddr*)&addr, sizeof(addr));
    printf("bind retval: %i\n", ret);
    if (ret)
        perror("bind");

    close(s);
    unlink(TEST_BIND_AF_UNIX_SERVER_PATH);
    return EXIT_SUCCESS;
}

int test_bind(int argc, char** argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please speficy an addr_type\n");
        return EXIT_FAILURE;
    }

    char* addr_family = argv[1];
    if (!strcmp(addr_family, "AF_INET")) {
        return test_bind_af_inet(argc - 1, argv + 1);
    } else if  (!strcmp(addr_family, "AF_INET6")) {
        return test_bind_af_inet6(argc - 1, argv + 1);
    } else if  (!strcmp(addr_family, "AF_UNIX")) {
        return test_bind_af_unix();
    }

    fprintf(stderr, "Specified %s addr_type is not a valid one, try: AF_INET, AF_INET6 or AF_UNIX\n", addr_family);
    return EXIT_FAILURE;
}

int test_forkexec(int argc, char **argv) {
    if (argc == 3) {
        char *subcmd = argv[1];
        char *open_trigger_filename = argv[2];
        if (strcmp(subcmd, "exec") == 0) {
            int child = fork();
            if (child == 0) {
                char *const args[] = {"syscall_tester", "fork", "open", open_trigger_filename, NULL};
                execv("/proc/self/exe", args);
            } else if (child > 0) {
                wait(NULL);
            }
            return EXIT_SUCCESS;
        } else if (strcmp(subcmd, "open") == 0) {
            int fd = open(open_trigger_filename, O_RDONLY|O_CREAT, 0444);
            if (fd >= 0) {
                close(fd);
                unlink(open_trigger_filename);
            }
            return EXIT_SUCCESS;
        }
    } else if (argc == 2) {
        char *open_trigger_filename = argv[1];
        int child = fork();
        if (child == 0) {
            int fd = open(open_trigger_filename, O_RDONLY|O_CREAT, 0444);
            if (fd >= 0) {
                close(fd);
                unlink(open_trigger_filename);
            }
            return EXIT_SUCCESS;
        } else if (child > 0) {
            wait(NULL);
        }
        return EXIT_SUCCESS;
    } else {
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_exit_fork(int argc, char **argv) {
    if (argc >= 2 && strcmp(argv[1], "exec") == 0) {
        int child = fork();
        if (child == 0) {
            char *const args[] = {"syscall_tester_child", NULL};
            execv("/proc/self/exe", args);
        } else {
            wait(NULL);
        }
    } else {
        int child = fork();
        if (child > 0) {
            wait(NULL);
        }
    }

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
    } else if (strcmp(cmd, "bind") == 0) {
        return test_bind(argc - 1, argv + 1);
    } else if (strcmp(cmd, "fork") == 0) {
        return test_forkexec(argc - 1, argv + 1);
    } else if (strcmp(cmd, "exit-fork") == 0) {
        return test_exit_fork(argc - 1, argv + 1);
    } else {
        fprintf(stderr, "Unknown command `%s`\n", cmd);
        return EXIT_FAILURE;
    }
}
