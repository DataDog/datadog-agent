#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>

#ifndef DD_AGENT_PATH
#error DD_AGENT_PATH must be defined
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
        fprintf(stderr, "Cannot determine agent location\n");
        exit(1);
    }

    setenv("DD_BUNDLED_AGENT", DD_AGENT, 0);

    execvp(DD_AGENT_PATH, argv);
    return 1;
}
