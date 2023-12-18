#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>

#define DEBUG 0

#ifndef DD_AGENT_PATH
#error DD_AGENT_PATH must be defined
#endif

#ifndef DD_AGENT
#define DD_AGENT "agent"
#endif

#if _WIN32
#include <Windows.h>
#include <shlwapi.h>

TCHAR executable[MAX_PATH*2];

char *get_process_executable() {
    DWORD result = GetModuleFileName(NULL, executable, MAX_PATH);
    if (result == MAX_PATH || result == 0)
        return NULL;
    PathRemoveFileSpec(executable);
    strncat(executable, "\\" DD_AGENT_PATH, MAX_PATH);
#if DEBUG
    printf("\nProcess executable: %s (%d)\n", executable, result);
#endif
    return executable;
}

int execute_process(char *executable, int argc, char **argv) {
    PROCESS_INFORMATION processInformation = {0};
    STARTUPINFO startupInfo                = {0};
    startupInfo.cb                         = sizeof(startupInfo);

    char cmdLine[32767] = {0};
    strncat(cmdLine, DD_AGENT, sizeof(cmdLine)-1);

    for (int i = 1; i < argc ; i++) {
        strncat(cmdLine, " ", sizeof(cmdLine)-1);
        strncat(cmdLine, "\"", sizeof(cmdLine)-1);
        strncat(cmdLine, argv[i], sizeof(cmdLine)-1);
        strncat(cmdLine, "\"", sizeof(cmdLine)-1);
    }

#if DEBUG
    printf("Executing %s with %s\n", executable, cmdLine);
#endif

    // Create the process
    BOOL result = CreateProcess(executable, cmdLine,
                                NULL, NULL, TRUE,
                                NORMAL_PRIORITY_CLASS,
                                NULL, NULL, &startupInfo, &processInformation);

    if (result == 0) {
        fprintf(stderr, "Failed to execute %s: error %d\n", executable, GetLastError());
        return GetLastError();
    }

    WaitForSingleObject(processInformation.hProcess, INFINITE);

    DWORD rc;
    GetExitCodeProcess(processInformation.hProcess, &rc);

    return rc;
}

#else

char *get_process_executable(void) {
    return DD_AGENT_PATH;
}

int execute_process(char *executable, int argc, char **argv) {
    int rc = execvp(executable, argv);
    fprintf(stderr, "Failed to execute %s (%s)\n", executable, strerror(rc));
    return rc;
}

#endif

int main(int argc, char **argv) {
    char *executable = get_process_executable();

    if (DD_AGENT_PATH == NULL || strlen(DD_AGENT_PATH) == 0) {
        fprintf(stderr, "Cannot determine agent location\n");
        exit(1);
    }

    if (argc > 0) {
        argv[0] = DD_AGENT;
    } else {
        argv = malloc(sizeof(char *) * 2);
        argv[0] = DD_AGENT;
        argv[1] = NULL;
        argc = 2;
    }

    return execute_process(executable, argc, argv);
}
