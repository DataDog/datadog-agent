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

using namespace winrt;
using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::Management::Deployment;
using namespace winrt::Windows::System;


static char* make_str(const std::string& s) {
    char* p = (char*)CoTaskMemAlloc(s.size() + 1);
    if (!p) throw std::bad_alloc{};
    memcpy(p, s.data(), s.size());
    p[s.size()] = '\0';
    return p;
}
static std::string ver_to_str(PackageVersion const& v) {
    char buf[32];
    _snprintf_s(buf, _TRUNCATE, "%u.%u.%u.%u", v.Major, v.Minor, v.Build, v.Revision);
    return buf;
}
static std::string dt_to_iso(DateTime const& dt) {
    ULARGE_INTEGER li{}; li.QuadPart = static_cast<ULONGLONG>(dt.time_since_epoch().count());
    FILETIME ft{ li.LowPart, li.HighPart }; SYSTEMTIME st{};
    if (!FileTimeToSystemTime(&ft, &st)) return {};
    char buf[32];
    _snprintf_s(buf, _TRUNCATE, "%04u-%02u-%02uT%02u:%02u:%02uZ",
        st.wYear, st.wMonth, st.wDay, st.wHour, st.wMinute, st.wSecond);
    return buf;
}
static uint8_t is64(ProcessorArchitecture a) {
    return (a == ProcessorArchitecture::X64 || a == ProcessorArchitecture::Arm64) ? 1 : 0;
}

extern "C" __declspec(dllexport)
int ListStoreEntries(MSStoreEntry** out_array, int32_t* out_count) {
    if (!out_array || !out_count) return 1;
    *out_array = nullptr; *out_count = 0;

    try {
        winrt::init_apartment();

        PackageManager pm;
        std::vector<MSStoreEntry> rows;

        auto packages = pm.FindPackagesWithPackageTypes(PackageTypes::Main);

        for (auto const& pkg : packages) {
            auto id = pkg.Id();
            std::string displayName = winrt::to_string(id.Name());
            std::string version = ver_to_str(id.Version());
            std::string installDate;
            // Not all packages have InstalledDate
            try {
                installDate = dt_to_iso(pkg.InstalledDate());
            }
            catch (...) {
                installDate = "";
            }
            std::string publisher = winrt::to_string(id.Publisher());
            std::string productCode = winrt::to_string(id.FamilyName());
            uint8_t is64bit = is64(id.Architecture());

            auto appListEntries = pkg.GetAppListEntries();

            if (appListEntries.Size() == 0) {
                MSStoreEntry e{};
                e.display_name = make_str(displayName);
                e.version = make_str(version);
                e.install_date = make_str(installDate);
                e.is_64bit = is64bit;
                e.publisher = make_str(publisher);
                e.product_code = make_str(productCode);

                rows.push_back(e);
            }
            else {
                for (auto const& appListEntry : appListEntries) {
                    auto displayInfo = appListEntry.DisplayInfo();
                    if (displayInfo) {
                        auto dn = displayInfo.DisplayName();
                        if (!dn.empty()) {
                            displayName = winrt::to_string(dn);
                        }
                    }

                    MSStoreEntry e{};
                    e.display_name = make_str(displayName);
                    e.version = make_str(version);
                    e.install_date = make_str(installDate);
                    e.is_64bit = is64bit;
                    e.publisher = make_str(publisher);
                    e.product_code = make_str(productCode);

                    rows.push_back(e);
                }
            }
        }

        if (!rows.empty()) {
            size_t bytes = rows.size() * sizeof(MSStoreEntry);
            auto* arr = (MSStoreEntry*)CoTaskMemAlloc(bytes);
            if (!arr) throw std::bad_alloc{};
            memcpy(arr, rows.data(), bytes);
            *out_array = arr;
            *out_count = (int32_t)rows.size();
        }
        return 0;
    }
    catch (...) {
        return 2;
    }
}

extern "C" __declspec(dllexport)
void FreeStoreEntries(MSStoreEntry* entries, int32_t count) {
    if (!entries) return;
    for (int32_t i = 0; i < count; ++i) {
        CoTaskMemFree(entries[i].display_name);
        CoTaskMemFree(entries[i].version);
        CoTaskMemFree(entries[i].install_date);
        CoTaskMemFree(entries[i].publisher);
        CoTaskMemFree(entries[i].product_code);
    }
    CoTaskMemFree(entries);
}
