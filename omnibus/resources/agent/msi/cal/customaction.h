#pragma once
bool AddPrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
bool RemovePrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
int EnableServiceForUser(const std::wstring& service, const std::wstring& user);
bool InitLsaString(
	PLSA_UNICODE_STRING pLsaString,
	LPCWSTR pwszString);

PSID GetSidForUser(LPCWSTR host, LPCWSTR user);
DWORD addDdUserPermsToFile(std::wstring filename);
LSA_HANDLE GetPolicyHandle();
int CreateSecretUser(MSIHANDLE hInstall, std::wstring& name, std::wstring& comment);
int CreateDDUser(MSIHANDLE hInstall);
DWORD DeleteUser(std::wstring& name);
DWORD DeleteSecretsRegKey();
DWORD changeRegistryAcls(const wchar_t* name);
VOID  DoStopSvc(MSIHANDLE hInstall, std::wstring svcName);

// rights we might be interested in
/*
#define SE_INTERACTIVE_LOGON_NAME           TEXT("SeInteractiveLogonRight")
#define SE_NETWORK_LOGON_NAME               TEXT("SeNetworkLogonRight")
#define SE_BATCH_LOGON_NAME                 TEXT("SeBatchLogonRight")
#define SE_SERVICE_LOGON_NAME               TEXT("SeServiceLogonRight")
#define SE_DENY_INTERACTIVE_LOGON_NAME      TEXT("SeDenyInteractiveLogonRight")
#define SE_DENY_NETWORK_LOGON_NAME          TEXT("SeDenyNetworkLogonRight")
#define SE_DENY_BATCH_LOGON_NAME            TEXT("SeDenyBatchLogonRight")
#define SE_DENY_SERVICE_LOGON_NAME          TEXT("SeDenyServiceLogonRight")
#if (_WIN32_WINNT >= 0x0501)
#define SE_REMOTE_INTERACTIVE_LOGON_NAME    TEXT("SeRemoteInteractiveLogonRight")
#define SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME TEXT("SeDenyRemoteInteractiveLogonRight")
#endif
*/