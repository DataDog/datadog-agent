#include "stdafx.h"
#include <io.h>
BOOL IsDots(const TCHAR* str) {
    if (wcscmp(str, L".") && wcscmp(str, L"..")) return FALSE;
    return TRUE;
}
BOOL DeleteDirectory(const TCHAR* sPath) {
    HANDLE hFind;    // file handle
    WIN32_FIND_DATA FindFileData;

    TCHAR DirPath[MAX_PATH];
    TCHAR FileName[MAX_PATH];

    wcscpy(DirPath, sPath);
    wcscat(DirPath, L"\\*.pyc");    // searching all files
    wcscpy(FileName, sPath);
    wcscat(FileName, L"\\");

    // find the first file
    hFind = FindFirstFile(DirPath, &FindFileData);
    if (hFind == INVALID_HANDLE_VALUE) return FALSE;
    wcscpy(DirPath, FileName);

    bool bSearch = true;
    while (bSearch) {    // until we find an entry
        if (FindNextFile(hFind, &FindFileData)) {
            if (IsDots(FindFileData.cFileName)) continue;
            wcscat(FileName, FindFileData.cFileName);
            if ((FindFileData.dwFileAttributes &
                FILE_ATTRIBUTE_DIRECTORY)) {

                // we have found a directory, recurse
                if (!DeleteDirectory(FileName)) {
                    FindClose(hFind);
                    return FALSE;    // directory couldn't be deleted
                }
                // remove the empty directory
                RemoveDirectory(FileName);
                wcscpy(FileName, DirPath);
            }
            else {
                if (FindFileData.dwFileAttributes &
                    FILE_ATTRIBUTE_READONLY)
                    // change read-only file mode
                    _wchmod(FileName, _S_IWRITE);
                if (!DeleteFile(FileName)) {    // delete the file
                    FindClose(hFind);
                    return FALSE;
                }
                wcscpy(FileName, DirPath);
            }
        }
        else {
            // no more files there
            if (GetLastError() == ERROR_NO_MORE_FILES)
                bSearch = false;
            else {
                // some error occurred; close the handle and return FALSE
                FindClose(hFind);
                return FALSE;
            }

        }

    }
    FindClose(hFind);                  // close the file handle

    return TRUE;

}
