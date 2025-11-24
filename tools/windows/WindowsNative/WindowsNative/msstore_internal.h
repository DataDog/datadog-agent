#pragma once

#ifdef __cplusplus

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

#include "windows_native_msstore.h"

struct MSStoreInternal : MSStore {
    std::vector<MSStoreEntry> entriesVec;
    std::vector<winrt::hstring> strings;
};

int ListStoreEntries(MSStoreInternal **out);
void FreeStoreEntries(MSStoreInternal *store);

#endif
