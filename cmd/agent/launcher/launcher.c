#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>

#ifndef DD_AGENT_PATH
#define DD_AGENT_PATH ""
#endif

#ifndef DD_AGENT
#define DD_AGENT "agent"
#endif

int main(int argc, char **argv) {
    if (argc > 1) {
        argv[0] = DD_AGENT;
    } else {
        argv = malloc(sizeof(char *) * 2);
        argv[0] = DD_AGENT;
        argv[1] = NULL;
    }

    if (strlen(DD_AGENT_PATH) == 0) {
        printf("Cannot determine agent location");
        exit(1);
    }

    execvp(DD_AGENT_PATH, argv);
}
