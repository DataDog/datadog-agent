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
#include "SID.h"

class CustomActionData;
// usercreate.cpp
bool generatePassword(wchar_t *passbuf, int passbuflen);
int doCreateUser(const std::wstring &name, const std::wstring &comment, const wchar_t *passbuf);
int doSetUserPassword(const std::wstring &name, const wchar_t *passbuf);
DWORD changeRegistryAcls(PSID sid, const wchar_t *name);
DWORD addDdUserPermsToFile(PSID sid, std::wstring &filename);

void removeUserPermsFromFile(std::wstring &filename, PSID sidremove);

DWORD DeleteUser(const wchar_t *host, const wchar_t *name);

bool AddPrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
bool RemovePrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd);
int EnableServiceForUser(PSID sid, const std::wstring &service);
DWORD AddUserToGroup(PSID userSid, wchar_t *groupSidString, wchar_t *defaultGroupName);
DWORD DelUserFromGroup(PSID userSid, wchar_t *groupSidString, wchar_t *defaultGroupName);
bool InitLsaString(PLSA_UNICODE_STRING pLsaString, LPCWSTR pwszString);

struct SidResult
{
    sid_ptr Sid;
    std::wstring Domain;
    DWORD Result;

    SidResult(DWORD result)
        : Result(result)
    {
    }

    SidResult(sid_ptr &sid, std::wstring const &domain, DWORD result)
        : Sid(std::move(sid))
        , Domain(domain)
        , Result(result)
    {
    }

    SidResult(SidResult const &) = delete;

    SidResult(SidResult &&other) noexcept
        : Sid(std::move(other.Sid))
        , Domain(other.Domain)
        , Result(other.Result)
    {
    }
};

/// <summary>
/// Retrives the Security Identifier Descriptor of the specified user.
/// </summary>
/// <param name="host">The host to search on.</param>
/// <param name="user">The username to look for.</param>
/// <returns>An <see cref="SidResult"/>.
/// If no user is found, the Sid field will be NULL and the Result field will contain the result of <see
/// cref="GetLastError">.</returns>
SidResult GetSidForUser(LPCWSTR host, LPCWSTR user);

bool GetNameForSid(LPCWSTR host, PSID sid, std::wstring &namestr);

LSA_HANDLE GetPolicyHandle();

// stopservices.cpp
VOID DoStopAllServices();
DWORD DoStartSvc(std::wstring &svcName);
int doesServiceExist(std::wstring &svcName);
int installServices(CustomActionData &data, PSID sid, const wchar_t *password);
int uninstallServices();
int verifyServices(CustomActionData &data);

// delfiles.cpp
BOOL DeleteFilesInDirectory(const wchar_t *dirname, const wchar_t *ext, bool dirs = false);
BOOL DeleteHomeDirectory(const std::wstring &user, PSID userSID);

// caninstall.cpp
bool canInstall(
    const CustomActionData &data,
    bool &bResetPassword,
    std::wstring *resultMessage);
bool canInstall(
    bool isDC,
    bool isReadOnlyDC,
    bool ddUserExists,
    bool isServiceAccount,
    bool isNtAuthority,
    bool isUserDomainUser,
    bool haveUserPassword,
    std::wstring userDomain,
    std::wstring computerDomain,
    bool ddServiceExists,
    bool &bResetPassword,
    std::wstring *resultMessage);
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

// see https://stackoverflow.com/a/45565001/425565
// Template ErrType can be HRESULT (long) or DWORD (unsigned long)
template<class ErrType> std::wstring GetErrorMessageStrW(ErrType errCode)
{
    const int buffsize = 4096;
    wchar_t buffer[buffsize];
    const DWORD len =
        FormatMessageW(FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS | FORMAT_MESSAGE_MAX_WIDTH_MASK,
                       nullptr, // (not used with FORMAT_MESSAGE_FROM_SYSTEM)
                       errCode, MAKELANGID(LANG_NEUTRAL, SUBLANG_DEFAULT), &buffer[0], buffsize, nullptr);
    if (len > 0)
    {
        return std::wstring(buffer, len);
    }
    std::wstringstream wsstr;
    wsstr << L"Failed to retrieve error message string for code " << errCode;
    return  wsstr.str();
}
