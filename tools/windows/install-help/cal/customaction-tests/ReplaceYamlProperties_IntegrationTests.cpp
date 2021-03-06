#include "stdafx.h"
#include "ReplaceYamlProperties_IntegrationTests.h"
#include "PropertyReplacer.h"

typedef std::map<std::wstring, std::wstring> value_map;
extern property_retriever propertyRetriever(value_map const &values);

TEST_F(ReplaceYamlPropertiesIntegrationTests, dd_agent_no_subservices)
{
    value_map values = {{L"APIKEY", L"testapikey"},
                        {L"LOGS_ENABLED", L"false"},
                        {L"LOGS_DD_URL", L"http://someurl.com"},
                        {L"PROCESS_ENABLED", L"false"},
                        {L"APM_ENABLED", L"false"}};
    std::wstring result = replace_yaml_properties(DatadogYaml, propertyRetriever(values));

    auto node = YAML::Load(std::string(result.begin(), result.end()));

    EXPECT_EQ(node["api_key"].as<std::string>(), "testapikey");

    // 'an Agent with APM disabled'
    EXPECT_TRUE(node["apm_config"].IsDefined());
    EXPECT_TRUE(node["apm_config"]["enabled"].IsDefined());
    EXPECT_EQ(node["apm_config"]["enabled"].as<std::string>(), "false");

    // 'an Agent with logs disabled'
    // There is no need to enable the logs config if the logs are disabled
    EXPECT_TRUE(node["logs_config"].IsDefined());
    EXPECT_TRUE(node["logs_enabled"].IsDefined());
    EXPECT_EQ(node["logs_enabled"].as<std::string>(), "false");

    // 'an Agent with process disabled'
    EXPECT_TRUE(node["process_config"].IsDefined());
    EXPECT_TRUE(node["process_config"]["enabled"].IsDefined());
    EXPECT_EQ(node["process_config"]["enabled"].as<std::string>(), "disabled");
}

TEST_F(ReplaceYamlPropertiesIntegrationTests, no_apikey_still_passes)
{
    value_map values = {};
    std::wstring result = replace_yaml_properties(DatadogYaml, propertyRetriever(values));

    auto node = YAML::Load(std::string(result.begin(), result.end()));

    EXPECT_TRUE(node["api_key"].IsDefined());
}
