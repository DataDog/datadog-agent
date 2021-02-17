#include "stdafx.h"
#include <cstdio>

HMODULE hDllModule = NULL;

void __cdecl WcaLog(__in LOGLEVEL llv, __in_z __format_string PCSTR fmt, ...)
{
    static char szBuf[1024];
    va_list args;
    va_start(args, fmt);
    vprintf(fmt, args);
    wprintf(L"\n");
    va_end(args);
}
