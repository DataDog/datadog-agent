#include "stdafx.h"

std::wstring secretUserUsername(L"datadog_secretuser");
std::wstring secretUserDescription(L"DataDog user used to fetch secrets from KMS");
std::wstring datadog_path = L"Datadog\\Datadog Agent";
std::wstring datadog_key_root = L"SOFTWARE\\" + datadog_path;
std::wstring datadog_key_secret_key = L"secrets";
std::wstring datadog_key_secrets = L"SOFTWARE\\" + datadog_path + L"\\" + datadog_key_secret_key;
std::wstring datadog_acl_key_secrets = L"MACHINE\\" + datadog_key_secrets;
std::wstring datadog_acl_key_datadog = L"MACHINE\\SOFTWARE\\" + datadog_path;
std::wstring datadog_service_name(L"DataDog Agent");

std::wstring ddAgentUserName(L"ddagentuser");
std::wstring ddAgentUserPasswordProperty(L"DDAGENTUSER_PASSWORD");
std::wstring ddAgentUserDescription(L"User context under which the DataDog Agent service runs");

std::wstring traceService(L"datadog-trace-agent");
std::wstring processService(L"datadog-process-agent");

std::wstring propertyDDUserCreated(L"DDUSERCREATED");
std::wstring propertySecretUserCreated(L"SECRETUSERCREATED");
std::wstring propertySecretPasswordWritten(L"SECRETPASSWORDWRITTEN");
std::wstring propertyRollbackState(L"CustomActionData");

std::wstring logfilename(L"c:\\ProgramData\\DataDog\\logs\\agent.log");
std::wstring authtokenfilename(L"c:\\ProgramData\\Datadog\\auth_token");
std::wstring datadogyamlfile(L"c:\\ProgramData\\Datadog\\datadog.yaml");

void toMbcs(std::string& target, LPCWSTR src) {
    size_t len = wcslen(src);
    size_t narrowlen = (2 * len) + 1;
    char * tgt = new char[narrowlen];
    wcstombs(tgt, src, narrowlen);
    target = tgt;
    delete[] tgt;
    return;
}
