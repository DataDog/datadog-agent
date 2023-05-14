#include "stdafx.h"

void parseKeyValueString(const std::wstring kvstring, std::map<std::wstring, std::wstring> &values)
{
    // Parse values out of property
    std::wstringstream ss(kvstring);
    std::wstring token;

    while (std::getline(ss, token))
    {
        // 'token' contains "<key>=<value>"
        std::wstringstream instream(token);
        std::wstring key, val;
        if (std::getline(instream, key, L'='))
        {
            trim_string(key);
            std::getline(instream, val);
            trim_string(val);
            if (!key.empty() && !val.empty())
            {
                values[key] = val;
            }
        }
    }
}

bool StaticPropertyView::present(const std::wstring &key) const
{
    return this->values.count(key) != 0;
}

bool StaticPropertyView::value(const std::wstring &key, std::wstring &val) const
{
    const auto kvp = values.find(key);
    if (kvp == values.end())
    {
        return false;
    }
    val = kvp->second;
    return true;
}

CAPropertyView::CAPropertyView(MSIHANDLE hi)
    : _hInstall(hi)
{
}

bool ImmediateCAPropertyView::present(const std::wstring &key) const
{
    std::wstring val;
    return this->value(key, val);
}

bool ImmediateCAPropertyView::value(const std::wstring &key, std::wstring &val) const
{
    std::wstring propertyValue;
    if (loadPropertyString(this->_hInstall, key.c_str(), propertyValue))
    {
        if (!propertyValue.empty())
        {
            val = propertyValue;
            return true;
        }
    }
    return false;
}

ImmediateCAPropertyView::ImmediateCAPropertyView(MSIHANDLE hi)
    : CAPropertyView(hi)
{
}

DeferredCAPropertyView::DeferredCAPropertyView(MSIHANDLE hi)
    : CAPropertyView(hi)
{
    /*
     * Deferred custom actions have limited access to installation
     * details, so we must load our properties from the CustomActionData propety.
     * https://docs.microsoft.com/en-us/windows/win32/msi/obtaining-context-information-for-deferred-execution-custom-actions
     */

    // Load CustomActionData property
    std::wstring data;
    if (!loadPropertyString(this->_hInstall, propertyCustomActionData.c_str(), data))
    {
        throw std::exception("Failed to load CustomActionData property");
    }

    parseKeyValueString(data, this->values);
}
