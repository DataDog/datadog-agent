#include "customaction-tests.h"

#include "ReplaceYamlProperties.h"


property_retriever propertyRetriever(value_map const &values)
{
    return [values](std::wstring const &propertyName) -> std::optional<std::wstring>
    {
        auto it = values.find(propertyName);
        if (it != values.end())
        {
            return it->second;
        }
        return std::nullopt;
    };
}

std::wstring random_string(size_t length)
{
    srand(_time32(nullptr));
    auto randchar = []() -> wchar_t {
        const wchar_t charset[] = L"0123456789"
                               L"ABCDEFGHIJKLMNOPQRSTUVWXYZ"
                               L"abcdefghijklmnopqrstuvwxyz";
        const size_t max_index = sizeof(charset) / sizeof(wchar_t) - 1;
        return charset[rand() % max_index];
    };
    std::wstring str(length, 0);
    std::generate_n(str.begin(), length, randchar);
    return str;
}

TEST_F(ReplaceYamlPropertiesTests, When_APIKEY_Present_Replace_It)
{
    value_map values = {{L"APIKEY", L"1234567890"}};
    std::vector<std::wstring> failedToReplace;
    std::wstring result = replace_yaml_properties(LR"(
## @param api_key - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key:)",
                                                  propertyRetriever(values), &failedToReplace);

    EXPECT_EQ(result, LR"(
## @param api_key - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: 1234567890)");
    EXPECT_EQ(failedToReplace.size(), 0);
}

TEST_F(ReplaceYamlPropertiesTests, When_Property_Specified_But_Not_Replaced_Warn_Once)
{
    value_map values;

    // EC2_USE_WINDOWS_PREFIX_DETECTION always succeeds in being replaced since it's inserted in the file.
    std::vector<std::wstring> properties = {
        L"APIKEY",          L"SITE",           L"HOSTNAME",    L"LOGS_ENABLED",          L"LOGS_DD_URL",
        L"PROCESS_ENABLED", L"PROCESS_DD_URL", L"APM_ENABLED", L"TRACE_DD_URL",          L"CMD_PORT",
        L"DD_URL",          L"PYVER",          L"PROXY_HOST",  L"HOSTNAME_FQDN_ENABLED", L"TAGS",
    };

    for (auto propName : properties)
    {
        values[propName] = random_string(8);
    }
    std::vector<std::wstring> failedToReplace;
    std::wstring result = replace_yaml_properties(LR"(
# This is some random text
random_prop: true
)",
                                                  propertyRetriever(values), &failedToReplace);

    EXPECT_EQ(result, LR"(
# This is some random text
random_prop: true
)");
    std::vector<std::wstring> duplicates;
    std::sort(properties.begin(), properties.end());
    std::sort(failedToReplace.begin(), failedToReplace.end());
    std::set_difference(failedToReplace.begin(), failedToReplace.end(),
                        properties.begin(), properties.end(),
                        std::back_inserter(duplicates));
    // This will print the properties that are in duplicates if any
    EXPECT_EQ(duplicates, std::vector<std::wstring>());
}

TEST_F(ReplaceYamlPropertiesTests, When_EC2_USE_WINDOWS_PREFIX_DETECTION_Add_It)
{
    value_map values = {{L"EC2_USE_WINDOWS_PREFIX_DETECTION", L"true"}};
    std::wstring result = replace_yaml_properties(LR"()", propertyRetriever(values));

    EXPECT_EQ(result, LR"(
ec2_use_windows_prefix_detection: true
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_EC2_USE_WINDOWS_PREFIX_DETECTION_Already_Exists_Dont_Duplicate_it)
{
    value_map values = {{L"EC2_USE_WINDOWS_PREFIX_DETECTION", L"true"}};
    std::wstring result = replace_yaml_properties(LR"(
ec2_use_windows_prefix_detection: false
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
ec2_use_windows_prefix_detection: true
)");
}
