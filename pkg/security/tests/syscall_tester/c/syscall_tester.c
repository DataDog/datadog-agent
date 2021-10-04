#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <string.h>
#include <stdint.h>
#include <sys/ioctl.h>

#define RPC_CMD 0xdeadc001
#define REGISTER_SPAN_TLS_OP 6

struct span_tls_t {
   uint64_t format;
   uint64_t max_threads;
   void *base;
};

void *register_span(unsigned trace_id, unsigned span_id) {
    uint64_t max_thread = 100;
    uint64_t len = max_thread * sizeof(uint64_t) * 2;

    uint64_t *base = (uint64_t *) malloc(len);
    if (base == NULL)
        return NULL;
    bzero(base, len);

    uint8_t request[257];
    bzero(request, sizeof(request));

    struct span_tls_t *tls = (struct span_tls_t *) &request[sizeof(uint8_t)];
    tls->max_threads = max_thread;
    tls->base = base;

    request[0] = REGISTER_SPAN_TLS_OP;
    ioctl(0, RPC_CMD, &request);

    // single thread pid == tid
    int offset = (getpid() % max_thread) * 2;
    base[offset] = span_id;
    base[offset+1] = trace_id;

    return tls;
}

int span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Please pass a span Id and a trace Id to exec_span and a command\n");
        return EXIT_FAILURE;
    }

    unsigned trace_id = atoi(argv[1]);
    unsigned span_id = atoi(argv[2]);

    void *tls = register_span(trace_id, span_id);
    if (tls == NULL)
        return EXIT_FAILURE;

    execv(argv[3], argv + 3);

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
    } else {
        fprintf(stderr, "Unknown command `%s`\n", cmd);
        return EXIT_FAILURE;
    }
}
