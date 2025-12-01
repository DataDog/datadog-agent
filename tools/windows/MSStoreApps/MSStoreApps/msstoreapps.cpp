#include <windows.h>
#include <combaseapi.h>
#include <string>
#include <vector>
#include <cstring>
#include <stdexcept>

#include <winrt/base.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Management.Deployment.h>

#include "msstoreapps.h"

using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::Management::Deployment;
using namespace winrt::Windows::System;

// Return codes
constexpr int RESULT_SUCCESS = 0;
constexpr int RESULT_INVALID_PARAMS = 1;
constexpr int RESULT_EXCEPTION = 2;

static char *hstring_to_str(const winrt::hstring &hs) {
    if (hs.empty()) {
        char *p = static_cast<char *>(CoTaskMemAlloc(1));
        if (!p) {
            throw std::bad_alloc{};
        }
        p[0] = '\0';
        return p;
    }

    // Convert hstring to UTF-8
    int32_t len = WideCharToMultiByte(CP_UTF8, 0, hs.c_str(), -1, nullptr, 0, nullptr, nullptr);
    if (len <= 0) {
        throw std::runtime_error("WideCharToMultiByte failed");
    }

    char *p = static_cast<char *>(CoTaskMemAlloc(len));
    if (!p) {
        throw std::bad_alloc{};
    }

    len = WideCharToMultiByte(CP_UTF8, 0, hs.c_str(), -1, p, len, nullptr, nullptr);
    if (len <= 0) {
        CoTaskMemFree(p);
        throw std::runtime_error("WideCharToMultiByte failed");
    }
    return p;
}

static char *ver_to_str(PackageVersion const &v) {
    char buf[32];
    int len = _snprintf_s(buf, _TRUNCATE, "%u.%u.%u.%u", v.Major, v.Minor, v.Build, v.Revision);
    if (len < 0) {
        len = 0;
        buf[0] = '\0';
    }
    char *p = static_cast<char *>(CoTaskMemAlloc(len + 1));
    if (!p) {
        throw std::bad_alloc{};
    }
    memcpy(p, buf, len + 1);
    return p;
}

static char *dt_to_iso(DateTime const &dt) {
    ULARGE_INTEGER li{};
    li.QuadPart = static_cast<ULONGLONG>(dt.time_since_epoch().count());
    FILETIME ft{ li.LowPart, li.HighPart };
    SYSTEMTIME st{};
    char buf[32];
    int len;

    if (!FileTimeToSystemTime(&ft, &st)) {
        buf[0] = '\0';
        len = 0;
    } else {
        len = _snprintf_s(buf, _TRUNCATE, "%04u-%02u-%02uT%02u:%02u:%02uZ",
            st.wYear, st.wMonth, st.wDay, st.wHour, st.wMinute, st.wSecond);
        if (len < 0) {
            len = 0;
            buf[0] = '\0';
        }
    }

    char *p = static_cast<char *>(CoTaskMemAlloc(len + 1));
    if (!p) {
        throw std::bad_alloc{};
    }
    memcpy(p, buf, len + 1);
    return p;
}

static uint8_t is64(ProcessorArchitecture a) {
    if (a == ProcessorArchitecture::X64 || a == ProcessorArchitecture::Arm64) {
        return 1;
    }
    return 0;
}

static MSStoreEntry make_entry(const Package &pkg, winrt::hstring displayName) {
    auto id = pkg.Id();
    char *installDate = nullptr;
    // Not all packages have InstalledDate
    try {
        installDate = dt_to_iso(pkg.InstalledDate());
    } catch (...) {
        installDate = static_cast<char *>(CoTaskMemAlloc(1));
        if (!installDate) {
            throw std::bad_alloc{};
        }
        installDate[0] = '\0';
    }

    MSStoreEntry e{};
    e.display_name = nullptr;
    e.version = nullptr;
    e.install_date = nullptr;
    e.publisher = nullptr;
    e.product_code = nullptr;

    try {
        e.display_name = hstring_to_str(displayName);
        e.version = ver_to_str(id.Version());
        e.install_date = installDate;
        e.is_64bit = is64(id.Architecture());
        e.publisher = hstring_to_str(id.Publisher());
        e.product_code = hstring_to_str(id.FamilyName());
        return e;
    } catch (...) {
        CoTaskMemFree(e.display_name);
        CoTaskMemFree(e.version);
        CoTaskMemFree(installDate);
        CoTaskMemFree(e.publisher);
        CoTaskMemFree(e.product_code);
        throw;
    }
}

extern "C" __declspec(dllexport) int ListStoreEntries(MSStoreEntry **out_array, int32_t *out_count) {
    if (!out_array || !out_count) {
        return RESULT_INVALID_PARAMS;
    }
    *out_array = nullptr;
    *out_count = 0;

    std::vector<MSStoreEntry> rows;
    MSStoreEntry *arr = nullptr;

    try {
        winrt::init_apartment();

        PackageManager pm;

        auto packages = pm.FindPackagesWithPackageTypes(PackageTypes::Main);

        for (auto const &pkg : packages) {
            auto id = pkg.Id();
            auto displayName = id.Name();
            auto appListEntries = pkg.GetAppListEntries();

            if (appListEntries.Size() == 0) {
                rows.push_back(make_entry(pkg, displayName));
            } else {
                for (auto const &appListEntry : appListEntries) {
                    auto displayInfo = appListEntry.DisplayInfo();
                    if (displayInfo) {
                        auto dn = displayInfo.DisplayName();
                        if (!dn.empty()) {
                            displayName = dn;
                        }
                    }

                    rows.push_back(make_entry(pkg, displayName));
                }
            }
        }

        if (!rows.empty()) {
            size_t bytes = rows.size() * sizeof(MSStoreEntry);
            arr = static_cast<MSStoreEntry *>(CoTaskMemAlloc(bytes));
            if (!arr) {
                throw std::bad_alloc{};
            }
            memcpy(arr, rows.data(), bytes);
            *out_array = arr;
            *out_count = static_cast<int32_t>(rows.size());
        }
        return RESULT_SUCCESS;
    } catch (...) {
        for (auto &e : rows) {
            CoTaskMemFree(e.display_name);
            CoTaskMemFree(e.version);
            CoTaskMemFree(e.install_date);
            CoTaskMemFree(e.publisher);
            CoTaskMemFree(e.product_code);
        }
        CoTaskMemFree(arr);
        return RESULT_EXCEPTION;
    }
}

extern "C" __declspec(dllexport) void FreeStoreEntries(MSStoreEntry *entries, int32_t count) {
    if (!entries) {
        return;
    }
    for (int32_t i = 0; i < count; ++i) {
        CoTaskMemFree(entries[i].display_name);
        CoTaskMemFree(entries[i].version);
        CoTaskMemFree(entries[i].install_date);
        CoTaskMemFree(entries[i].publisher);
        CoTaskMemFree(entries[i].product_code);
    }
    CoTaskMemFree(entries);
}