// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include "uac.h"

// Attempts to drop privileges from an elevated process by creating a new process and
// setting the parent process to be the user's explorer.exe. This causes the new process to
// inherit its access token from explorer.exe.
//
// Technique relies on having permission to open explorer.exe with PROCESS_CREATE_PROCESS,
// this access is verified against the explorer.exe process DACL.
// Generally,
//   If the current process was elevated via a consent prompt, the user account is the same and access will be granted.
//   If the current process was elevated via a credential prompt, the user account is different and access will be denied.
// https://learn.microsoft.com/en-us/windows/security/identity-protection/user-account-control/how-user-account-control-works
//
// TODO: Try to enable SeDebugPrivilege. This will allow this function to support credential prompts
//       if group policy has not been modified to remove SeDebugPrivilege from Administrators.
BOOL LaunchUnelevated(LPCWSTR CommandLine)
{
    BOOL result = FALSE;
    // Get handle to the Shell's desktop window
    // https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getshellwindow
    HWND hwnd = GetShellWindow();

    if (hwnd != NULL)
    {
        DWORD pid;
        // Get pid that created the window, this should be PID of explorer.exe
        if (GetWindowThreadProcessId(hwnd, &pid) != 0)
        {
            HANDLE process = OpenProcess(PROCESS_CREATE_PROCESS, FALSE, pid);

            if (process != NULL)
            {
                // To set the parent process, create a thread attribute list containing PROC_THREAD_ATTRIBUTE_PARENT_PROCESS
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
                        DeleteProcThreadAttributeList(p);
                        free(p);
                    }
                }
                CloseHandle(process);
            }
        }
    }
    return result;
}
