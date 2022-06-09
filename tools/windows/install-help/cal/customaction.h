#pragma once
/*
 * parameters that define the password generating algorithm
 *
 */
#define MIN_PASS_LEN 16         /* minimum length of password to generate */
#define MAX_PASS_LEN 20         /* maximum length of password to generate */
#define MIN_NUM_LOWER_CHARS 2   /* minimum allowable number of lowercase chars */
#define MIN_NUM_UPPER_CHARS 2   /* minimum allowable number of uppercase chars */
#define MIN_NUM_NUMBER_CHARS 2  /* minimum allowable number of numeric chars */
#define MIN_NUM_SPECIAL_CHARS 2 /* minimum number of special characters */

class SecurityIdentifier;
class CustomActionData;

// usercreate.cpp
bool generatePassword(wchar_t *passbuf, int passbuflen);
int doCreateUser(const std::wstring &name, const std::wstring &comment, const wchar_t *passbuf);
int doSetUserPassword(const std::wstring &name, const wchar_t *passbuf);
DWORD changeRegistryAcls(SecurityIdentifier const &sid, const wchar_t *name);
DWORD addDdUserPermsToFile(SecurityIdentifier const &sid, std::wstring &filename);

void removeUserPermsFromFile(std::wstring &filename, SecurityIdentifier const &sid);

DWORD DeleteUser(const wchar_t *host, const wchar_t *name);

bool AddPrivileges(SecurityIdentifier const &sid, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
bool RemovePrivileges(SecurityIdentifier const &sid, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
int EnableServiceForUser(SecurityIdentifier const &sid, const std::wstring &service);
DWORD AddUserToGroup(SecurityIdentifier const &sid, wchar_t *groupSidString, wchar_t *defaultGroupName);
DWORD DelUserFromGroup(SecurityIdentifier const &sid, wchar_t *groupSidString, wchar_t *defaultGroupName);
bool InitLsaString(PLSA_UNICODE_STRING pLsaString, LPCWSTR pwszString);

LSA_HANDLE GetPolicyHandle();

// stopservices.cpp
VOID DoStopAllServices();
DWORD DoStartSvc(std::wstring &svcName);
int doesServiceExist(std::wstring &svcName);
int installServices(CustomActionData &data, SecurityIdentifier const &sid, const wchar_t *password);
int uninstallServices(CustomActionData &data);
int verifyServices(CustomActionData &data);

// delfiles.cpp
BOOL DeleteFilesInDirectory(const wchar_t *dirname, const wchar_t *ext, bool dirs = false);
BOOL DeleteHomeDirectory(const std::wstring &user, SecurityIdentifier const &sid);

// caninstall.cpp
bool canInstall(BOOL isDC, int ddUserExists, int ddServiceExists, const CustomActionData &data, bool &bResetPassword);
extern HMODULE hDllModule;
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

// FinalizeInstall.cpp
UINT doFinalizeInstall(CustomActionData &data);

// doUninstall.cpp
typedef enum _uninstall_type
{
    UNINSTALL_UNINSTALL,
    UNINSTALL_ROLLBACK
} UNINSTALL_TYPE;

UINT doUninstallAs(UNINSTALL_TYPE t);
