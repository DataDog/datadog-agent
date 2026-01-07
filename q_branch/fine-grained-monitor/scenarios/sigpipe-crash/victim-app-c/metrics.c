/*
 * metrics.c - C library that writes to a Unix Domain Socket.
 *
 * This library demonstrates SIGPIPE crash behavior. It:
 * - Spawns a background thread at library load time via __attribute__((constructor))
 * - Maintains a persistent UDS connection
 * - Does NOT handle SIGPIPE (default disposition = terminate)
 *
 * When the UDS server closes the connection, write() triggers SIGPIPE,
 * terminating the process with exit code 141 (128 + 13).
 */

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <signal.h>
#include <pthread.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <time.h>

/* Command types for the writer thread */
#define CMD_CONNECT 1
#define CMD_WRITE   2
#define CMD_CLOSE   3
#define CMD_SHUTDOWN 4

/* Shared state protected by mutex */
static pthread_mutex_t g_mutex = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t g_cmd_cond = PTHREAD_COND_INITIALIZER;
static pthread_cond_t g_resp_cond = PTHREAD_COND_INITIALIZER;
static int g_command = 0;
static int g_response = 0;
static int g_response_ready = 0;
static char g_socket_path[256] = {0};
static pthread_t g_writer_thread;
static int g_initialized = 0;

/* Writer thread's socket fd */
static int g_socket_fd = -1;

/*
 * SIGPIPE handler that terminates with exit code 141.
 *
 * Per Go's signal documentation: "If the SIGPIPE is received on a non-Go
 * thread the signal will be forwarded to the non-Go handler, if any."
 *
 * Go's runtime always intercepts signals first. Setting SIG_DFL doesn't
 * work because Go handles SIGPIPE internally (returning EPIPE). The only
 * way to crash on SIGPIPE is to have a handler that Go forwards to.
 *
 * Exit code 141 = 128 + 13 (SIGPIPE), matching kernel default behavior.
 */
static void sigpipe_crash_handler(int sig) {
    (void)sig;
    const char msg[] = "[SIGPIPE] Signal received! Terminating (exit 141)\n";
    write(STDERR_FILENO, msg, sizeof(msg) - 1);
    _exit(141);
}

static void* writer_thread_main(void* arg) {
    (void)arg;

    /*
     * Ensure SIGPIPE has default disposition in this thread.
     * This is the KEY to reproducing the crash - we explicitly
     * set SIG_DFL and unblock SIGPIPE.
     */
    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = SIG_DFL;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;
    sigaction(SIGPIPE, &sa, NULL);

    /* Unblock SIGPIPE for this thread */
    sigset_t sigset;
    sigemptyset(&sigset);
    sigaddset(&sigset, SIGPIPE);
    pthread_sigmask(SIG_UNBLOCK, &sigset, NULL);

    fprintf(stderr, "[writer-c] Thread started, SIGPIPE=SIG_DFL\n");

    while (1) {
        pthread_mutex_lock(&g_mutex);
        while (g_command == 0) {
            pthread_cond_wait(&g_cmd_cond, &g_mutex);
        }
        int cmd = g_command;
        g_command = 0;
        pthread_mutex_unlock(&g_mutex);

        int result = 0;

        switch (cmd) {
            case CMD_CONNECT: {
                fprintf(stderr, "[writer-c] Connecting to %s\n", g_socket_path);

                int fd = socket(AF_UNIX, SOCK_STREAM, 0);
                if (fd < 0) {
                    fprintf(stderr, "[writer-c] socket() failed: %s\n", strerror(errno));
                    result = -1;
                    break;
                }

                struct sockaddr_un addr;
                memset(&addr, 0, sizeof(addr));
                addr.sun_family = AF_UNIX;
                strncpy(addr.sun_path, g_socket_path, sizeof(addr.sun_path) - 1);

                if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
                    fprintf(stderr, "[writer-c] connect() failed: %s\n", strerror(errno));
                    close(fd);
                    result = -1;
                    break;
                }

                g_socket_fd = fd;
                fprintf(stderr, "[writer-c] Connected (fd=%d)\n", fd);
                result = 0;
                break;
            }

            case CMD_WRITE: {
                if (g_socket_fd < 0) {
                    fprintf(stderr, "[writer-c] Not connected\n");
                    result = -1;
                    break;
                }

                /*
                 * Install our SIGPIPE crash handler before each write.
                 * Go's runtime intercepts all signals and only forwards
                 * to non-Go handlers. SIG_DFL doesn't work - we need an
                 * actual handler for Go to forward to.
                 */
                struct sigaction old_sa;
                sigaction(SIGPIPE, NULL, &old_sa);
                if (old_sa.sa_handler != sigpipe_crash_handler) {
                    fprintf(stderr, "[writer-c] Installing SIGPIPE crash handler\n");
                    struct sigaction new_sa;
                    memset(&new_sa, 0, sizeof(new_sa));
                    new_sa.sa_handler = sigpipe_crash_handler;
                    sigemptyset(&new_sa.sa_mask);
                    new_sa.sa_flags = 0;
                    sigaction(SIGPIPE, &new_sa, NULL);
                }

                /* Also ensure SIGPIPE is unblocked for this thread */
                sigset_t current_mask;
                pthread_sigmask(SIG_BLOCK, NULL, &current_mask);
                if (sigismember(&current_mask, SIGPIPE)) {
                    fprintf(stderr, "[writer-c] SIGPIPE was blocked, unblocking\n");
                    sigset_t unblock_set;
                    sigemptyset(&unblock_set);
                    sigaddset(&unblock_set, SIGPIPE);
                    pthread_sigmask(SIG_UNBLOCK, &unblock_set, NULL);
                }

                char payload[128];
                snprintf(payload, sizeof(payload),
                    "{\"timestamp\":%ld,\"cpu\":42.5,\"memory\":1024}\n",
                    (long)time(NULL));

                /*
                 * This write() will trigger SIGPIPE if the socket is closed.
                 * With SIGPIPE set to SIG_DFL, the process will terminate
                 * with exit code 141 (128 + SIGPIPE).
                 */
                ssize_t written = write(g_socket_fd, payload, strlen(payload));
                if (written < 0) {
                    fprintf(stderr, "[writer-c] write() failed: %s (errno=%d)\n",
                            strerror(errno), errno);
                    result = -1;
                } else {
                    result = 0;
                }
                break;
            }

            case CMD_CLOSE: {
                if (g_socket_fd >= 0) {
                    close(g_socket_fd);
                    g_socket_fd = -1;
                }
                fprintf(stderr, "[writer-c] Connection closed\n");
                result = 0;
                break;
            }

            case CMD_SHUTDOWN: {
                fprintf(stderr, "[writer-c] Shutting down\n");
                pthread_mutex_lock(&g_mutex);
                g_response = 0;
                g_response_ready = 1;
                pthread_cond_signal(&g_resp_cond);
                pthread_mutex_unlock(&g_mutex);
                return NULL;
            }
        }

        /* Signal response */
        pthread_mutex_lock(&g_mutex);
        g_response = result;
        g_response_ready = 1;
        pthread_cond_signal(&g_resp_cond);
        pthread_mutex_unlock(&g_mutex);
    }

    return NULL;
}

/* Library constructor - runs at load time before main() */
__attribute__((constructor))
static void init_writer_thread(void) {
    fprintf(stderr, "[ctor-c] Spawning writer thread (outside Go's runtime)\n");

    if (pthread_create(&g_writer_thread, NULL, writer_thread_main, NULL) != 0) {
        fprintf(stderr, "[ctor-c] Failed to create writer thread\n");
        return;
    }

    g_initialized = 1;
    fprintf(stderr, "[ctor-c] Writer thread spawned\n");
}

/* Helper to send command and wait for response */
static int send_command(int cmd) {
    if (!g_initialized) {
        fprintf(stderr, "Writer thread not initialized\n");
        return -1;
    }

    pthread_mutex_lock(&g_mutex);
    g_command = cmd;
    g_response_ready = 0;
    pthread_cond_signal(&g_cmd_cond);

    while (!g_response_ready) {
        pthread_cond_wait(&g_resp_cond, &g_mutex);
    }

    int result = g_response;
    pthread_mutex_unlock(&g_mutex);

    return result;
}

/* Public API - called from Go via CGO */
int init_metrics(const char* socket_path) {
    pthread_mutex_lock(&g_mutex);
    strncpy(g_socket_path, socket_path, sizeof(g_socket_path) - 1);
    pthread_mutex_unlock(&g_mutex);

    return send_command(CMD_CONNECT);
}

int write_metrics(void) {
    return send_command(CMD_WRITE);
}

void close_metrics(void) {
    send_command(CMD_CLOSE);
}
