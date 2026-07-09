// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

/*
 * ssl_ctx_race_v2 - Diagnostic test for ssl_ctx_by_pid_tgid race condition
 *
 * This version adds different test modes to isolate whether the issue is:
 * 1. Race condition on SSL_write (tcp_sendmsg path)
 * 2. Missing correlation on SSL_read (tcp_recvmsg path)
 * 3. Both
 *
 * Test modes:
 *   --writes-only     Only do SSL_write, skip SSL_read (isolates write path)
 *   --interleaved     Interleave SSL_write calls before reading responses
 *   --sequential      Original sequential behavior (default)
 *
 * Usage: ssl_ctx_race_v2 <host1> <port1> <host2> <port2> [iterations] [--mode]
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

#define DEFAULT_ITERATIONS 500

typedef enum {
    MODE_SEQUENTIAL,      // Original: write1+read1, write2+read2
    MODE_WRITES_ONLY,     // Only writes: write1, write2, write1, write2...
    MODE_INTERLEAVED,     // Interleaved writes: write1, write2, then read1, read2
} test_mode_t;

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
    const char *marker;
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
    if (!conn) return NULL;
    memset(conn, 0, sizeof(ssl_conn_t));
    conn->marker = marker;
    conn->remote_port = port;

    conn->sock = connect_to_server(host, port);
    if (conn->sock < 0) {
        free(conn);
        return NULL;
    }
    conn->local_port = get_local_port(conn->sock);

    conn->ctx = SSL_CTX_new(TLS_client_method());
    if (!conn->ctx) {
        close(conn->sock);
        free(conn);
        return NULL;
    }
    SSL_CTX_set_verify(conn->ctx, SSL_VERIFY_NONE, NULL);

    conn->ssl = SSL_new(conn->ctx);
    if (!conn->ssl) {
        SSL_CTX_free(conn->ctx);
        close(conn->sock);
        free(conn);
        return NULL;
    }

    SSL_set_fd(conn->ssl, conn->sock);

    if (SSL_connect(conn->ssl) <= 0) {
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
        if (conn->ctx) SSL_CTX_free(conn->ctx);
        if (conn->sock >= 0) close(conn->sock);
        free(conn);
    }
}

// Only write, don't read response
int do_ssl_write_only(ssl_conn_t *conn, int iteration) {
    char request[512];
    snprintf(request, sizeof(request),
             "GET /200/%s-iter%d HTTP/1.1\r\n"
             "Host: localhost:%d\r\n"
             "Connection: keep-alive\r\n"
             "\r\n",
             conn->marker, iteration, conn->remote_port);

    int written = SSL_write(conn->ssl, request, strlen(request));
    if (written <= 0) {
        fprintf(stderr, "SSL_write failed for %s iter %d\n", conn->marker, iteration);
        return -1;
    }
    return written;
}

// Only read (assumes a pending response)
int do_ssl_read_only(ssl_conn_t *conn) {
    char response[4096];
    int received = SSL_read(conn->ssl, response, sizeof(response) - 1);
    if (received <= 0) {
        fprintf(stderr, "SSL_read failed for %s\n", conn->marker);
        return -1;
    }
    return received;
}

// Original sequential: write+read
int do_http_request(ssl_conn_t *conn, int iteration) {
    if (do_ssl_write_only(conn, iteration) < 0) return -1;
    return do_ssl_read_only(conn);
}

void run_sequential(ssl_conn_t *conn1, ssl_conn_t *conn2, int iterations) {
    fprintf(stderr, "MODE: SEQUENTIAL (write1+read1, write2+read2)\n");
    fprintf(stderr, "This tests the original behavior.\n\n");

    for (int i = 0; i < iterations; i++) {
        do_http_request(conn1, i);
        do_http_request(conn2, i);
        if ((i + 1) % 100 == 0) {
            fprintf(stderr, "Progress: %d/%d\n", i + 1, iterations);
        }
    }
}

void run_writes_only(ssl_conn_t *conn1, ssl_conn_t *conn2, int iterations) {
    fprintf(stderr, "MODE: WRITES_ONLY (write1, write2, write1, write2...)\n");
    fprintf(stderr, "This isolates SSL_write -> tcp_sendmsg path.\n");
    fprintf(stderr, "If race exists, writes should still be misattributed.\n\n");

    // Do all writes first
    for (int i = 0; i < iterations; i++) {
        do_ssl_write_only(conn1, i);
        do_ssl_write_only(conn2, i);
        if ((i + 1) % 100 == 0) {
            fprintf(stderr, "Progress: %d/%d writes\n", i + 1, iterations);
        }
    }

    fprintf(stderr, "All writes done. Now draining responses...\n");

    // Drain responses (ignore errors - server may timeout)
    for (int i = 0; i < iterations; i++) {
        do_ssl_read_only(conn1);
        do_ssl_read_only(conn2);
    }
}

void run_interleaved(ssl_conn_t *conn1, ssl_conn_t *conn2, int iterations) {
    fprintf(stderr, "MODE: INTERLEAVED (write1, write2, read1, read2 per iteration)\n");
    fprintf(stderr, "This creates maximum race window between writes.\n");
    fprintf(stderr, "Both writes happen before either tcp_sendmsg completes.\n\n");

    for (int i = 0; i < iterations; i++) {
        // Write to both connections back-to-back
        // This should maximize the race window
        do_ssl_write_only(conn1, i);
        do_ssl_write_only(conn2, i);

        // Then read both responses
        do_ssl_read_only(conn1);
        do_ssl_read_only(conn2);

        if ((i + 1) % 100 == 0) {
            fprintf(stderr, "Progress: %d/%d\n", i + 1, iterations);
        }
    }
}

int main(int argc, char *argv[]) {
    if (argc < 5) {
        fprintf(stderr, "Usage: %s <host1> <port1> <host2> <port2> [iterations] [--sequential|--writes-only|--interleaved]\n", argv[0]);
        return 1;
    }

    const char *host1 = argv[1];
    int port1 = atoi(argv[2]);
    const char *host2 = argv[3];
    int port2 = atoi(argv[4]);
    int iterations = DEFAULT_ITERATIONS;
    test_mode_t mode = MODE_SEQUENTIAL;

    for (int i = 5; i < argc; i++) {
        if (strcmp(argv[i], "--sequential") == 0) {
            mode = MODE_SEQUENTIAL;
        } else if (strcmp(argv[i], "--writes-only") == 0) {
            mode = MODE_WRITES_ONLY;
        } else if (strcmp(argv[i], "--interleaved") == 0) {
            mode = MODE_INTERLEAVED;
        } else {
            iterations = atoi(argv[i]);
        }
    }

    SSL_load_error_strings();
    OpenSSL_add_ssl_algorithms();

    struct sigaction sa;
    sa.sa_handler = signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;
    sigaction(SIGUSR1, &sa, NULL);

    fprintf(stderr, "Establishing connection 1 to %s:%d...\n", host1, port1);
    ssl_conn_t *conn1 = create_ssl_connection(host1, port1, "conn1");
    if (!conn1) {
        fprintf(stderr, "Failed to establish connection 1\n");
        return 1;
    }

    fprintf(stderr, "Establishing connection 2 to %s:%d...\n", host2, port2);
    ssl_conn_t *conn2 = create_ssl_connection(host2, port2, "conn2");
    if (!conn2) {
        free_ssl_connection(conn1);
        return 1;
    }

    printf("READY:%d:%d:%d:%d\n",
           conn1->local_port, conn1->remote_port,
           conn2->local_port, conn2->remote_port);
    fflush(stdout);

    fprintf(stderr, "Connections established:\n");
    fprintf(stderr, "  conn1: local=%d -> remote=%d\n", conn1->local_port, conn1->remote_port);
    fprintf(stderr, "  conn2: local=%d -> remote=%d\n", conn2->local_port, conn2->remote_port);
    fprintf(stderr, "\n");
    fprintf(stderr, "IMPORTANT: Start system-probe NOW, then send SIGUSR1\n");
    fprintf(stderr, "Waiting for SIGUSR1 (PID=%d)...\n", getpid());

    while (!start_test) {
        pause();
    }

    fprintf(stderr, "\nStarting test with %d iterations...\n\n", iterations);

    switch (mode) {
        case MODE_SEQUENTIAL:
            run_sequential(conn1, conn2, iterations);
            break;
        case MODE_WRITES_ONLY:
            run_writes_only(conn1, conn2, iterations);
            break;
        case MODE_INTERLEAVED:
            run_interleaved(conn1, conn2, iterations);
            break;
    }

    fprintf(stderr, "\nTest complete. Check /debug/http_monitoring for results.\n");
    printf("DONE\n");
    fflush(stdout);

    free_ssl_connection(conn1);
    free_ssl_connection(conn2);
    return 0;
}
