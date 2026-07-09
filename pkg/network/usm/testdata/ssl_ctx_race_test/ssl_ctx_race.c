// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

/*
 * ssl_ctx_race - Test helper for ssl_ctx_by_pid_tgid race condition
 *
 * This program tests whether the race condition in ssl_ctx_by_pid_tgid can
 * cause practical misattribution of SSL connections.
 *
 * The race condition:
 * 1. Thread calls SSL_read(conn1) -> tup_from_ssl_ctx() misses ssl_sock_by_ctx
 *    -> stores ctx1 in ssl_ctx_by_pid_tgid[pid_tgid]
 * 2. Thread calls SSL_read(conn2) BEFORE tcp_sendmsg fires for conn1
 *    -> OVERWRITES with ctx2
 * 3. tcp_sendmsg fires for conn1 -> map_ssl_ctx_to_sock() reads ssl_ctx_by_pid_tgid
 *    -> gets ctx2 (WRONG!)
 *
 * Usage: ssl_ctx_race <host1> <port1> <host2> <port2> [iterations]
 *
 * The program:
 * 1. Connects to two HTTPS servers and establishes SSL sessions
 * 2. Prints "READY" with local port info and waits for SIGUSR1
 * 3. On signal: performs rapid interleaved SSL_write/SSL_read operations
 * 4. Reports results for verification
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <openssl/ssl.h>
#include <openssl/err.h>

#define DEFAULT_ITERATIONS 1000

static volatile sig_atomic_t start_test = 0;

void signal_handler(int sig) {
    if (sig == SIGUSR1) {
        start_test = 1;
    }
}

typedef struct {
    int sock;
    SSL *ssl;
    SSL_CTX *ctx;
    int local_port;
    int remote_port;
    const char *marker;  // Unique marker for this connection
} ssl_conn_t;

int get_local_port(int sock) {
    struct sockaddr_in addr;
    socklen_t len = sizeof(addr);
    if (getsockname(sock, (struct sockaddr*)&addr, &len) < 0) {
        return -1;
    }
    return ntohs(addr.sin_port);
}

int connect_to_server(const char *host, int port) {
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) {
        perror("socket");
        return -1;
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);

    if (inet_pton(AF_INET, host, &addr.sin_addr) <= 0) {
        perror("inet_pton");
        close(sock);
        return -1;
    }

    if (connect(sock, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        perror("connect");
        close(sock);
        return -1;
    }

    return sock;
}

ssl_conn_t* create_ssl_connection(const char *host, int port, const char *marker) {
    ssl_conn_t *conn = malloc(sizeof(ssl_conn_t));
    if (!conn) {
        return NULL;
    }
    memset(conn, 0, sizeof(ssl_conn_t));
    conn->marker = marker;
    conn->remote_port = port;

    // Connect TCP
    conn->sock = connect_to_server(host, port);
    if (conn->sock < 0) {
        free(conn);
        return NULL;
    }
    conn->local_port = get_local_port(conn->sock);

    // Create SSL context
    conn->ctx = SSL_CTX_new(TLS_client_method());
    if (!conn->ctx) {
        fprintf(stderr, "SSL_CTX_new failed\n");
        ERR_print_errors_fp(stderr);
        close(conn->sock);
        free(conn);
        return NULL;
    }

    // Don't verify server certificate (test only)
    SSL_CTX_set_verify(conn->ctx, SSL_VERIFY_NONE, NULL);

    // Create SSL connection
    conn->ssl = SSL_new(conn->ctx);
    if (!conn->ssl) {
        fprintf(stderr, "SSL_new failed\n");
        ERR_print_errors_fp(stderr);
        SSL_CTX_free(conn->ctx);
        close(conn->sock);
        free(conn);
        return NULL;
    }

    // Associate SSL with socket
    SSL_set_fd(conn->ssl, conn->sock);

    // Perform SSL handshake
    if (SSL_connect(conn->ssl) <= 0) {
        fprintf(stderr, "SSL_connect failed for %s\n", marker);
        ERR_print_errors_fp(stderr);
        SSL_free(conn->ssl);
        SSL_CTX_free(conn->ctx);
        close(conn->sock);
        free(conn);
        return NULL;
    }

    return conn;
}

void free_ssl_connection(ssl_conn_t *conn) {
    if (conn) {
        if (conn->ssl) {
            SSL_shutdown(conn->ssl);
            SSL_free(conn->ssl);
        }
        if (conn->ctx) {
            SSL_CTX_free(conn->ctx);
        }
        if (conn->sock >= 0) {
            close(conn->sock);
        }
        free(conn);
    }
}

// Send HTTP request and read response
// Returns number of bytes received, or -1 on error
int do_http_request(ssl_conn_t *conn, int iteration) {
    char request[512];
    char response[4096];

    // Create unique request path with marker and iteration
    snprintf(request, sizeof(request),
             "GET /200/%s-iter%d HTTP/1.1\r\n"
             "Host: localhost:%d\r\n"
             "Connection: keep-alive\r\n"
             "\r\n",
             conn->marker, iteration, conn->remote_port);

    int written = SSL_write(conn->ssl, request, strlen(request));
    if (written <= 0) {
        int err = SSL_get_error(conn->ssl, written);
        fprintf(stderr, "SSL_write failed for %s: error %d\n", conn->marker, err);
        return -1;
    }

    int received = SSL_read(conn->ssl, response, sizeof(response) - 1);
    if (received <= 0) {
        int err = SSL_get_error(conn->ssl, received);
        fprintf(stderr, "SSL_read failed for %s: error %d\n", conn->marker, err);
        return -1;
    }

    response[received] = '\0';
    return received;
}

int main(int argc, char *argv[]) {
    if (argc < 5) {
        fprintf(stderr, "Usage: %s <host1> <port1> <host2> <port2> [iterations]\n", argv[0]);
        fprintf(stderr, "Example: %s 127.0.0.1 8001 127.0.0.1 8002 1000\n", argv[0]);
        return 1;
    }

    const char *host1 = argv[1];
    int port1 = atoi(argv[2]);
    const char *host2 = argv[3];
    int port2 = atoi(argv[4]);
    int iterations = (argc > 5) ? atoi(argv[5]) : DEFAULT_ITERATIONS;

    // Initialize OpenSSL
    SSL_load_error_strings();
    OpenSSL_add_ssl_algorithms();

    // Setup signal handler
    struct sigaction sa;
    sa.sa_handler = signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;
    sigaction(SIGUSR1, &sa, NULL);

    // Establish both SSL connections BEFORE signaling ready
    fprintf(stderr, "Establishing connection 1 to %s:%d...\n", host1, port1);
    ssl_conn_t *conn1 = create_ssl_connection(host1, port1, "conn1");
    if (!conn1) {
        fprintf(stderr, "Failed to establish connection 1\n");
        return 1;
    }

    fprintf(stderr, "Establishing connection 2 to %s:%d...\n", host2, port2);
    ssl_conn_t *conn2 = create_ssl_connection(host2, port2, "conn2");
    if (!conn2) {
        fprintf(stderr, "Failed to establish connection 2\n");
        free_ssl_connection(conn1);
        return 1;
    }

    // Print connection info for verification
    // Format: READY:<conn1_local_port>:<conn1_remote_port>:<conn2_local_port>:<conn2_remote_port>
    printf("READY:%d:%d:%d:%d\n",
           conn1->local_port, conn1->remote_port,
           conn2->local_port, conn2->remote_port);
    fflush(stdout);

    fprintf(stderr, "Connections established:\n");
    fprintf(stderr, "  conn1: local=%d -> remote=%d (marker=%s)\n",
            conn1->local_port, conn1->remote_port, conn1->marker);
    fprintf(stderr, "  conn2: local=%d -> remote=%d (marker=%s)\n",
            conn2->local_port, conn2->remote_port, conn2->marker);
    fprintf(stderr, "Waiting for SIGUSR1 to start test (PID=%d)...\n", getpid());

    // Wait for signal
    while (!start_test) {
        pause();
    }

    fprintf(stderr, "Starting rapid interleaved operations (%d iterations)...\n", iterations);

    // Perform rapid interleaved operations
    // The goal is to trigger the race where:
    // 1. SSL_write on conn1 stores ctx1 in ssl_ctx_by_pid_tgid
    // 2. SSL_write on conn2 overwrites with ctx2 BEFORE tcp_sendmsg fires
    // 3. tcp_sendmsg for conn1 reads ctx2 -> misattribution

    int conn1_success = 0, conn1_fail = 0;
    int conn2_success = 0, conn2_fail = 0;

    for (int i = 0; i < iterations; i++) {
        // Interleave: conn1, conn2, conn1, conn2, ...
        // This maximizes the chance of the race condition

        if (do_http_request(conn1, i) > 0) {
            conn1_success++;
        } else {
            conn1_fail++;
        }

        if (do_http_request(conn2, i) > 0) {
            conn2_success++;
        } else {
            conn2_fail++;
        }

        // Progress indicator every 100 iterations
        if ((i + 1) % 100 == 0) {
            fprintf(stderr, "Progress: %d/%d iterations\n", i + 1, iterations);
        }
    }

    fprintf(stderr, "Test complete.\n");
    fprintf(stderr, "Results:\n");
    fprintf(stderr, "  conn1 (port %d->%d): success=%d, fail=%d\n",
            conn1->local_port, conn1->remote_port, conn1_success, conn1_fail);
    fprintf(stderr, "  conn2 (port %d->%d): success=%d, fail=%d\n",
            conn2->local_port, conn2->remote_port, conn2_success, conn2_fail);

    // Output summary line for parsing
    // Format: DONE:<conn1_success>:<conn1_fail>:<conn2_success>:<conn2_fail>
    printf("DONE:%d:%d:%d:%d\n", conn1_success, conn1_fail, conn2_success, conn2_fail);
    fflush(stdout);

    // Cleanup
    free_ssl_connection(conn1);
    free_ssl_connection(conn2);

    return 0;
}
