// uninstall-cmd.cpp : This file contains the 'main' function. Program execution begins and ends there.
//

#include "stdafx.h"
#include <iostream>
#define MAX_LOG_LINE 1024
HMODULE hDllModule = NULL;

void __cdecl WcaLog(__in LOGLEVEL llv, __in_z __format_string PCSTR fmt, ...)
{
    static char szBuf[MAX_LOG_LINE]; // note.  This makes this function NOT thread safe.
    va_list args;
    va_start(args, fmt);
    vprintf(fmt, args);
    wprintf(L"\n");
    va_end(args);
}

// replace this function with an error.  We should never be calling this during the
// console based invocation.
UINT WINAPI MsiGetPropertyW(MSIHANDLE hInstall,
                            LPCWSTR szName,                                    // property identifier, case-sensitive
                            _Out_writes_opt_(*pcchValueBuf) LPWSTR szValueBuf, // buffer for returned property value
                            _Inout_opt_ LPDWORD pcchValueBuf)                  // in/out buffer character count
{
    return ERROR_INVALID_FUNCTION;
}

int wmain(int argc, wchar_t **argv)
{
    hDllModule = GetModuleHandle(NULL);
    initializeStringsFromStringTable();
    std::wstring defaultData;
    parseArgs(argc - 1, &(argv[1]), defaultData);
    wprintf(L"%s\n", defaultData.c_str());

    doUninstallAs(UNINSTALL_UNINSTALL);
}

// Run program: Ctrl + F5 or Debug > Start Without Debugging menu
// Debug program: F5 or Debug > Start Debugging menu

// Tips for Getting Started:
//   1. Use the Solution Explorer window to add/manage files
//   2. Use the Team Explorer window to connect to source control
//   3. Use the Output window to see build output and other messages
//   4. Use the Error List window to view errors
//   5. Go to Project > Add New Item to create new code files, or Project > Add Existing Item to add existing code files
//   to the project
//   6. In the future, to open this project again, go to File > Open > Project and select the .sln file
