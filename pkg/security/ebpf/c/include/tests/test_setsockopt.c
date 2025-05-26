#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <netinet/in.h>

int main() {
    int sockfd;
    int optval = 1;

    // Ouvre une socket IPv4 TCP
    sockfd = socket(AF_INET, SOCK_STREAM, 0);
    if (sockfd < 0) {
        perror("socket");
        return 1;
    }
    printf("Socket avec fd %d ouverte.\n", sockfd);

    // Utilise setsockopt pour activer SO_REUSEADDR
    if (setsockopt(sockfd, SOL_SOCKET, SO_REUSEADDR, &optval, sizeof(optval)) < 0) {
        perror("setsockopt");
        close(sockfd);
        return 1;
    }
    printf("setsockopt appelé avec level = %d, optname = %d.\n", SOL_SOCKET, SO_REUSEADDR);

    // Ferme la socket
    close(sockfd);

    printf("Socket ouverte, setsockopt appelé, socket fermée.\n");
    return 0;
}