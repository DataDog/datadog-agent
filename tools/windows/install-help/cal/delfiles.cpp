#include "stdafx.h"
#include <io.h>
BOOL IsDots(const TCHAR* str) {
    if (wcscmp(str, L".") && wcscmp(str, L"..")) {
        return FALSE;
    }
    return TRUE;
}

/**
 * This function recursively deletes all files in a given tree that match a given
 * extension.  It will only accept an absolute path.
 * 
 * @param dirname  The root path to start the deletion
 * 
 * @param ext   the filename and/or extension to delete.  Can be a fixed name or wildcard
 * 
 * @param dirs  If true, will delete directories which match the ext parameter, if the
 *              directory is empty. If false (the default), will delete files
 */
BOOL DeleteFilesInDirectory(const wchar_t* dirname, const wchar_t* ext, bool dirs) {
    HANDLE hFind;
    WIN32_FIND_DATA FindFileData;

    std::filesystem::path checkpath(dirname);
    if(!checkpath.is_absolute()){
        // don't allow this.   If the path is not absolute, assume we didn't mean
        // to delete it
        WcaLog(LOGMSG_STANDARD, "Not deleting directory %S, not absolute", dirname);
        return false;
    }
    std::wstring DirPath = dirname;
    DirPath += L"\\";
    DirPath += ext;

    hFind = FindFirstFile(DirPath.c_str(), &FindFileData);
    DWORD err = GetLastError();
    bool bSearch = true;
    if (INVALID_HANDLE_VALUE == hFind && err != ERROR_FILE_NOT_FOUND) {
        return TRUE;
    } else if (hFind != INVALID_HANDLE_VALUE)
    {
        for (bool bContinue = true; bContinue; bContinue = FindNextFile(hFind, &FindFileData))
        {
            if (IsDots(FindFileData.cFileName))
            {
                continue;
            }
            std::wstring FileName = dirname;
            FileName += L"\\";
            FileName += FindFileData.cFileName;
            WcaLog(LOGMSG_STANDARD, "checking %S %x", FindFileData.cFileName, FindFileData.dwFileAttributes);
            if ((FindFileData.dwFileAttributes &
                FILE_ATTRIBUTE_DIRECTORY))
            {

                // we have found a directory, recurse
                if (!DeleteFilesInDirectory(FileName.c_str(), ext, dirs))
                {
                    FindClose(hFind);
                    return FALSE;    // directory couldn't be deleted
                }
                if(dirs) {
                    if (FindFileData.dwFileAttributes &
                        FILE_ATTRIBUTE_READONLY)
                    {
                        // change read-only file mode
                        if (_wchmod(FileName.c_str(), _S_IWRITE))
                        {
                            WcaLog(LOGMSG_STANDARD, "Failed to change perms on %S", FileName.c_str());
                        }
                    }

                    if (!RemoveDirectory(FileName.c_str()))
                    {
                        err = GetLastError();
                        WcaLog(LOGMSG_STANDARD, "Failed to delete directory %d %S", err, FileName.c_str());
                    }
                }
            }
            else {
                if (FindFileData.dwFileAttributes &
                    FILE_ATTRIBUTE_READONLY)
                {
                    // change read-only file mode
                    _wchmod(FileName.c_str(), _S_IWRITE);
                }
                if (!DeleteFile(FileName.c_str())) {    // delete the file
                    WcaLog(LOGMSG_STANDARD, "Failed to delete file %S", FileName.c_str());
                }
            }
        }
        err = GetLastError();
        FindClose(hFind);                  // close the file handle
        if (ERROR_NO_MORE_FILES != err) {
            return FALSE;
        }
    }
    // now go back and redo, just looking for the directories.
    DirPath = dirname;
    DirPath += L"\\*";

    hFind = FindFirstFile(DirPath.c_str(), &FindFileData);
    if (INVALID_HANDLE_VALUE == hFind) {
        return FALSE;
    }
    else if (hFind != INVALID_HANDLE_VALUE)
    {
        for (bool bContinue = true; bContinue; bContinue = FindNextFile(hFind, &FindFileData))
        {
            if (IsDots(FindFileData.cFileName))
            {
                continue;
            }
            if ((FindFileData.dwFileAttributes &
                FILE_ATTRIBUTE_DIRECTORY))
            {
                std::wstring FileName = dirname;
                FileName += L"\\";
                FileName += FindFileData.cFileName;

                // we have found a directory, recurse
                if (!DeleteFilesInDirectory(FileName.c_str(), ext, dirs))
                {
                    FindClose(hFind);
                    return FALSE;    // directory couldn't be deleted
                }
            }
        }
        err = GetLastError();
        FindClose(hFind);                  // close the file handle
        if (ERROR_NO_MORE_FILES != err) {
            return FALSE;
        }
    }
    
    return TRUE;
}


/**
 * This function recursively deletes all  directories in the profile
 * directory belonging to the given user.  It will only delete directories
 * that the given user has been granted explicit access to, to prevent
 * deleting incorrect directories
 *
 * @param user  The username for which directories are being deleted
 *
 * @param userSID  The SID for the aforementioned user
 *
 */
BOOL DeleteHomeDirectory(const std::wstring &user, PSID userSID)
{
    // first, find the path to the home directories
    bool ret = false;
    std::vector<wchar_t> homeDir;
    DWORD homeDirSize = _MAX_PATH;
    bool needsLarger = false;
    HANDLE hFind = INVALID_HANDLE_VALUE;
    WIN32_FIND_DATA findFileData;
    DWORD err;
    std::wstring search;
    do
    {
        homeDir.resize(homeDirSize);
        if (!GetProfilesDirectory(homeDir.data(), &homeDirSize))
        {
            err = GetLastError();
            if (ERROR_INSUFFICIENT_BUFFER == err)
            {
                // loop back again.
                needsLarger = true;
                WcaLog(LOGMSG_STANDARD, "Finding home directory, need larger path %d", homeDirSize);
            }
            else {
                WcaLog(LOGMSG_STANDARD, "Error getting home directory %d", err);
                goto doneDelete;
            }
        } 
    } while (needsLarger);

    // enumerate all of the directories in the profile directory that might match
    // this one.  Need the wildcards because the OS will add suffixes if we have a
    // collision.
    search = homeDir.data(); 
    search += L"\\*" + user + L"*";

    hFind = FindFirstFile(search.c_str(), &findFileData);
    err = GetLastError();
    if (INVALID_HANDLE_VALUE == hFind && err != ERROR_FILE_NOT_FOUND) {
        ret = false;
        goto doneDelete;
    }
    else if (hFind != INVALID_HANDLE_VALUE)
    {
        for (bool bContinue = true; bContinue; bContinue = FindNextFile(hFind, &findFileData))
        {
            if (IsDots(findFileData.cFileName))
            {
                continue;
            }
            if (!(findFileData.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY))
            {
                // we're only looking for directories at this point
                continue;
            }
            std::wstring fullpath = homeDir.data();
            fullpath += L"\\";
            fullpath += findFileData.cFileName;

            // get the sid for the file
            // Get the owner SID of the file.
            PACL fileDacl;
            DWORD dwRet = GetNamedSecurityInfo(fullpath.c_str(),
                SE_FILE_OBJECT,
                DACL_SECURITY_INFORMATION,
                NULL,
                NULL,
                &fileDacl,
                NULL,
                NULL);
            if (0 != dwRet) {
                WcaLog(LOGMSG_STANDARD, "Failed to get security info for %S %d", findFileData.cFileName, dwRet);
            }
            else {
                // get the size information
                ACL_SIZE_INFORMATION aclsizeinfo;
                DWORD aclsize = sizeof(ACL_SIZE_INFORMATION);
                bool bMatched = false;
                
                if (!GetAclInformation(fileDacl, &aclsizeinfo, aclsize, AclSizeInformation))
                {
                    WcaLog(LOGMSG_STANDARD, "Failed to get acl size information %d", GetLastError());
                    continue;
                }
                for (int i = 0; i < aclsizeinfo.AceCount; i++)
                {
                    LPVOID pAce = NULL;
                    if (GetAce(fileDacl, i, &pAce))
                    {
                        ACE_HEADER *hdr = (ACE_HEADER*)pAce;
                        if (hdr->AceType == ACCESS_ALLOWED_ACE_TYPE)
                        {
                            ACCESS_ALLOWED_ACE * aaa = (ACCESS_ALLOWED_ACE*)pAce;
                            PSID thisSid = &(aaa->SidStart);
                            if (EqualSid(userSID, thisSid))
                            {
                                WcaLog(LOGMSG_STANDARD, "User sid has access to %S, scheduling for delete", fullpath.c_str());
                                bMatched = true;
                                break;
                            }
                        }
                    }
                }
                if (bMatched)
                {
                    WcaLog(LOGMSG_STANDARD, "SID is equal; deleting %S", findFileData.cFileName);
                    DeleteFilesInDirectory(fullpath.c_str(), L"*.*", true);
                    RemoveDirectory(fullpath.c_str());
                }
                else {
                    WcaLog(LOGMSG_STANDARD, "SID not equal, not deleting %S", findFileData.cFileName);
                }

            }

        }
    }
doneDelete:
    if (INVALID_HANDLE_VALUE != hFind) {
        FindClose(hFind);
    }
    return ret;

}
