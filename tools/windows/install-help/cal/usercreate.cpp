#include "stdafx.h"
#pragma comment(lib, "shlwapi.lib")

bool generatePassword(wchar_t *passbuf, int passbuflen)
{
    if (passbuflen < MAX_PASS_LEN + 1)
    {
        return false;
    }
#define RANDOM_BUFFER_SIZE 128
    unsigned char randbuf[RANDOM_BUFFER_SIZE];
    const wchar_t *availLower = L"abcdefghijklmnopqrstuvwxyz";
    const wchar_t *availUpper = L"ABCDEFGHIJKLMNOPQRSTUVWXYZ";
    const wchar_t *availNum = L"1234567890";
    const wchar_t *availSpec = L"()`~!@#$%^&*-+=|{}[]:;'<>,.?/";

#define CHARTYPE_LOWER 0
#define CHARTYPE_UPPER 1
#define CHARTYPE_NUMBER 2
#define CHARTYPE_SPECIAL 3
    const wchar_t *classes[] = {
        availLower,
        availUpper,
        availNum,
        availSpec,
    };
    size_t classlengths[] = {wcslen(availLower), wcslen(availUpper), wcslen(availNum), wcslen(availSpec)};
    int numtypes = sizeof(classes) / sizeof(wchar_t *);

    int usedClasses[] = {0, 0, 0, 0};

    NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
    if (0 != ret)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
        return false;
    }
    // we'll do a random length password between 12 and 18 chars
    int len = (randbuf[0] % (MAX_PASS_LEN - MIN_PASS_LEN)) + MIN_PASS_LEN;
    int times = 0;

    do
    {
        int randbufindex = 0;
        memset(usedClasses, 0, sizeof(usedClasses));
        memset(passbuf, 0, sizeof(wchar_t) * (MAX_PASS_LEN + 1));
        NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
        if (0 != ret)
        {
            WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
            return false;
        }

        for (int i = 0; i < len && randbufindex < RANDOM_BUFFER_SIZE - 2; i++)
        {
            int chartype = randbuf[randbufindex++] % numtypes;

            int max_ndx = int(classlengths[chartype] - 1);
            int ndx = randbuf[randbufindex++] % max_ndx;

            passbuf[i] = classes[chartype][ndx];
            usedClasses[chartype]++;
        }
        times++;
    } while ((usedClasses[CHARTYPE_LOWER] < MIN_NUM_LOWER_CHARS || usedClasses[CHARTYPE_UPPER] < MIN_NUM_UPPER_CHARS ||
              usedClasses[CHARTYPE_NUMBER] < MIN_NUM_NUMBER_CHARS ||
              usedClasses[CHARTYPE_SPECIAL] < MIN_NUM_SPECIAL_CHARS) ||
             ((usedClasses[CHARTYPE_LOWER] + usedClasses[CHARTYPE_UPPER]) <
              (usedClasses[CHARTYPE_NUMBER] + usedClasses[CHARTYPE_SPECIAL])));

    WcaLog(LOGMSG_STANDARD, "Took %d passes to generate the password", times);
    return true;
}
DWORD changeRegistryAcls(PSID sid, const wchar_t *name)
{

    WcaLog(LOGMSG_STANDARD, "Changing registry ACL on %S", name);
    ExplicitAccess localsystem;
    localsystem.BuildGrantSid(TRUSTEE_IS_USER, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_LOCAL_SYSTEM_RID, 0);

    ExplicitAccess localAdmins;
    localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID,
                              DOMAIN_ALIAS_RID_ADMINS);

    // ExplicitAccess suser;
    // suser.BuildGrantUser(secretUserUsername.c_str(), GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ);

    ExplicitAccess dduser;
    dduser.BuildGrantUser((SID *)sid, GENERIC_ALL | KEY_ALL_ACCESS, SUB_CONTAINERS_AND_OBJECTS_INHERIT);

    WinAcl acl;
    acl.AddToArray(localsystem);
    // acl.AddToArray(suser);
    acl.AddToArray(localAdmins);
    acl.AddToArray(dduser);

    PACL newAcl = NULL;
    PACL oldAcl = NULL;
    DWORD ret = 0;
    // only want to set new acl info
    oldAcl = NULL;
    ret = acl.SetEntriesInAclW(oldAcl, &newAcl);

    ret = SetNamedSecurityInfoW((LPWSTR)name, SE_REGISTRY_KEY,
                                DACL_SECURITY_INFORMATION, // | PROTECTED_DACL_SECURITY_INFORMATION,
                                NULL, NULL, newAcl, NULL);

    if (0 != ret)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to set named security info %d", ret);
    }
    return ret;
}

DWORD addDdUserPermsToFile(PSID sid, std::wstring &filename)
{

    if (!PathFileExistsW((LPCWSTR)filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file %S doesn't exist, not doing anything", filename.c_str());
        return 0;
    }
    WcaLog(LOGMSG_STANDARD, "Changing file permissions on %S", filename.c_str());
    ExplicitAccess dduser;
    dduser.BuildGrantUser((SID *)sid, FILE_ALL_ACCESS, SUB_CONTAINERS_AND_OBJECTS_INHERIT);

    // get the current ACLs and append, rather than just set; if the file exists,
    // the user may have already set custom ACLs on the file, and we don't want
    // to disrupt that

    DWORD dwRes = 0;
    PACL pOldDACL = NULL, pNewDACL = NULL;
    PSECURITY_DESCRIPTOR pSD = NULL;
    WinAcl acl;
    acl.AddToArray(dduser);

    dwRes = GetNamedSecurityInfo(filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION, NULL, NULL, &pOldDACL,
                                 NULL, &pSD);
    if (ERROR_SUCCESS == dwRes)
    {
        dwRes = acl.SetEntriesInAclW(pOldDACL, &pNewDACL);
        if (dwRes == 0)
        {
            dwRes = SetNamedSecurityInfoW((LPWSTR)filename.c_str(), SE_FILE_OBJECT,
                                          DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION, NULL, NULL,
                                          pNewDACL, NULL);
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "%d setting entries in acl", dwRes);
        }
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "%d getting existing perms", dwRes);
    }
    if (pSD)
    {
        LocalFree((HLOCAL)pSD);
    }
    if (pNewDACL)
    {
        LocalFree((HLOCAL)pNewDACL);
    }
    return dwRes;
}

void removeUserPermsFromFile(std::wstring &filename, PSID sidremove)
{
    if (!PathFileExistsW((LPCWSTR)filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file doesn't exist, not doing anything");
        return;
    }
    ExplicitAccess dduser;
    // get the current ACLs;  check to see if the DD user is in there, if so
    // remove
    DWORD dwRes = 0;
    PACL pOldDacl = NULL;
    PSECURITY_DESCRIPTOR pSD = NULL;
    ACL_SIZE_INFORMATION sizeInfo;
    memset(&sizeInfo, 0, sizeof(ACL_SIZE_INFORMATION));

    dwRes = GetNamedSecurityInfo(filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION, NULL, NULL, &pOldDacl,
                                 NULL, &pSD);
    if (ERROR_SUCCESS != dwRes)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to get file DACL, not removing user perms");
        return;
    }
    BOOL bRet = GetAclInformation(pOldDacl, (PVOID)&sizeInfo, sizeof(ACL_SIZE_INFORMATION), AclSizeInformation);
    if (FALSE == bRet)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to get DACL size information");
        goto doneRemove;
    }
    for (DWORD i = 0; i < sizeInfo.AceCount; i++)
    {
        ACCESS_ALLOWED_ACE *ace;

        if (GetAce(pOldDacl, i, (LPVOID *)&ace))
        {
            PSID compareSid = (PSID)(&ace->SidStart);
            if (EqualSid(compareSid, sidremove))
            {
                WcaLog(LOGMSG_STANDARD, "Matched sid on file %S, removing", filename.c_str());
                if (!DeleteAce(pOldDacl, i))
                {
                    WcaLog(LOGMSG_STANDARD, "Failed to delete ACE on file %S", filename.c_str());
                }
            }
        }
    }
    dwRes = SetNamedSecurityInfoW((LPWSTR)filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION, NULL, NULL,
                                  pOldDacl, NULL);
    if (dwRes != 0)
    {
        WcaLog(LOGMSG_STANDARD, "%d resetting permissions on %S", dwRes, filename.c_str());
    }

doneRemove:

    if (pSD)
    {
        LocalFree((HLOCAL)pSD);
    }

    return;
}

int doCreateUser(const std::wstring &name, const std::wstring &comment, const wchar_t *passbuf)
{

    USER_INFO_1 ui;
    memset(&ui, 0, sizeof(USER_INFO_1));
    ui.usri1_name = (LPWSTR)name.c_str();
    ui.usri1_password = (LPWSTR)passbuf;
    ui.usri1_priv = USER_PRIV_USER;
    ui.usri1_comment = (LPWSTR)comment.c_str();
    ui.usri1_flags = UF_DONT_EXPIRE_PASSWD;

    WcaLog(LOGMSG_STANDARD, "Adding user %S", name.c_str());
    DWORD ret = NetUserAdd(nullptr, // LOCAL_MACHINE
                           1,       // indicates we're using a USER_INFO_1
                           reinterpret_cast<LPBYTE>(&ui), nullptr);
    /*
     * If the function fails, the return value can be one of the following error codes.
     *   - ERROR_ACCESS_DENIED
     *   The user does not have access to the requested information.
     *   - NERR_InvalidComputer
     *   The computer name is invalid.
     *   - NERR_NotPrimary
     *   The operation is allowed only on the primary domain controller of the domain.
     *   - NERR_GroupExists
     *   The group already exists.
     *   - NERR_UserExists
     *   The user account already exists.
     *   - NERR_PasswordTooShort
     *   The password is shorter than required. (The password could also be too long, be too recent in its change
     * history, not have enough unique characters, or not meet another password policy requirement.)
     */
    if (ret == NERR_Success)
    {
        WcaLog(LOGMSG_STANDARD, "Successfully added user.");
    }
    else if (ret == NERR_UserExists)
    {
        WcaLog(LOGMSG_STANDARD, "Warning: the user already exists.");
        ret = 0;
    }
    else
    {
        const auto lmErrIt = lmerrors.find(ret - NERR_BASE);
        if (lmErrIt != lmerrors.end())
        {
            WcaLog(LOGMSG_STANDARD, "NetUserAdd: %d = %S", ret, lmErrIt->second.c_str());
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "NetUserAdd: %d", ret);
        }
    }
    return ret;
}

int doSetUserPassword(const std::wstring &name, const wchar_t *passbuf)
{
    USER_INFO_1003 ui;
    memset(&ui, 0, sizeof(USER_INFO_1003));
    ui.usri1003_password = (LPWSTR)passbuf;
    DWORD ret = NetUserSetInfo(NULL, name.c_str(), 1003, (LPBYTE)&ui, NULL);
    WcaLog(LOGMSG_STANDARD, "NetUserSetInfo Change Password %d", ret);
    return ret;
}
DWORD DeleteUser(const wchar_t *host, const wchar_t *name)
{
    NET_API_STATUS ret = NetUserDel(NULL, name);
    return (DWORD)ret;
}
