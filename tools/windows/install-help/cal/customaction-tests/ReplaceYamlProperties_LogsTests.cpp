#include "precompiled/stdafx.h"
#include "ReplaceYamlProperties.h"

TEST_F(ReplaceYamlPropertiesTests, When_Logs_Enabled_Correctly_Replace)
{
    value_map values = {
        {L"LOGS_ENABLED", L"true"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# logs_enabled: false

# logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
logs_enabled: true

logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)");
}

TEST_F(ReplaceYamlPropertiesTests, When_Logs_Disabled_Correctly_Replace)
{
    value_map values = {
        {L"LOGS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# logs_enabled: false

# logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
logs_enabled: false

logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)");
}

TEST_F(ReplaceYamlPropertiesTests, Always_Set_logs_dd_url)
{
    value_map values = {
        {L"LOGS_DD_URL", L"https://logs.someurl.datadoghq.com:8443"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# logs_enabled: false

# logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
# logs_enabled: false

logs_config:

  logs_dd_url: https://logs.someurl.datadoghq.com:8443
)");
}


TEST_F(ReplaceYamlPropertiesTests, When_Logs_Enabled_And_Logs_Url_Specified_Correctly_Replace)
{
    value_map values = {
        {L"LOGS_DD_URL", L"https://logs.someurl.datadoghq.com:8443"},
        {L"LOGS_ENABLED", L"false"},
    };
    std::wstring result = replace_yaml_properties(LR"(
# logs_enabled: false

# logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
)",
                                                  propertyRetriever(values));

    EXPECT_EQ(result, LR"(
logs_enabled: false

logs_config:

  logs_dd_url: https://logs.someurl.datadoghq.com:8443
)");
}
