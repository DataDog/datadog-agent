#include <windows.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Management.Deployment.h>

#include "msstoreapps.h"

using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::Management::Deployment;
using namespace winrt::Windows::System;

static uint64_t is64(ProcessorArchitecture a) {
    if (a == ProcessorArchitecture::X64 || a == ProcessorArchitecture::Arm64) {
        return 1;
    }
    return 0;
}

static int64_t dtToUnixTimestamp(const DateTime &dt) {
    // Convert to Unix time_t (seconds since 1970-01-01)
    return winrt::clock::to_time_t(dt);
}

static const wchar_t *copyHStr(MSStoreInternal *msStore, winrt::hstring hstr) {
    // Add to msStore's string vector to keep hstring alive
    msStore->strings.push_back(hstr);
    return hstr.c_str();
}

// Safe accessor template for fields that may throw exceptions
template <typename Func>
static auto safeAccess(Func fn, auto defaultValue) -> decltype(defaultValue) {
    try {
        return fn();
    } catch (...) {
        return defaultValue;
    }
}

static void addEntryToStore(MSStoreInternal *msStore, const Package &pkg, winrt::hstring displayName) {
    auto id = pkg.Id();
    PackageVersion version = safeAccess([&]() { return id.Version(); }, PackageVersion{});

    MSStoreEntry e{};
    e.display_name = copyHStr(msStore, displayName);
    e.version_major = version.Major;
    e.version_minor = version.Minor;
    e.version_build = version.Build;
    e.version_revision = version.Revision;
    e.install_date = safeAccess([&]() { return dtToUnixTimestamp(pkg.InstalledDate()); }, 0LL);
    e.is_64bit = safeAccess([&]() { return is64(id.Architecture()); }, 0ULL);
    e.publisher = copyHStr(msStore, safeAccess([&]() { return id.Publisher(); }, winrt::hstring{}));
    e.product_code = copyHStr(msStore, safeAccess([&]() { return id.FamilyName(); }, winrt::hstring{}));

    msStore->entriesVec.push_back(e);
}

extern "C" __declspec(dllexport) BOOL GetStore(MSStore **out) {
    if (!out) {
        SetLastError(ERROR_INVALID_PARAMETER);
        return FALSE;
    }

    try {
        auto msStore = std::make_unique<MSStoreInternal>();
        msStore->count = 0;
        msStore->entries = nullptr;

        winrt::init_apartment();

        PackageManager pm;

        auto packages = pm.FindPackagesWithPackageTypes(PackageTypes::Main);

        for (auto const &pkg : packages) {
            auto id = pkg.Id();
            auto displayName = safeAccess([&]() { return id.Name(); }, winrt::hstring{});
            auto appListEntries = pkg.GetAppListEntries();

            if (appListEntries.Size() == 0) {
                addEntryToStore(msStore.get(), pkg, displayName);
            } else {
                for (auto const &appListEntry : appListEntries) {
                    auto displayInfo = appListEntry.DisplayInfo();
                    if (displayInfo) {
                        auto dn = displayInfo.DisplayName();
                        if (!dn.empty()) {
                            displayName = dn;
                        }
                    }

                    addEntryToStore(msStore.get(), pkg, displayName);
                }
            }
        }

        msStore->count = static_cast<int64_t>(msStore->entriesVec.size());
        if (!msStore->entriesVec.empty()) {
            msStore->entries = msStore->entriesVec.data();
        }

        *out = msStore.release();
        SetLastError(ERROR_SUCCESS);
        return TRUE;
    } catch (...) {
        SetLastError(ERROR_UNHANDLED_EXCEPTION);
        return FALSE;
    }
}

extern "C" __declspec(dllexport) BOOL FreeStore(MSStore *msStore) {
    if (!msStore) {
        SetLastError(ERROR_INVALID_PARAMETER);
        return FALSE;
    }
    delete static_cast<MSStoreInternal *>(msStore);
    SetLastError(ERROR_SUCCESS);
    return TRUE;
}
