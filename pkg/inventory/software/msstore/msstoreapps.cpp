#include <windows.h>
#include <roapi.h>
#include <combaseapi.h>
#include <iostream>
#include <string>

#include <winrt/base.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Management.Deployment.h>
#include <winrt/Windows.System.h>

#include "msstoreapps.h"

using namespace winrt;
using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::Management::Deployment;
using namespace winrt::Windows::System;

static char* dup_utf8(const std::string& s) {
    char* p = (char*)CoTaskMemAlloc(s.size() + 1);
    if (!p) throw std::bad_alloc{};
    memcpy(p, s.data(), s.size());
    p[s.size()] = '\0';
    return p;
}
static char* dup_h(hstring const& hs) { return dup_utf8(winrt::to_string(hs)); }
static char* dup_lit(const char* lit) { return dup_utf8(lit); }

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
        winrt::init_apartment(apartment_type::single_threaded);

        PackageManager pm;
        std::vector<MSStoreEntry> rows;
        rows.reserve(256);

        auto iterable = pm.FindPackages();

        for (auto const& pkg : iterable) {
            try {
                if (pkg.IsFramework() || pkg.IsResourcePackage()) continue;

                auto id = pkg.Id();
                auto name = winrt::to_string(id.Name());

                // Keep display name simple for now (avoid AppListEntries while diagnosing)
                std::string display = name;

                MSStoreEntry e{};
                e.display_name = dup_utf8(display);
                e.version = dup_utf8(ver_to_str(id.Version()));
                e.install_date = dup_utf8(dt_to_iso(pkg.InstalledDate()));
                e.source = dup_lit("msstore");
                e.is_64bit = is64(id.Architecture());
                e.publisher = dup_h(id.Publisher());
                e.status = dup_lit("installed");
                e.product_code = dup_h(id.FamilyName());

                rows.push_back(e);
            }
            catch (winrt::hresult_error const& e) {
                std::string s = "[msstore] skip package, HRESULT=0x" +
                    std::to_string((unsigned int)e.code().value) + "\n";
                // skip and continue
            }
            catch (...) {
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
        CoTaskMemFree((void*)entries[i].display_name);
        CoTaskMemFree((void*)entries[i].version);
        CoTaskMemFree((void*)entries[i].install_date);
        CoTaskMemFree((void*)entries[i].source);
        CoTaskMemFree((void*)entries[i].publisher);
        CoTaskMemFree((void*)entries[i].status);
        CoTaskMemFree((void*)entries[i].product_code);
    }
    CoTaskMemFree(entries);
}
