#include "stdafx.h"

std::wstring datadog_path = L"Datadog\\Datadog Agent";
std::wstring datadog_key_root = L"SOFTWARE\\" + datadog_path;
std::wstring datadog_acl_key_datadog = L"MACHINE\\SOFTWARE\\" + datadog_path;
std::wstring installStepsKey = datadog_key_root + L"\\installSteps";
std::wstring datadog_service_name(L"DataDog Agent");

std::wstring ddAgentUserName(L"ddagentuser");
std::wstring ddAgentUserPasswordProperty(L"DDAGENTUSER_PASSWORD");
std::wstring ddAgentUserDescription(L"User context under which the DataDog Agent service runs");

std::wstring traceService(L"datadog-trace-agent");
std::wstring processService(L"datadog-process-agent");
std::wstring agentService(L"datadogagent");

std::wstring propertyDDUserCreated(L"DDUSERCREATED");
std::wstring propertyRollbackState(L"CustomActionData");

std::wstring programdataroot(L"c:\\ProgramData\\DataDog\\");
std::wstring logfilename(L"c:\\ProgramData\\DataDog\\logs\\agent.log");
std::wstring authtokenfilename(L"c:\\ProgramData\\Datadog\\auth_token");
std::wstring datadogyamlfile(L"c:\\ProgramData\\Datadog\\datadog.yaml");
std::wstring confddir(L"c:\\ProgramData\\Datadog\\conf.d");
std::wstring logdir(L"c:\\ProgramData\\Datadog\\logs");

// installation steps
std::wstring strDdUserCreated(L"00-ddUserCreated");
std::wstring strDdUserPasswordChanged(L"01-ddUserPasswordChanged");
std::wstring strFilePermissionsChanged(L"02-ddUserFilePermsChanged");
std::wstring strAddDdUserToPerfmon(L"03-ddUserAddedToPerfmon");
std::wstring strChangedRegistryPermissions(L"04-ddRegPermsChanged");

void toMbcs(std::string& target, LPCWSTR src) {
    size_t len = wcslen(src);
    size_t narrowlen = (2 * len) + 1;
    char * tgt = new char[narrowlen];
    wcstombs(tgt, src, narrowlen);
    target = tgt;
    delete[] tgt;
    return;
}
