#include "msstore_internal.h"

using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::Management::Deployment;
using namespace winrt::Windows::System;

// Return codes
constexpr int RESULT_SUCCESS = 0;
constexpr int RESULT_INVALID_PARAMS = 1;
constexpr int RESULT_EXCEPTION = 2;

// Offset between 1601-01-01 and 1970-01-01 in milliseconds
constexpr int64_t EPOCH_DIFF_MILLIS = 11644473600000LL;

static uint8_t is64(ProcessorArchitecture a) {
    if (a == ProcessorArchitecture::X64 || a == ProcessorArchitecture::Arm64) {
        return 1;
    }
    return 0;
}

static int64_t dtToUnixEpochMs(const DateTime& dt) {
    int64_t ticks = dt.time_since_epoch().count();
    // Convert ticks to milliseconds
    int64_t millisSince1601 = ticks / 10000; // 10,000 * 100ns = 1ms
    int64_t unixMillis = millisSince1601 - EPOCH_DIFF_MILLIS;
    return unixMillis;
}

static const wchar_t* copyHStr(MSStoreInternal* msStore, winrt::hstring hstr) {
    // Add to msStore's string vector to keep hstring alive
    msStore->strings.push_back(hstr);
    return hstr.c_str();
}

static void addEntryToStore(MSStoreInternal* msStore, const Package& pkg, winrt::hstring displayName) {
    auto id = pkg.Id();
    int64_t installDate = 0;
    // Not all packages have InstalledDate
    try {
        installDate = dtToUnixEpochMs(pkg.InstalledDate());
    }
    catch (...) {
    }
    PackageVersion version = id.Version();

    MSStoreEntry e{};
    e.display_name = copyHStr(msStore, displayName);
    e.version_major = version.Major;
    e.version_minor = version.Minor;
    e.version_build = version.Build;
    e.version_revision = version.Revision;
    e.install_date = installDate;
    e.is_64bit = is64(id.Architecture());
    e.publisher = copyHStr(msStore, id.Publisher());
    e.product_code = copyHStr(msStore, id.FamilyName());

    msStore->entriesVec.push_back(e);
}

int ListStoreEntries(MSStoreInternal** out) {
    if (!out) {
        return RESULT_INVALID_PARAMS;
    }

    MSStoreInternal* msStore = new MSStoreInternal();
    msStore->count = 0;
    msStore->entries = nullptr;

    try {
        winrt::init_apartment();

        PackageManager pm;

        auto packages = pm.FindPackagesWithPackageTypes(PackageTypes::Main);

        for (auto const &pkg : packages) {
            auto id = pkg.Id();
            auto displayName = id.Name();
            auto appListEntries = pkg.GetAppListEntries();

            if (appListEntries.Size() == 0) {
                addEntryToStore(msStore, pkg, displayName);
            } else {
                for (auto const &appListEntry : appListEntries) {
                    auto displayInfo = appListEntry.DisplayInfo();
                    if (displayInfo) {
                        auto dn = displayInfo.DisplayName();
                        if (!dn.empty()) {
                            displayName = dn;
                        }
                    }

                    addEntryToStore(msStore, pkg, displayName);
                }
            }
        }

        msStore->count = static_cast<int32_t>(msStore->entriesVec.size());
        if (!msStore->entriesVec.empty()) {
            msStore->entries = msStore->entriesVec.data();
        }

        *out = msStore;
        return RESULT_SUCCESS;
    } catch (...) {
        return RESULT_EXCEPTION;
    }
}

void FreeStoreEntries(MSStoreInternal* msStore) {
    delete msStore;
}
