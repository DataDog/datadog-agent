#include <stdio.h>
#include <stdlib.h>
#include <strings.h>
#include <sys/socket.h>
#include <arpa/inet.h>
#include <poll.h>
#include <unistd.h>

int is_apm_enabled() {
    char *apm_enabled = getenv("DD_APM_ENABLED");
    return apm_enabled == NULL || strcasecmp(apm_enabled, "true") != 0;
}

int is_containerized() {
    // check if the process is running in a container
    char *docker_dd_agent = getenv("DOCKER_DD_AGENT");
    return docker_dd_agent != NULL && strcmp(docker_dd_agent, "") != 0;
}

// logic from comp/trace/config/setup.go
const char *get_bind_host() {
    char *apm_non_local_traffic = getenv("DD_APM_NON_LOCAL_TRAFFIC");
    if (apm_non_local_traffic != NULL && strcasecmp(apm_non_local_traffic, "true") == 0) {
        return "0:0:0:0";
    }

    char *bind_host = getenv("DD_BIND_HOST");
    if (bind_host != NULL && strcmp(bind_host, "") != 0) {
        return bind_host;
    }

    if (is_containerized()) {
        return "0:0:0:0";
    }

    return bind_host;
}

uint16_t get_apm_receiver_port() {
    char *apm_receiver_port = getenv("DD_APM_RECEIVER_PORT");
    if (apm_receiver_port != NULL && strcmp(apm_receiver_port, "") != 0) {
        return atoi(apm_receiver_port);
    }
    return 8126;
}

const char *get_apm_receiver_socket_path() {
    char *apm_receiver_socket = getenv("DD_APM_RECEIVER_SOCKET");
    if (apm_receiver_socket != NULL && strcmp(apm_receiver_socket, "") != 0) {
        return apm_receiver_socket;
    }
    return "/var/run/datadog/apm.socket";
}

int apm_receiver_net_socket() {
    uint16_t apm_receiver_port = get_apm_receiver_port();
    if (apm_receiver_port < 0) {
        printf("Invalid apm receiver port number: %d\n", apm_receiver_port);
        return -1;
    }

    if (apm_receiver_port == 0) {
        printf("APM receiver port is disabled\n");
        return -1;
    }

    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock == -1) {
        perror("socket");
        return -1;
    }

    struct sockaddr_in address;
    address.sin_family = AF_INET;
    // TODO: use real bind host
    //const char *bind_host = get_bind_host();
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(apm_receiver_port);

    if (bind(sock, (struct sockaddr *)&address, sizeof(address)) != 0) {
        perror("bind");
        return -1;
    }

    if (listen(sock, 32) != 0) {
        perror("listen");
        return -1;
    }

    return sock;
}

int apm_receiver_unix_socket() {
    //TODO
    return -1;
}

int main(int argc, char **argv) {
    if (!is_apm_enabled()) {
        printf("APM is disabled\n");
        return 0;
    }

    struct pollfd fds[2];
    nfds_t nfds = 0;

    int net_fd = apm_receiver_net_socket();
    if (net_fd != -1) {
        fds[nfds].fd = net_fd;
        fds[nfds].events = POLLIN;
        nfds++;

        char port_str[20];
        snprintf(port_str, 20, "%d", net_fd);
        setenv("DD_APM_NET_RECEIVER_FD", port_str, 1);
    }

    int unix_fd = apm_receiver_unix_socket();
    if (unix_fd != -1) {
        fds[nfds].fd = unix_fd;
        fds[nfds].events = POLLIN;
        nfds++;

        char port_str[20];
        snprintf(port_str, 20, "%d", unix_fd);
        setenv("DD_APM_UNIX_RECEIVER_FD", port_str, 1);
    }

    if (nfds == 0) {
        printf("Neither net nor unix receiver are available.");
        return 1;
    }

    while (1) {
        int ret = poll(fds, nfds, -1);
        if (ret > 0) {
            execv(argv[1], argv + 1);
        }
        perror("poll");
    }
}
