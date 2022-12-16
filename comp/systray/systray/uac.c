// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include "uac.h"

BOOL LaunchUnelevated(LPCWSTR CommandLine)
{
    BOOL result = FALSE;
    HWND hwnd = GetShellWindow();

    if (hwnd != NULL)
    {
        DWORD pid;
        if (GetWindowThreadProcessId(hwnd, &pid) != 0)
        {
            HANDLE process = OpenProcess(PROCESS_CREATE_PROCESS, FALSE, pid);

            if (process != NULL)
            {
                SIZE_T size;
                if ((!InitializeProcThreadAttributeList(NULL, 1, 0, &size)) && (GetLastError() == ERROR_INSUFFICIENT_BUFFER))
                {
                    LPPROC_THREAD_ATTRIBUTE_LIST p = (LPPROC_THREAD_ATTRIBUTE_LIST)malloc(size);
                    if (p != NULL)
                    {
                        if (InitializeProcThreadAttributeList(p, 1, 0, &size))
                        {
                            if (UpdateProcThreadAttribute(p, 0,
                                                          PROC_THREAD_ATTRIBUTE_PARENT_PROCESS,
                                                          &process, sizeof(process),
                                                          NULL, NULL))
                            {
                                STARTUPINFOEXW siex = {0};
                                siex.lpAttributeList = p;
                                siex.StartupInfo.cb = sizeof(siex);
                                PROCESS_INFORMATION pi = {0};

                                size_t cmdlen = wcslen(CommandLine);
                                size_t rawcmdlen = (cmdlen + 1) * sizeof(WCHAR);
                                PWSTR cmdstr = (PWSTR)malloc(rawcmdlen);
                                if (cmdstr != NULL)
                                {
                                    memcpy(cmdstr, CommandLine, rawcmdlen);
                                    if (CreateProcessW(NULL, cmdstr, NULL, NULL, FALSE,
                                                       CREATE_NEW_CONSOLE | EXTENDED_STARTUPINFO_PRESENT,
                                                       NULL, NULL, &siex.StartupInfo, &pi))
                                    {
                                        result = TRUE;
                                        CloseHandle(pi.hProcess);
                                        CloseHandle(pi.hThread);
                                    }
                                    free(cmdstr);
                                }
                            }
                        }
                        free(p);
                    }
                }
                CloseHandle(process);
            }
        }
    }
    return result;
}
