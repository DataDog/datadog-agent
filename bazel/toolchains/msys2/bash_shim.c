// bash_shim: thin C launcher routed to hermetic MSYS2 bash, used as Bazel's
// --shell_executable on Windows. A .bat wrapper here truncates multi-line
// -c arguments at the first newline because cmd.exe re-tokenises argv when
// running batch files; this .exe is invoked directly via CreateProcessW and
// keeps the raw command line intact.
//
// See README.md in this directory for the build/rebuild workflow.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <windows.h>

#define MSYS2_REL L"..\\..\\external\\+msys2_base_repository+msys2_base"
#define MINGW_REL L"..\\..\\external\\+winlibs_mingw_repository+winlibs_mingw64"

static int file_exists(const wchar_t *p) {
    DWORD attr = GetFileAttributesW(p);
    return attr != INVALID_FILE_ATTRIBUTES && !(attr & FILE_ATTRIBUTE_DIRECTORY);
}

static void die_missing(const wchar_t *what, const wchar_t *where, const wchar_t *fetch_hint) {
    fwprintf(stderr, L"bash_shim: hermetic %ls not found at %ls\n", what, where);
    fwprintf(stderr, L"bash_shim: run 'bazelisk fetch %ls' to materialise it\n", fetch_hint);
}

// Skip past argv[0] in a raw command line as Windows would tokenise it.
static const wchar_t *skip_argv0(const wchar_t *cmd) {
    const wchar_t *p = cmd;
    if (*p == L'"') {
        for (++p; *p && *p != L'"'; ++p) {}
        if (*p == L'"') ++p;
    } else {
        while (*p && *p != L' ' && *p != L'\t') ++p;
    }
    while (*p == L' ' || *p == L'\t') ++p;
    return p;
}

int wmain(int argc, wchar_t **argv) {
    (void)argc; (void)argv;

    wchar_t cwd[MAX_PATH];
    if (!GetCurrentDirectoryW(MAX_PATH, cwd)) {
        fwprintf(stderr, L"bash_shim: GetCurrentDirectory failed: %lu\n", GetLastError());
        return 1;
    }

    wchar_t bash_path[MAX_PATH], gcc_path[MAX_PATH], msys_bin[MAX_PATH], mingw_bin[MAX_PATH];
    _snwprintf(bash_path, MAX_PATH, L"%ls\\%ls\\usr\\bin\\bash.exe", cwd, MSYS2_REL);
    _snwprintf(gcc_path,  MAX_PATH, L"%ls\\%ls\\bin\\gcc.exe",       cwd, MINGW_REL);
    _snwprintf(msys_bin,  MAX_PATH, L"%ls\\%ls\\usr\\bin",            cwd, MSYS2_REL);
    _snwprintf(mingw_bin, MAX_PATH, L"%ls\\%ls\\bin",                 cwd, MINGW_REL);

    if (!file_exists(bash_path)) {
        die_missing(L"bash", bash_path, L"@msys2_base//...");
        return 1;
    }
    if (!file_exists(gcc_path)) {
        die_missing(L"gcc", gcc_path, L"@winlibs_mingw64//...");
        return 1;
    }

    // Prepend hermetic tool dirs to PATH so bash and the commands it execs
    // resolve to the pinned binaries rather than whatever leaked in.
    DWORD path_len = GetEnvironmentVariableW(L"PATH", NULL, 0);
    wchar_t *old_path = (wchar_t *)calloc(path_len + 1, sizeof(wchar_t));
    if (!old_path) { fwprintf(stderr, L"bash_shim: oom\n"); return 1; }
    if (path_len) GetEnvironmentVariableW(L"PATH", old_path, path_len);
    size_t new_path_len = wcslen(msys_bin) + wcslen(mingw_bin) + wcslen(old_path) + 3;
    wchar_t *new_path = (wchar_t *)calloc(new_path_len, sizeof(wchar_t));
    if (!new_path) { fwprintf(stderr, L"bash_shim: oom\n"); return 1; }
    _snwprintf(new_path, new_path_len, L"%ls;%ls;%ls", msys_bin, mingw_bin, old_path);
    SetEnvironmentVariableW(L"PATH", new_path);
    free(old_path);
    free(new_path);

    // Forward the raw command line so embedded newlines in -c "..." survive
    // intact instead of going through argv[] reparsing.
    const wchar_t *rest = skip_argv0(GetCommandLineW());
    size_t bash_len = wcslen(bash_path);
    size_t rest_len = wcslen(rest);
    size_t full_len = bash_len + rest_len + 4; // quotes + space + NUL
    wchar_t *full_cmd = (wchar_t *)calloc(full_len, sizeof(wchar_t));
    if (!full_cmd) { fwprintf(stderr, L"bash_shim: oom\n"); return 1; }
    _snwprintf(full_cmd, full_len, L"\"%ls\" %ls", bash_path, rest);

    STARTUPINFOW si = { .cb = sizeof(si) };
    PROCESS_INFORMATION pi = { 0 };
    if (!CreateProcessW(NULL, full_cmd, NULL, NULL, TRUE, 0, NULL, NULL, &si, &pi)) {
        fwprintf(stderr, L"bash_shim: CreateProcess failed: %lu\n", GetLastError());
        free(full_cmd);
        return 1;
    }
    free(full_cmd);

    WaitForSingleObject(pi.hProcess, INFINITE);
    DWORD exit_code = 1;
    GetExitCodeProcess(pi.hProcess, &exit_code);
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    return (int)exit_code;
}
