// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ignore

/*
 * ssl_threadpool - Test helper for ssl_sock_by_ctx leaks in multi-threaded clients
 *
 * Usage: ssl_threadpool <host> <port> <out_dir> <connections> <threads> <requests_per_thread> <rotations>
 *
 * The program mimics a client with a thread pool sharing a set of TLS connections:
 * 1. Writes its PID to <out_dir>/pid, prints SSL_THREADPOOL_READY and waits for
 *    <out_dir>/start to exist, so the test can wait for the uprobes to be attached first
 * 2. Opens <connections> TLS connections (SSL_set_fd + SSL_connect)
 * 3. Starts <threads> worker threads; each one sends <requests_per_thread> HTTP/1.1
 *    requests on randomly picked connections (one mutex per connection), so every
 *    connection is used by many threads
 * 4. Meanwhile the main thread closes and reopens connections <rotations> times
 * 5. Joins the workers, writes the number of completed requests to <out_dir>/paused and waits
 *    for <out_dir>/close to exist, so the test can check the map while the connections are open
 * 6. Closes every connection cleanly (SSL_shutdown + SSL_free + close)
 * 7. Writes the number of completed requests to <out_dir>/requests and stays alive, so
 *    entries left in the map can't be removed by the dead-process cleaner
 *
 * Every request path is /200/ssl-threadpool/p<local port>, so the test can check that USM
 * attributed each request to the connection it was sent on.
 */

#define _GNU_SOURCE // strcasestr

#include <arpa/inet.h>
#include <netinet/in.h>
#include <openssl/ssl.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#define MAX_CONNECTIONS 64
#define MAX_THREADS 256

struct connection {
    SSL *ssl;
    int fd;
    int local_port;
    pthread_mutex_t mu;
};

static SSL_CTX *ssl_ctx;
static struct connection conns[MAX_CONNECTIONS];
static const char *host;
static int port, num_conns, requests_per_thread;
static int completed;
static pthread_mutex_t completed_mu = PTHREAD_MUTEX_INITIALIZER;

static void fail(const char *msg) {
    printf("SSL_THREADPOOL_ERROR %s\n", msg);
    fflush(stdout);
    exit(1);
}

static void open_connection(int i) {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in addr = {.sin_family = AF_INET, .sin_port = htons(port)};
    inet_pton(AF_INET, host, &addr.sin_addr);
    if (fd < 0 || connect(fd, (struct sockaddr *)&addr, sizeof(addr)) != 0) {
        fail("connect");
    }
    struct sockaddr_in local;
    socklen_t len = sizeof(local);
    getsockname(fd, (struct sockaddr *)&local, &len);

    SSL *ssl = SSL_new(ssl_ctx);
    SSL_set_fd(ssl, fd);
    if (SSL_connect(ssl) != 1) {
        fail("SSL_connect");
    }
    conns[i].ssl = ssl;
    conns[i].fd = fd;
    conns[i].local_port = ntohs(local.sin_port);
}

static void close_connection(int i) {
    SSL_shutdown(conns[i].ssl);
    SSL_free(conns[i].ssl);
    close(conns[i].fd);
}

// Sends one request on connection i and reads the whole response. The caller holds conns[i].mu.
static int do_request(int i) {
    char req[128], resp[4096];
    int n = snprintf(req, sizeof(req), "GET /200/ssl-threadpool/p%d HTTP/1.1\r\nHost: localhost\r\n\r\n", conns[i].local_port);
    if (SSL_write(conns[i].ssl, req, n) != n) {
        return 0;
    }
    int got = 0;
    for (;;) {
        int r = SSL_read(conns[i].ssl, resp + got, sizeof(resp) - 1 - got);
        if (r <= 0) {
            return 0;
        }
        got += r;
        resp[got] = 0;
        char *body = strstr(resp, "\r\n\r\n");
        if (body == NULL) {
            continue;
        }
        char *cl = strcasestr(resp, "Content-Length:");
        int content_length = cl ? atoi(cl + strlen("Content-Length:")) : 0;
        if (got - (body + 4 - resp) >= content_length) {
            return 1;
        }
    }
}

static void *worker(void *arg) {
    unsigned int seed = (unsigned int)(long)arg;
    for (int n = 0; n < requests_per_thread; n++) {
        int i = rand_r(&seed) % num_conns;
        pthread_mutex_lock(&conns[i].mu);
        int ok = do_request(i);
        pthread_mutex_unlock(&conns[i].mu);
        if (ok) {
            pthread_mutex_lock(&completed_mu);
            completed++;
            pthread_mutex_unlock(&completed_mu);
        }
    }
    return NULL;
}

// Writes value to <dir>/<name> through a temporary file and a rename, so the test never reads a
// partial file.
static void write_int_file(const char *dir, const char *name, int value) {
    char path[512], tmp_path[520];
    snprintf(path, sizeof(path), "%s/%s", dir, name);
    snprintf(tmp_path, sizeof(tmp_path), "%s.tmp", path);
    FILE *f = fopen(tmp_path, "w");
    if (f == NULL) {
        fail("fopen");
    }
    fprintf(f, "%d\n", value);
    fclose(f);
    if (rename(tmp_path, path) != 0) {
        fail("rename");
    }
}

int main(int argc, char *argv[]) {
    if (argc != 8) {
        fprintf(stderr, "Usage: %s <host> <port> <out_dir> <connections> <threads> <requests_per_thread> <rotations>\n", argv[0]);
        return 1;
    }
    host = argv[1];
    port = atoi(argv[2]);
    const char *out_dir = argv[3];
    num_conns = atoi(argv[4]);
    int num_threads = atoi(argv[5]);
    requests_per_thread = atoi(argv[6]);
    int rotations = atoi(argv[7]);
    if (num_conns <= 0 || num_conns > MAX_CONNECTIONS || num_threads <= 0 || num_threads > MAX_THREADS) {
        fail("connections must be in [1, 64] and threads in [1, 256]");
    }
    // A connection reset by the server must fail the request, not kill the process.
    signal(SIGPIPE, SIG_IGN);

    char start_path[512], close_path[512];
    snprintf(start_path, sizeof(start_path), "%s/start", out_dir);
    snprintf(close_path, sizeof(close_path), "%s/close", out_dir);

    ssl_ctx = SSL_CTX_new(TLS_client_method());
    SSL_CTX_set_verify(ssl_ctx, SSL_VERIFY_NONE, NULL);

    write_int_file(out_dir, "pid", getpid());
    printf("SSL_THREADPOOL_READY\n");
    fflush(stdout);
    while (access(start_path, F_OK) != 0) {
        usleep(100 * 1000);
    }

    for (int i = 0; i < num_conns; i++) {
        pthread_mutex_init(&conns[i].mu, NULL);
        open_connection(i);
    }

    pthread_t threads[MAX_THREADS];
    for (int t = 0; t < num_threads; t++) {
        pthread_create(&threads[t], NULL, worker, (void *)(long)(t + 1));
    }
    for (int r = 0; r < rotations; r++) {
        int i = r % num_conns;
        usleep(1000);
        pthread_mutex_lock(&conns[i].mu);
        close_connection(i);
        open_connection(i);
        pthread_mutex_unlock(&conns[i].mu);
    }
    for (int t = 0; t < num_threads; t++) {
        pthread_join(threads[t], NULL);
    }

    write_int_file(out_dir, "paused", completed);
    while (access(close_path, F_OK) != 0) {
        usleep(100 * 1000);
    }

    for (int i = 0; i < num_conns; i++) {
        close_connection(i);
    }

    write_int_file(out_dir, "requests", completed);

    // Stay alive until the test stops the container.
    for (;;) {
        pause();
    }
}
