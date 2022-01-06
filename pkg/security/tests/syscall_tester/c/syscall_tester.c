#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <string.h>
#include <stdint.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <pthread.h>

#define RPC_CMD 0xdeadc001
#define REGISTER_SPAN_TLS_OP 6

pid_t gettid(void);

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

    struct span_tls_t *tls = (struct span_tls_t *) malloc(sizeof(struct span_tls_t));
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
    if (argc < 3) {
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
    } else if (strcmp(cmd, "span-open") == 0) {
        return span_open(argc - 1, argv + 1);
    } else {
        fprintf(stderr, "Unknown command `%s`\n", cmd);
        return EXIT_FAILURE;
    }
}
