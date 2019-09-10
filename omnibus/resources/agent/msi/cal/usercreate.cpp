#include "stdafx.h"
#include <filesystem>
#include "unique_ptr_adapter.hpp"

#pragma comment(lib, "shlwapi.lib")

namespace dd
{
    template <>
    struct details::ptr_converter<PSECURITY_DESCRIPTOR, SECURITY_DESCRIPTOR*>
    {
        static SECURITY_DESCRIPTOR* convert(PSECURITY_DESCRIPTOR p)
        {
            return reinterpret_cast<SECURITY_DESCRIPTOR*>(p);
        }
    };

   /**
    * \brief Deleter that uses LocalFree
    */
    struct LocalFreeDeleter
    {
        // ReSharper disable once CppMemberFunctionMayBeConst
        void operator()(HLOCAL handle)
        {
            LocalFree(handle);
        }
    };

    template <>
    struct ptr_traits<ACL>
    {
        typedef std::unique_ptr<ACL, LocalFreeDeleter> unique_ptr;
        typedef unique_ptr_adapter<unique_ptr, PACL> store_ptr;
    };

    using acl_traits = ptr_traits<ACL>;

    template <>
    struct ptr_traits<SECURITY_DESCRIPTOR>
    {
        typedef std::unique_ptr<SECURITY_DESCRIPTOR, LocalFreeDeleter> unique_ptr;
        typedef unique_ptr_adapter<unique_ptr, PSECURITY_DESCRIPTOR> store_ptr;
    };

    using security_descriptor_traits = ptr_traits<SECURITY_DESCRIPTOR>;
}


bool generatePassword(wchar_t* passbuf, int passbuflen) {
    if (passbuflen < MAX_PASS_LEN + 1) {
        return false;
    }
#define RANDOM_BUFFER_SIZE 128
    unsigned char randbuf[RANDOM_BUFFER_SIZE];
    const wchar_t * availLower = L"abcdefghijklmnopqrstuvwxyz";
    const wchar_t * availUpper = L"ABCDEFGHIJKLMNOPQRSTUVWXYZ";
    const wchar_t * availNum = L"1234567890";
    const wchar_t * availSpec = L"()`~!@#$%^&*-+=|{}[]:;'<>,.?/";

#define CHARTYPE_LOWER 0
#define CHARTYPE_UPPER 1
#define CHARTYPE_NUMBER 2
#define CHARTYPE_SPECIAL 3
    const wchar_t * classes[] = {
        availLower,
        availUpper,
        availNum,
        availSpec,
    };
    size_t classlengths[] = {
        wcslen(availLower),
        wcslen(availUpper),
        wcslen(availNum),
        wcslen(availSpec)
    };
    int numtypes = sizeof(classes) / sizeof(wchar_t*);

    int usedClasses[] = { 0, 0, 0, 0 };

    NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
    if (0 != ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
        return false;
    }
    // we'll do a random length password between 12 and 18 chars
    int len = (randbuf[0] % (MAX_PASS_LEN - MIN_PASS_LEN)) + MIN_PASS_LEN;
    int times = 0;

    do {
        int randbufindex = 0;
        memset(usedClasses, 0, sizeof(usedClasses));
        memset(passbuf, 0, sizeof(wchar_t) * (MAX_PASS_LEN + 1));
        NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
        if (0 != ret) {
            WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
            return false;
        }

        for (int i = 0; i < len && randbufindex < RANDOM_BUFFER_SIZE - 2; i++) {
            int chartype = randbuf[randbufindex++] % numtypes;

            int max_ndx = int(classlengths[chartype] - 1);
            int ndx = randbuf[randbufindex++] % max_ndx;

            passbuf[i] = classes[chartype][ndx];
            usedClasses[chartype]++;
        }
        times++;
    } while ((usedClasses[CHARTYPE_LOWER] < 2 || usedClasses[CHARTYPE_UPPER] < 2 ||
        usedClasses[CHARTYPE_NUMBER] < 2 || usedClasses[CHARTYPE_SPECIAL] < 2) ||
        ((usedClasses[CHARTYPE_LOWER] + usedClasses[CHARTYPE_UPPER]) <
        (usedClasses[CHARTYPE_NUMBER] + usedClasses[CHARTYPE_SPECIAL])));

    WcaLog(LOGMSG_STANDARD, "Took %d passes to generate the password", times);
    return true;

}
DWORD changeRegistryAcls(CustomActionData& data, const wchar_t* name) {

    std::string namestr;
    toMbcs(namestr, name);
    WcaLog(LOGMSG_STANDARD, "Changing registry ACL on %s", namestr.c_str());
    ExplicitAccess localsystem;
    localsystem.BuildGrantSid(TRUSTEE_IS_USER, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_LOCAL_SYSTEM_RID, 0);

    ExplicitAccess localAdmins;
    localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS);

    //ExplicitAccess suser;
    //suser.BuildGrantUser(secretUserUsername.c_str(), GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ);

    PSID  usersid = GetSidForUser(NULL, data.getQualifiedUsername().c_str());
    ExplicitAccess dduser;
    dduser.BuildGrantUser((SID *)usersid, GENERIC_ALL | KEY_ALL_ACCESS,
        SUB_CONTAINERS_AND_OBJECTS_INHERIT);


    WinAcl acl;
    acl.AddToArray(localsystem);
    //acl.AddToArray(suser);
    acl.AddToArray(localAdmins);
    acl.AddToArray(dduser);


    PACL newAcl = NULL;
    PACL oldAcl = NULL;
    DWORD ret = 0;
    // only want to set new acl info
    oldAcl = NULL;
    ret = acl.SetEntriesInAclW(oldAcl, &newAcl);

    ret = SetNamedSecurityInfoW((LPWSTR)name, SE_REGISTRY_KEY, DACL_SECURITY_INFORMATION, // | PROTECTED_DACL_SECURITY_INFORMATION,
        NULL, NULL, newAcl, NULL);

    if (0 != ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to set named security info %d", ret);
    }
    return ret;

}

DWORD SetPermissionsOnFile(
    std::wstring const& userName,
    std::wstring const& filePath,
    std::vector<dd::Permission> const& permissions)
{
    if (!PathFileExists(filePath.c_str()))
    {
        return ERROR_FILE_NOT_FOUND;
    }

    // BuildExplicitAccessWithName needs access to a contiguous non-const chunk of memory
    // +1 because std::string.length() doesn't include the null terminator
    std::vector<std::wstring::value_type> userNameForExplicitAccess(userName.length() + 1);
    std::copy(userName.begin(), userName.end(), userNameForExplicitAccess.begin());

    std::vector<EXPLICIT_ACCESS> explicitAccesses;
    for (const auto permission : permissions)
    {
        EXPLICIT_ACCESS explicitAccess = {};
        BuildExplicitAccessWithName(
            &explicitAccess,
            &userNameForExplicitAccess[0],
            permission.AccessPermissions,
            permission.AccessMode,
            permission.Inheritance);
        explicitAccesses.push_back(explicitAccess);
    }

    PACL oldAcl;
    dd::acl_traits::unique_ptr newAcl;
    dd::security_descriptor_traits::unique_ptr securityDescr;
    RETURN_IF_FAILED(
        GetNamedSecurityInfo(
            filePath.c_str(),
            SE_FILE_OBJECT,
            DACL_SECURITY_INFORMATION,
            nullptr,
            nullptr,
            &oldAcl,
            nullptr,
            &dd::security_descriptor_traits::store_ptr(securityDescr)));

    RETURN_IF_FAILED(
        SetEntriesInAcl(
            explicitAccesses.size(),
            &explicitAccesses[0],
            oldAcl,
            &dd::acl_traits::store_ptr(newAcl)));

    // SetNamedSecurityInfo needs access to a contiguous non-const chunk of memory
    // +1 because std::string.length() doesn't include the null terminator
    std::vector<std::wstring::value_type> filePathForNamedSecurityInfo(filePath.length() + 1);
    std::copy(filePath.begin(), filePath.end(), filePathForNamedSecurityInfo.begin());

    RETURN_IF_FAILED(
        SetNamedSecurityInfo(
            &filePathForNamedSecurityInfo[0],
            SE_FILE_OBJECT,
            DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
            nullptr,
            nullptr,
            newAcl.get(),
            nullptr));

    return ERROR_SUCCESS;
}

void removeUserPermsFromFile(std::wstring &filename, PSID sidremove)
{
    if(!PathFileExistsW((LPCWSTR) filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file doesn't exist, not doing anything");
        return ;
    }
    ExplicitAccess dduser;
    // get the current ACLs;  check to see if the DD user is in there, if so
    // remove
    std::string shortfile;
    toMbcs(shortfile, filename.c_str());
    DWORD dwRes = 0;
    PACL pOldDacl = NULL;
    PSECURITY_DESCRIPTOR pSD = NULL;
    ACL_SIZE_INFORMATION sizeInfo;
    memset(&sizeInfo, 0, sizeof(ACL_SIZE_INFORMATION));

    dwRes = GetNamedSecurityInfo(filename.c_str(), SE_FILE_OBJECT, 
          DACL_SECURITY_INFORMATION,
          NULL, NULL, &pOldDacl, NULL, &pSD);
    if (ERROR_SUCCESS != dwRes) {
        WcaLog(LOGMSG_STANDARD, "Failed to get file DACL, not removing user perms");
        return;
    }
    BOOL bRet = GetAclInformation(pOldDacl, (PVOID)&sizeInfo, sizeof(ACL_SIZE_INFORMATION), AclSizeInformation);
    if(FALSE == bRet) {
        WcaLog(LOGMSG_STANDARD, "Failed to get DACL size information");
        goto doneRemove;
    }
    for(int i = 0; i < sizeInfo.AceCount; i++) {
        ACCESS_ALLOWED_ACE *ace;

        if (GetAce(pOldDacl, i, (LPVOID*)&ace)) {
            PSID compareSid = (PSID)(&ace->SidStart);
            if (EqualSid(compareSid, sidremove)) {
                WcaLog(LOGMSG_STANDARD, "Matched sid on file %s, removing", shortfile.c_str());
                if (!DeleteAce(pOldDacl, i)) {
                    WcaLog(LOGMSG_STANDARD, "Failed to delete ACE on file %s", shortfile.c_str());
                }
            }
        }
    }
    dwRes = SetNamedSecurityInfoW((LPWSTR) filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION,
            NULL, NULL, pOldDacl, NULL);
    if(dwRes != 0) {
        WcaLog(LOGMSG_STANDARD, "%d resetting permissions on %s", dwRes, shortfile.c_str());
    }

doneRemove:

    if(pSD){
        LocalFree((HLOCAL) pSD);
    }
    
    return ;
}

int doCreateUser(const std::wstring& name, const wchar_t * domain, std::wstring& comment, const wchar_t* passbuf)
{
    
    USER_INFO_1 ui;
    memset(&ui, 0, sizeof(USER_INFO_1));
    WcaLog(LOGMSG_STANDARD, "entered createuser");
    ui.usri1_name = (LPWSTR)name.c_str();
    ui.usri1_password = (LPWSTR)passbuf;
    ui.usri1_priv = USER_PRIV_USER;
    ui.usri1_comment = (LPWSTR)comment.c_str();
    ui.usri1_flags = UF_DONT_EXPIRE_PASSWD;
    DWORD ret = 0;
    

    WcaLog(LOGMSG_STANDARD, "Calling NetUserAdd.");
    ret = NetUserAdd(NULL, // LOCAL_MACHINE
        1, // indicates we're using a USER_INFO_1
        (LPBYTE)&ui,
        NULL);
    WcaLog(LOGMSG_STANDARD, "NetUserAdd. %d", ret);
    return ret;

}



DWORD DeleteUser(const wchar_t* host, const wchar_t* name){
    NET_API_STATUS ret = NetUserDel(NULL, name);
    return (DWORD)ret;
}



bool isDomainController(MSIHANDLE hInstall)
{
    bool ret = false;
    DWORD status = 0;
    SERVER_INFO_101 *si = NULL;
    DWORD le = 0;
    status = NetServerGetInfo(NULL, 101, (LPBYTE *)&si);
    if (NERR_Success != status) {
        le = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to get server info");
        return false;
    }
    if (SV_TYPE_WORKSTATION & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_WORKSTATION");
    }
    if (SV_TYPE_SERVER & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_SERVER\n");
    }
    if (SV_TYPE_DOMAIN_CTRL & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_CTRL\n");
        ret = true;
    }
    if (SV_TYPE_DOMAIN_BAKCTRL & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_BAKCTRL\n");
        ret = true;
    }
    if (si) {
        NetApiBufferFree((LPVOID)si);
    }
    return ret;
}

int doesUserExist(MSIHANDLE hInstall, const CustomActionData& data, bool isDC)
{
    int retval = 0;
    SID *newsid = NULL;
    DWORD cbSid = 0;
    LPWSTR refDomain = NULL;
    DWORD cchRefDomain = 0;
    SID_NAME_USE use;
    std::string narrowdomain;
    DWORD err = 0;
    const wchar_t * userToTry = data.getQualifiedUsername().c_str();

    BOOL bRet = LookupAccountName(NULL, userToTry, newsid, &cbSid, refDomain, &cchRefDomain, &use);
    if (bRet) {
        err = GetLastError();
        // this should *never* happen, because we didn't pass in a buffer large enough for
        // the sid or the domain name.
        WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
        return -1;
    }
    err = GetLastError();
    if (ERROR_NONE_MAPPED == err) {
        // this user doesn't exist.  We're done
        return 0;
    }
    if (ERROR_INSUFFICIENT_BUFFER != err) {
        if (!isDC) {
            // can only try this if we're not on a primary/backup DC; on DCs we must
            // be able to contact the domain authority.  
            if (err >= ERROR_NO_TRUST_LSA_SECRET && err <= ERROR_TRUST_FAILURE) {
                WcaLog(LOGMSG_STANDARD, "Can't reach domain controller %d", err);
                // if the user specified a domain, then also must be able to contact
                // the domain authority
                if (data.getDomainPtr() == NULL) {
                    WcaLog(LOGMSG_STANDARD, "trying fully qualified local account");
                    bRet = LookupAccountName(NULL, data.getFullUsername().c_str(), newsid, &cbSid, refDomain, &cchRefDomain, &use);
                    if (bRet) {
                        // this should *never* happen, because we didn't pass in a buffer large enough for
                        // the sid or the domain name.
                        WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
                        return -1;
                    }
                    err = GetLastError();
                    if (ERROR_NONE_MAPPED == err) {
                        // this user doesn't exist.  We're done
                        WcaLog(LOGMSG_STANDARD, "retried user doesn't exist");
                        return 0;
                    }
                    if (ERROR_INSUFFICIENT_BUFFER != err) {
                        WcaLog(LOGMSG_STANDARD, "Failed retry of lookup account name %d", err);
                        return -1;
                    }
                }
                else {
                    WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: supplied domain, but can't check user database %d 0x%x", err, err);
                    return -1;
                }
            }
            else {
                WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
                return -1;
            }
            userToTry = data.getFullUsername().c_str();

        }
        else {        // we don't know what happened
            // on a DC, can't try without domain access
            WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Expected insufficient buffer, got error %d 0x%x", err, err);
            return -1;
        }
    }
    newsid = (SID *) new BYTE[cbSid];
    ZeroMemory(newsid, cbSid);

    refDomain = new wchar_t[cchRefDomain + 1];
    ZeroMemory(refDomain, (cchRefDomain + 1) * sizeof(wchar_t));

    // try it again
    bRet = LookupAccountName(NULL, userToTry, newsid, &cbSid, refDomain, &cchRefDomain, &use);
    if (!bRet) {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to lookup account name %d", GetLastError());
        retval = -1;
        goto cleanAndFail;
    }
    if (!IsValidSid(newsid)) {
        WcaLog(LOGMSG_STANDARD, "New SID is invalid");
        retval = -1;
        goto cleanAndFail;
    }
    retval = 1;
    toMbcs(narrowdomain, refDomain);
    WcaLog(LOGMSG_STANDARD, "Got SID from %s", narrowdomain.c_str());

cleanAndFail:
    if (newsid) {
        delete[](BYTE*)newsid;
    }
    if (refDomain) {
        delete[] refDomain;
    }
    return retval;
}

namespace {
	std::wstring getLastErrorMessage() {
		return L"Error code: " + std::to_wstring(GetLastError());
	}
}

bool setUserProfileFolder(const std::wstring& username, const wchar_t* domain, const std::wstring& password) {
	HANDLE token = nullptr;
	if (LogonUser(username.c_str(), domain, password.c_str(), LOGON32_LOGON_SERVICE, 
					LOGON32_PROVIDER_DEFAULT, &token) == 0) {
		WcaLog(LOGMSG_STANDARD, "Cannot logon as user %S: %S.", username.c_str(), getLastErrorMessage().c_str());
		return false;
	}
	
	PROFILEINFO profileInfo;
	ZeroMemory(&profileInfo, sizeof(PROFILEINFO));
	profileInfo.dwSize = sizeof(PROFILEINFO);
	std::vector<wchar_t> profileUserName{ username.begin(), username.end() };
	profileUserName.push_back(0);
	profileInfo.lpUserName = &profileUserName[0];
	
	if (!LoadUserProfile(token, &profileInfo)) {
		WcaLog(LOGMSG_STANDARD, "Cannot load profile for %S: %S", username.c_str(), getLastErrorMessage().c_str());
		return false;
	}
	
	DWORD bufferSize = 0;	
	if (GetUserProfileDirectory(token, nullptr, &bufferSize) || GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
		WcaLog(LOGMSG_STANDARD, "Cannot get the user profile buffer size. %S", getLastErrorMessage().c_str());
		return false;
	}
	
	std::vector<wchar_t> userProfileFolder(bufferSize);	
	if (!GetUserProfileDirectory(token, &userProfileFolder[0], &bufferSize)) {
		WcaLog(LOGMSG_STANDARD, "Cannot get the user profile. %S", getLastErrorMessage().c_str());
		return false;
	}

	USER_INFO_1006 ui;
	ui.usri1006_home_dir = &userProfileFolder[0];
	
	if (NetUserSetInfo(nullptr, username.c_str(), 1006, reinterpret_cast<LPBYTE>(&ui), nullptr) != NERR_Success) {
		WcaLog(LOGMSG_STANDARD, "Cannot set user profile. %S", getLastErrorMessage().c_str());
		return false;
	}
	
	WcaLog(LOGMSG_STANDARD, "User profile set to: %S", ui.usri1006_home_dir);
	return true;
}

bool getUserProfileFolder(const std::wstring& username, std::wstring& userPofileFolder) {
	USER_INFO_1* userInfo1 = nullptr;

	if (NetUserGetInfo(nullptr, username.c_str(), 1, reinterpret_cast<LPBYTE*>(&userInfo1)) != NERR_Success) {
		WcaLog(LOGMSG_STANDARD, "NetUserGetInfo failed.");
		return false;
	}

	if (!userInfo1->usri1_home_dir) {
		WcaLog(LOGMSG_STANDARD, "userInfo1->usri1_home_dir is null.");
		return false;
	}
	WcaLog(LOGMSG_STANDARD, "User profile is: %S", userInfo1->usri1_home_dir);

	userPofileFolder = userInfo1->usri1_home_dir;
	NetApiBufferFree(userInfo1);
	return true;
}

namespace {	
	void RemoveLockedFolder(const std::wstring& folder) {
		std::vector<std::filesystem::path> paths;

		paths.emplace_back(folder);
		for (auto p : std::filesystem::recursive_directory_iterator(folder)) {
			paths.push_back(p.path());
		}

		// We reverse the order because we need to delete empty folders.
		std::reverse(paths.begin(), paths.end());

		for (const auto& p : paths) {
			std::error_code error;
			std::filesystem::remove(p, error);
			if (error) {
				// Delete the file or folder on the next reboot.
				// Folder must be empty
				if (!MoveFileEx(p.c_str(), nullptr, MOVEFILE_DELAY_UNTIL_REBOOT)) {
					WcaLog(LOGMSG_STANDARD, "Cannot remove path: %s (Error code %d)", p.c_str(), GetLastError());
				}
			}
		}
	}

	bool RemoveRegisterKey(const std::wstring& userSid) {
		if (userSid.empty())
			return false;

		std::wstring key = L"SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\ProfileList\\" + userSid;
		auto status = RegDeleteKey(HKEY_LOCAL_MACHINE, key.c_str());
		if (status != ERROR_SUCCESS) {
			WcaLog(LOGMSG_STANDARD, "Cannot remove registry key %S: (Error code %d)", key.c_str(), status);
			return false;
		}
		return true;
	}
}

std::optional<std::wstring> GetSidString(PSID sid) {
	LPWSTR stringSid = nullptr;
	if (!ConvertSidToStringSid(sid, &stringSid))
		return std::nullopt;
	
	std::wstring result = stringSid;

	LocalFree(stringSid);
	return result;
}

bool RemoveUserProfile(const std::wstring& installedUser, const std::wstring& userProfileFolder, const std::wstring& userSid) {
	try {		
		RemoveLockedFolder(userProfileFolder);
		return RemoveRegisterKey(userSid);
	}
	catch (const std::exception& e) {
		WcaLog(LOGMSG_STANDARD, "Error: %s in RemoveUserProfile.", e.what());
	}
	catch (...) {
		WcaLog(LOGMSG_STANDARD, "Unknow error in RemoveUserProfile.");
	}
	return false;
}
