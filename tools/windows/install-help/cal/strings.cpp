#include "stdafx.h"


std::wstring datadog_path ;                             //IDS_DATADOG_PATH
std::wstring datadog_key_root_base;                     // IDS_DATADOG_KEY_ROOT_BASE
std::wstring datadog_acl_key_datadog_base;              // IDS_DATADOG_ACL_KEY_DATADOG_BASE

std::wstring datadog_service_name;                      //IDS_DATADOG_SERVICE_NAME

std::wstring ddAgentUserName;                           // IDS_DATADOG_AGENT_USER_NAME

std::wstring ddAgentUserDescription;                    // IDS_DATADOG_AGENT_USER_DESCRIPTION

std::wstring traceService;                              // IDS_TRACE_SERVICE_NAME
std::wstring processService;                            // IDS_PROCESS_SERVICE_NAME
std::wstring agentService;                              // IDS_AGENT_SERVICE_NAME

std::wstring propertyDDAgentUserName;                   // IDS_PROPERTY_DDAGENTUSER
std::wstring propertyDDAgentUserPassword;               // IDS_PROPERTY_DDAGENTUSER_PASSWORD
std::wstring propertyAppDataDir;                        // IDS_PROPERTY_PROGRAMDATA
std::wstring propertyProgramFilesDir;                   // IDS_PROPERTY_PROGRAMFILESDIR

//std::wstring propertyEnableServicesDeferredKey(L"enableservices");
//std::wstring propertyRollbackState(L"CustomActionData");
std::wstring logsSuffix;                        // IDS_LOGSSUFFIX
std::wstring authTokenSuffix;                   // IDS_AUTHTOKENSUFFIX
std::wstring datadogyaml;                       // IDS_DATADOGYAML
std::wstring confdsuffix;                       // IDS_CONFSDSUFFIX
std::wstring logsdirsuffix;                     // IDS_LOGSDIRSUFFIX
std::wstring datadogdir;

std::wstring strRollbackKeyName;                // IDS_REGKEY_ROLLBACK_KEY_NAME
std::wstring strUninstallKeyName;               // IDS_REGKEY_UNINSTALL_KEY_NAME

std::wstring programdataroot;
std::wstring logfilename;
std::wstring authtokenfilename;
std::wstring datadogyamlfile;
std::wstring confddir;
std::wstring logdir;
std::wstring installdir;
std::wstring embedded2Dir;
std::wstring embedded3Dir;

std::wstring propertyCustomActionData(L"CustomActionData");
std::wstring datadog_key_root;
std::wstring datadog_acl_key_datadog;

std::wstring agent_exe;
std::wstring trace_exe;
std::wstring process_exe;

std::wstring computername;
std::wstring domainname; // if domain joined, workgroup name otherwise
bool isDomainJoined = false;


std::wstring* loadStrings[] = {
    &datadog_path,
    &datadog_key_root_base,
    &datadog_acl_key_datadog_base,
    &datadog_key_root,
    &datadog_service_name,
    &ddAgentUserName,
    &ddAgentUserDescription,
    &traceService,
    &processService,
    &agentService,
    &propertyDDAgentUserName,
    &propertyDDAgentUserPassword,
    &propertyAppDataDir,
    &propertyProgramFilesDir,
    &logsSuffix,
    &authTokenSuffix,
    &datadogyaml,
    &confdsuffix,
    &logsdirsuffix,
    &datadogdir,
    &strRollbackKeyName,
    &strUninstallKeyName
};

// strings for tracking install state
std::wstring installCreatedDDUser;
std::wstring installCreatedDDDomain;
std::wstring installInstalledServices;
std::wstring *installStrings[] = {
    &installCreatedDDUser,
    &installCreatedDDDomain,
    &installInstalledServices,
};
void loadStringToWstring(int id, std::wstring *target)
{
#define DEFAULT_BUFFER_SIZE 512
    wchar_t defaultbuffer[DEFAULT_BUFFER_SIZE];
    memset(defaultbuffer, 0, DEFAULT_BUFFER_SIZE * sizeof(wchar_t));
    int nRc = LoadStringW(hDllModule, id, defaultbuffer, DEFAULT_BUFFER_SIZE);

    if (nRc == 0) {
        // string isn't present
        return;
    }
    if (nRc < DEFAULT_BUFFER_SIZE - 1) {
        // it fit in the buffer, just return it
        *target = defaultbuffer;
        return;
    }
    // ideally, we'll never get here.  The LoadString API is lame, and doesn't
    // tell you how big a buffer you need.  So, keep trying until we don't use
    // the whole buffer

    nRc = DEFAULT_BUFFER_SIZE * 2; // initialize to get past the initial comparison in the for
    for (int bufsz = DEFAULT_BUFFER_SIZE * 2; nRc >= (bufsz - 1); bufsz += DEFAULT_BUFFER_SIZE)
    {
        wchar_t * tgtbuffer = new wchar_t[bufsz];
        memset(tgtbuffer, 0, bufsz * sizeof(wchar_t));
        nRc = LoadStringW(hDllModule, id, tgtbuffer, bufsz);
        if (nRc < bufsz - 1) {
            *target = tgtbuffer;
        }
        delete[] tgtbuffer;
    }
}
static bool initialized = false;



bool wrapGetComputerNameExW(COMPUTER_NAME_FORMAT fmt, std::wstring& result)
{
    wchar_t * buffer = NULL;
    DWORD sz = 0;
    BOOL res = GetComputerNameExW(fmt, buffer, &sz);
    if(res) {
        // this should never succeed
        WcaLog(LOGMSG_STANDARD, "Unexpected.  Didn't get buffer size for computer name %d", (int)fmt);
        return false;
    }
    DWORD err = GetLastError();
    if(ERROR_MORE_DATA != err){
        WcaLog(LOGMSG_STANDARD, "Unable to get computername info %d", err);
        return false;
    }
    buffer = new wchar_t[sz + 1];
    sz = sz + 1;
    res = GetComputerNameExW(fmt, buffer, &sz);
    if(res){
        result = _wcslwr_s(buffer, sz + 1);
    } else {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Unable to get computername info %d", err);
    }
    if(buffer){
        delete [] buffer;
    }
    return res;
}
void getHostInformation() {
    wchar_t buf[MAX_COMPUTERNAME_LENGTH + 1];
    DWORD sz = MAX_COMPUTERNAME_LENGTH+1;
    if(!GetComputerNameW(buf, &sz)){
        WcaLog(LOGMSG_STANDARD, "Failed to get computername %d", GetLastError());
    } else {
        computername = _wcslwr_s(buf, MAX_COMPUTERNAME_LENGTH + 1);
        WcaLog(LOGMSG_STANDARD, "Computername is %S (%d)", computername.c_str(), sz);
    }
    // get the computername again and compare, just to make sure
    std::wstring compare_computer;
    if(wrapGetComputerNameExW(ComputerNameDnsHostname, compare_computer)) {
        if(computername != compare_computer){
            WcaLog(LOGMSG_STANDARD, "Got two different computer names %S %S", computername.c_str(), compare_computer.c_str());
        }
    }
    std::wstring domain; 
    if(wrapGetComputerNameExW(ComputerNameDnsDomain, domain))
    {
        // newer domains will look like DNS domains.  (i.e. domain.local)
        // just take the domain portion, which is all we're interested in.
        size_t pos = domain.find(L'.');
        if(pos != std::wstring::npos){
            domain = domain.substr(0, pos);
        }
    }

    // check if it's actually domain joined or not
    std::wstring joined_domain;
    LPWSTR name = NULL;
    NETSETUP_JOIN_STATUS st;
    int nErr = NetGetJoinInformation(NULL, &name, &st);
    if(nErr == NERR_Success) {
        joined_domain = _wcslwr_s(name, wcslen(name) + 1);
        NetApiBufferFree(name);
    } else {
        WcaLog(LOGMSG_STANDARD, "Error getting domain joining information %d", GetLastError());
    }
    switch(st){
        case NetSetupUnknownStatus:
            WcaLog(LOGMSG_STANDARD, "Unknown domain joining status, assuming not joined");
            isDomainJoined = false;
            break;
        case NetSetupUnjoined:
            WcaLog(LOGMSG_STANDARD, "Computer explicitly not joined to domain");
            isDomainJoined = false;
            break;
        case NetSetupWorkgroupName:
            WcaLog(LOGMSG_STANDARD, "Computer is joined to a workgroup");
            isDomainJoined = false;
            break;
        case NetSetupDomainName:
            WcaLog(LOGMSG_STANDARD, "Computer is domain-joined");
            isDomainJoined = true;
            break;
    }
    if(isDomainJoined) {
        if(domain != joined_domain) {
            WcaLog(LOGMSG_STANDARD, "Computed domains don't match (%S %S)", domain.c_str(), joined_domain.c_str());
        }
        domainname = domain;
    }
}

void getOsStrings()
{
    PWSTR outstr = NULL;
    // build up all the path-based strings
    std::wstring programfiles;

    ddRegKey ddroot;
    std::wstring confroot;
    if(!ddroot.getStringValue(L"ConfigRoot", programdataroot))
    {
        if(SHGetKnownFolderPath(FOLDERID_ProgramData, 0, 0, &outstr) == S_OK)
        {
            programdataroot = outstr;
            programdataroot += datadogdir;
        }
        if(programdataroot.back() != L'\\'){
            programdataroot += L"\\";
        }
    }
    if(!ddroot.getStringValue(L"InstallPath", installdir))
    {
        if(SHGetKnownFolderPath(FOLDERID_ProgramFiles, 0, 0, &outstr) == S_OK)
        {
            programfiles = outstr;
            installdir = programfiles + datadogdir;
        }
        if(installdir.back() != L'\\'){
            installdir += L"\\";
        }
    }
    logfilename = programdataroot + logsSuffix;
    authtokenfilename = programdataroot + authTokenSuffix;
    datadogyamlfile = programdataroot + datadogyaml;
    confddir = programdataroot + confdsuffix;
    logdir = programdataroot + logsdirsuffix;

    agent_exe = L"\"" + installdir + L"bin\\agent.exe\"";
    process_exe = L"\"" + installdir + L"bin\\agent\\process-agent.exe\" --config=\"" + programdataroot + L"datadog.yaml\"" ;
    trace_exe   = L"\"" + installdir + L"bin\\agent\\trace-agent.exe\" --config=\"" + programdataroot + L"datadog.yaml\"" ;
    embedded2Dir = installdir + L"embedded2";
    embedded3Dir = installdir + L"embedded3";
    datadog_acl_key_datadog = datadog_acl_key_datadog_base + datadog_path;

    getHostInformation();

}
void initializeStringsFromStringTable()
{
    if (initialized) {
        return;
    }
    for (int i = 0; i < sizeof(loadStrings) / sizeof(std::wstring*); i++)
    {
        loadStringToWstring(STRINGTABLE_BASE + i, loadStrings[i]);
    }
    for (int i = 0; i < sizeof(installStrings) / sizeof(std::wstring*); i++) {
        loadStringToWstring(INSTALLTABLE_BASE + i, installStrings[i]);
    }
    getOsStrings();
    initialized = true;
}

bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, std::wstring& dststr)
{
    wchar_t *dst = NULL;
    DWORD len = 0;
    if(loadPropertyString(hInstall, propertyName, &dst, &len)) {
        dststr = dst;
        delete [] dst;
        return true;
    }
    return false;
}

bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, wchar_t **dst, DWORD *len)
{
    TCHAR* szValueBuf = NULL;
    DWORD cchValueBuf = 0;

    UINT uiStat =  MsiGetProperty(hInstall, propertyName, L"", &cchValueBuf);
    //cchValueBuf now contains the size of the property's string, without null termination
    if (ERROR_MORE_DATA == uiStat)
    {
        ++cchValueBuf; // add 1 for null termination
        szValueBuf = new wchar_t[cchValueBuf];
        if (szValueBuf)
        {
            uiStat = MsiGetProperty(hInstall, propertyName, szValueBuf, &cchValueBuf);
        }
    }
    if (ERROR_SUCCESS != uiStat)
    {
        if (szValueBuf != NULL) 
           delete[] szValueBuf;
        WcaLog(LOGMSG_STANDARD, "failed to get  property");
        return false;
    }
    if (wcslen(szValueBuf) == 0){
        WcaLog(LOGMSG_STANDARD, "Property %S is empty", propertyName);
        delete [] szValueBuf;
        return false;
    }
    *dst=szValueBuf;
    *len = cchValueBuf;
    return true;
}


bool loadDdAgentPassword(MSIHANDLE hInstall, wchar_t **pass, DWORD *len) {
    return loadPropertyString(hInstall, propertyDDAgentUserPassword.c_str(), pass, len);
}


