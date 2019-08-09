#include "stdafx.h"
#include <io.h>
#include <shellapi.h>

BOOL IsDots(const TCHAR* str) {
    if (wcscmp(str, L".") && wcscmp(str, L"..")) return FALSE;
    return TRUE;
}
BOOL DeleteFilesInDirectory(const wchar_t* dirname, const wchar_t* ext) {
    HANDLE hFind;
    WIN32_FIND_DATA FindFileData;

    std::wstring DirPath = dirname;
    DirPath += L"\\";
    DirPath += ext;

    hFind = FindFirstFile(DirPath.c_str(), &FindFileData);
    DWORD err = GetLastError();
    bool bSearch = true;
    if (INVALID_HANDLE_VALUE == hFind && err != ERROR_FILE_NOT_FOUND) {
        return FALSE;
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
            if ((FindFileData.dwFileAttributes &
                FILE_ATTRIBUTE_DIRECTORY))
            {

                // we have found a directory, recurse
                if (!DeleteFilesInDirectory(FileName.c_str(), ext))
                {
                    FindClose(hFind);
                    return FALSE;    // directory couldn't be deleted
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
                    WcaLog(LOGMSG_STANDARD, "Failed to delete pyc file");
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
                if (!DeleteFilesInDirectory(FileName.c_str(), ext))
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
