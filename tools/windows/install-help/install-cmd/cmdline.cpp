#include "stdafx.h"
const wchar_t *opts[] = {L"-bindir", L"-confdir",    L"-uname",      L"-password",   L"-api_key",
                         L"-site",   L"-proxy_host", L"-proxy_port", L"-proxy_user", L"-proxy_password"};

const wchar_t *calargs[] = {L"PROJECTLOCATION",  L"APPLICATIONDATADIRECTORY",
                            L"DDAGENTUSER_NAME", L"DDAGENTUSER_PASSWORD",
                            L"APIKEY",           L"SITE",
                            L"PROXY_HOST",       L"PROXY_PORT",
                            L"PROXY_USER",       L"PROXY_PASSWORD"};

const wchar_t *defaults[] = {L"C:\\Program Files\\Datadog\\Datadog Agent\\",
                             L"C:\\ProgramData\\Datadog\\",
                             L"",
                             L"",
                             L"",
                             L"",
                             L"",
                             L"",
                             L"",
                             L""};

typedef enum _cmdargs
{
    ARG_BINDIR = 0,
    ARG_CONFDIR,
    ARG_USERNAME,
    ARG_PASSWORD,
    ARG_APIKEY,
    ARG_SITE,
    ARG_PROXY_HOST,
    ARG_PROXY_PORT,
    ARG_PROXY_USER,
    ARG_PROXY_PASSWORD,
    ARG_LAST
} CMDARGS;

CMDARGS operator++(CMDARGS &r, int)
{
    r = (CMDARGS)((int)r + 1);
    return r;
}

void usage()
{
    wprintf(L"Usage: install-cmd [-bindir <path>] [-confdir <path>] [-uname <username>] [-password <password>]\n\n");
    return;
}
bool parseArgs(int argc, wchar_t **argv, std::wstring &calstring)
{
    std::map<CMDARGS, bool> suppliedArgs;
    // all the args take params, so we better have an even number
    if (argc % 2 != 0)
    {
        usage();
        return false;
    }
    for (int i = 0; i < argc - 1; i++)
    {
        bool bFound = false;
        for (CMDARGS a = ARG_BINDIR; a < ARG_LAST; a++)
        {
            if (_wcsicmp(argv[i], opts[(int)a]) == 0)
            {
                bFound = true;
                i++;
                suppliedArgs[a] = true;
                calstring += calargs[(int)a];
                calstring += L"=";
                calstring += argv[i];
                calstring += L"\n";
                break;
            }
        }
        if (!bFound)
        {
            usage();
            return false;
        }
    }
    for (CMDARGS a = ARG_BINDIR; a < ARG_LAST; a++)
    {
        if (suppliedArgs.find(a) == suppliedArgs.end())
        {
            // didn't supply it; add the empty string on
            calstring += calargs[(int)a];
            calstring += L"=";
            calstring += defaults[(int)a];
            calstring += L"\n";
        }
    }
    return true;
}
