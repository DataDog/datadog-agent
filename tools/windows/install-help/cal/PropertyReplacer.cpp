#include "stdafx.h"
#include "PropertyReplacer.h"

namespace
{
    template <class Map>
    bool has_key(Map const &m, const typename Map::key_type &key)
    {
        auto const &it = m.find(key);
        return it != m.end();
    }

    bool to_bool(std::wstring str)
    {
        std::transform(str.begin(), str.end(), str.begin(), ::tolower);
        std::wistringstream is(str);
        bool b;
        is >> std::boolalpha >> b;
        return b;
    }

    typedef std::function<std::wstring(std::wstring const &, const property_retriever &)> formatter_func;

    /// <summary>
    /// Simply concatenates <paramref name="str"/> with the value of the matching property.
    /// </summary>
    /// <param name="str">The string to use as a replacement.</param>
    /// <returns>A function that conforms to <see cref="formatter_func"> that when called with a property value,
    /// will return a concatenated string of <paramref name="str"/> and the property value. </returns>
    formatter_func format_simple_value(const std::wstring &str)
    {
        return [str](std::wstring const &propertyValue, const property_retriever &)
        {
            return str + propertyValue;
        };
    }

    formatter_func simple_replace(const std::wstring &str)
    {
        return [str](std::wstring const &, const property_retriever &) { return str; };
    }

    std::wstring format_tags(const std::wstring &tags, const property_retriever &)
    {
        std::wistringstream valueStream(tags);
        std::wstringstream result;
        std::wstring token;
        result << L"tags: ";
        while (std::getline(valueStream, token, static_cast<wchar_t>(',')))
        {
            result << std::endl << L"  - " << token;
        }
        return result.str();
    };

    std::wstring format_proxy(std::wstring proxyHost, const property_retriever &propertyRetriever)
    {
        const auto proxyPort = propertyRetriever(L"PROXY_PORT");
        const auto proxyUser = propertyRetriever(L"PROXY_USER");
        const auto proxyPassword = propertyRetriever(L"PROXY_PASSWORD");
        std::wstringstream proxy;
        std::size_t schemeEnd = proxyHost.find(L"://", 0);
        if (schemeEnd == std::string::npos)
        {
            proxy << "http://";
        }
        else
        {
            proxy << proxyHost.substr(0, schemeEnd+3);
            proxyHost.erase(0, schemeEnd+3);
        }
        if (proxyUser)
        {
            proxy << *proxyUser;
            if (proxyPassword)
            {
                proxy << L":" << *proxyPassword;
            }
            proxy << L"@";
        }
        proxy << proxyHost;
        if (proxyPort)
        {
            proxy << L":" << *proxyPort;
        }
        std::wstringstream newValue;
        newValue << L"proxy:" << std::endl
                 << L"  https: " << proxy.str() << std::endl
                 << L"  http: " << proxy.str() << std::endl;
        return newValue.str();
    };

} // namespace

PropertyReplacer::PropertyReplacer(std::wstring &input, std::wstring const &match)
    : _input(input)
{
    _matches.emplace_back(match);
}

bool PropertyReplacer::replace_with(std::wstring const &replacement)
{
    auto start = _input.begin();
    auto end = _input.end();
    std::size_t offset = 0;
    for (auto matchIt = _matches.begin(); matchIt != _matches.end();)
    {
        std::match_results<decltype(start)> results;
        if (!std::regex_search(start + offset, end, results, *matchIt, std::regex_constants::format_first_only))
        {
            return false;
        }
        if (++matchIt == _matches.end())
        {
            _input.erase(offset + results.position(), results.length());
            _input.insert(offset + results.position(), replacement);
        }
        else
        {
            offset += results.position();
        }
    }
    return true;
}

PropertyReplacer &PropertyReplacer::then(std::wstring const &match)
{
    _matches.emplace_back(match);
    return *this;
}

PropertyReplacer PropertyReplacer::match(std::wstring &input, std::wstring const &match)
{
    return PropertyReplacer(input, match);
}

std::wstring replace_yaml_properties(
    std::wstring input,
    const property_retriever &propertyRetriever,
    std::vector<std::wstring> *failedToReplace)
{
    enum PropId
    {
        WxsKey,
        Regex,
        Replacement
    };
    for (auto prop : std::vector<std::tuple<std::wstring, std::wstring, formatter_func>>
    {
         {L"APIKEY",                            L"^[ #]*api_key:.*",                                        format_simple_value(L"api_key: ")},
         {L"SITE",                              L"^[ #]*site:.*",                                           format_simple_value(L"site: ")},
         {L"HOSTNAME",                          L"^[ #]*hostname:.*",                                       format_simple_value(L"hostname: ")},
         {L"LOGS_ENABLED",                      L"^[ #]*logs_config:.*",                                    simple_replace(L"logs_config:")},
         {L"LOGS_ENABLED",                      L"^[ #]*logs_enabled:.*",                                   format_simple_value(L"logs_enabled: ")},
         {L"LOGS_DD_URL",                       L"^[ #]*logs_config:.*",                                    simple_replace(L"logs_config:")},
         {L"LOGS_DD_URL",                       L"^[ #]*logs_dd_url:.*",                                    format_simple_value(L"  logs_dd_url: ")},
         {L"PROCESS_ENABLED",                   L"^[ #]*process_config:.*",                                 simple_replace(L"process_config:")},
         {L"PROCESS_DD_URL",                    L"^[ #]*process_config:.*",                                 format_simple_value(L"process_config:\n  process_dd_url: ")},
         {L"PROCESS_DISCOVERY_ENABLED",         L"^[ #]*process_config:.*",                                 simple_replace(L"process_config:")},
         {L"PROCESS_DISCOVERY_ENABLED",         L"^[ #]*process_discovery:.*",                              simple_replace(L"  process_discovery:")},
         {L"APM_ENABLED",                       L"^[ #]*apm_config:.*",                                     simple_replace(L"apm_config:")},
         {L"TRACE_DD_URL",                      L"^[ #]*apm_config:.*",                                     simple_replace(L"apm_config:")},
         {L"CMD_PORT",                          L"^[ #]*cmd_port:.*",                                       format_simple_value(L"cmd_port: ")},
         {L"DD_URL",                            L"^[ #]*dd_url:.*",                                         format_simple_value(L"dd_url: ")},
         {L"PYVER",                             L"^[ #]*python_version:.*",                                 format_simple_value(L"python_version: ")},
         {L"PROXY_HOST",                        L"^[ #]*proxy:.*",                                          format_proxy},
         {L"HOSTNAME_FQDN_ENABLED",             L"^[ #]*hostname_fqdn:.*",                                  format_simple_value(L"hostname_fqdn: ")},
         {L"TAGS",                              L"^[ #]*tags:(?:(?:.|\n)*?)^[ #]*- <TAG_KEY>:<TAG_VALUE>",  format_tags},
    })
    {
        auto propKey = std::get<WxsKey>(prop);
        auto propValue = propertyRetriever(propKey);

        if (propValue)
        {
            if (!PropertyReplacer::match(input, std::get<Regex>(prop)).replace_with(std::get<Replacement>(prop)(*propValue, propertyRetriever)))
            {
                if (failedToReplace != nullptr)
                {
                    failedToReplace->push_back(propKey);
                }
            }
        }
    }
    auto processEnabledProp = propertyRetriever(L"PROCESS_ENABLED");
    if (processEnabledProp)
    {
        std::wstring processEnabled = to_bool(*processEnabledProp) ? L"true" : L"false";
        auto p = PropertyReplacer::match(input, L"process_config:");
        if (!PropertyReplacer::match(input, L"process_config:")
                 .then(L"^[ #]*process_collection:.*")
                 .replace_with(L"  process_collection:") ||
            !PropertyReplacer::match(input, L"^[ #]*process_collection:.*")
                 .then(L"^[ #]*enabled:.*")
                 .replace_with(L"    enabled: " + processEnabled))
        {
            if (failedToReplace != nullptr)
            {
                failedToReplace->push_back(L"PROCESS_ENABLED");
            }
        }
    }

    auto processDiscoveryEnabledProp = propertyRetriever(L"PROCESS_DISCOVERY_ENABLED");
    if (processDiscoveryEnabledProp)
    {
        if (!PropertyReplacer::match(input, L"process_config:")
                 .then(L"^  process_discovery:.*")
                 .then(L"^[ #]*enabled:.*")
                 .replace_with(L"    enabled: " + *processDiscoveryEnabledProp))
        {
            if (failedToReplace != nullptr)
            {
                failedToReplace->push_back(L"PROCESS_DISCOVERY_ENABLED");
            }
        }
    }

    auto apmEnabled = propertyRetriever(L"APM_ENABLED");
    if (apmEnabled)
    {
        if (!PropertyReplacer::match(input, L"apm_config:")
                 .then(L"^[ #]*enabled:.*")
                 .replace_with(L"  enabled: " + *apmEnabled))
        {
            if (failedToReplace != nullptr)
            {
                failedToReplace->push_back(L"APM_ENABLED");
            }
        }
    }

    auto traceUrl = propertyRetriever(L"TRACE_DD_URL");
    if (traceUrl)
    {
        if (!PropertyReplacer::match(input, L"apm_config:")
                 .then(L"^[ #]*apm_dd_url:.*")
                 .replace_with(format_simple_value(L"  apm_dd_url: ")(*traceUrl, propertyRetriever)))
        {
            if (failedToReplace != nullptr)
            {
                failedToReplace->push_back(L"TRACE_DD_URL");
            }
        }
    }

    auto ec2UseWindowsPrefixDetection = propertyRetriever(L"EC2_USE_WINDOWS_PREFIX_DETECTION");
    if (ec2UseWindowsPrefixDetection)
    {
        if (!PropertyReplacer::match(input, L"^[ #]*ec2_use_windows_prefix_detection:.*")
                 .replace_with(format_simple_value(L"ec2_use_windows_prefix_detection: ")(*ec2UseWindowsPrefixDetection,
                                                                                          propertyRetriever)))
        {
            input.append(L"\nec2_use_windows_prefix_detection: " + *ec2UseWindowsPrefixDetection + L"\n");
        }
    }

    // Remove duplicated entries
    if (failedToReplace != nullptr)
    {
        std::sort(failedToReplace->begin(), failedToReplace->end());
        auto last = std::unique(failedToReplace->begin(), failedToReplace->end());
        failedToReplace->erase(last, failedToReplace->end());
    }

    return input;
}
