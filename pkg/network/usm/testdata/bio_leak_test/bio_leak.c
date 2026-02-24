// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ignore

/*
 * bio_leak - Test helper for fd_by_ssl_bio map leak
 *
 * This program creates stale entries in the fd_by_ssl_bio eBPF map by calling
 * BIO_new_socket() without a subsequent SSL_set_bio() call.
 *
 * Usage: bio_leak <server_host> <server_port> <num_entries>
 *
 * The program:
 * 1. Connects to the specified TLS server
 * 2. Calls BIO_new_socket(fd) - this triggers uretprobe that adds entry to map
 * 3. Does NOT call SSL_set_bio() - entry is never deleted
 * 4. Keeps BIOs alive to prevent address reuse, ensuring unique entries
 * 5. Exits after creating the specified number of stale entries
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <openssl/ssl.h>
#include <openssl/bio.h>
#include <openssl/err.h>

#define MAX_ENTRIES 1024

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

int main(int argc, char *argv[]) {
    if (argc != 4) {
        fprintf(stderr, "Usage: %s <host> <port> <num_entries>\n", argv[0]);
        fprintf(stderr, "Example: %s 127.0.0.1 8443 100\n", argv[0]);
        return 1;
    }

    const char *host = argv[1];
    int port = atoi(argv[2]);
    int num_entries = atoi(argv[3]);

    if (num_entries <= 0 || num_entries > MAX_ENTRIES) {
        fprintf(stderr, "num_entries must be between 1 and %d\n", MAX_ENTRIES);
        return 1;
    }

    // Initialize OpenSSL
    SSL_load_error_strings();
    OpenSSL_add_ssl_algorithms();

    // Arrays to keep BIOs and sockets alive (prevent address reuse)
    BIO *bios[MAX_ENTRIES];
    int sockets[MAX_ENTRIES];
    int created = 0;

    for (int i = 0; i < num_entries; i++) {
        // Connect to server
        int sock = connect_to_server(host, port);
        if (sock < 0) {
            fprintf(stderr, "Failed to connect for entry %d\n", i);
            continue;
        }

        // Create BIO - this triggers uretprobe__BIO_new_socket
        // which adds entry to fd_by_ssl_bio map
        BIO *bio = BIO_new_socket(sock, BIO_NOCLOSE);
        if (bio == NULL) {
            fprintf(stderr, "BIO_new_socket failed for entry %d\n", i);
            close(sock);
            continue;
        }

        // IMPORTANT: Do NOT call SSL_set_bio()
        // This leaves the entry in fd_by_ssl_bio map (LEAK!)

        // Keep BIO and socket alive to prevent address reuse
        bios[created] = bio;
        sockets[created] = sock;
        created++;
    }

    // Print results for test verification
    printf("CREATED:%d\n", created);
    fflush(stdout);

    // Clean up - BIO_free triggers uprobe__BIO_free which removes the map entry
    for (int i = 0; i < created; i++) {
        if (bios[i]) {
            BIO_free(bios[i]);
        }
        if (sockets[i] >= 0) {
            close(sockets[i]);
        }
    }

    return 0;
}
