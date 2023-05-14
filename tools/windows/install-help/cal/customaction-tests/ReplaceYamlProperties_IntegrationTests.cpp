#include "precompiled/stdafx.h"
#include <yaml-cpp/yaml.h>
#include <fstream>
#include <filesystem>
#include "gtest/gtest.h"
#include "PropertyReplacer.h"
#include "ReplaceYamlProperties.h"
#include "ReplaceYamlProperties_IntegrationTests.h"


TEST_F(ReplaceYamlPropertiesIntegrationTests, dd_agent_installopts_spec)
{
    value_map values =
    {
        {L"APIKEY", L"testapikey"},
        {L"TAGS", L"k1:v1,k2:v2"},
        {L"CMD_PORT", L"4999"},
        {L"PROXY_HOST", L"proxy.foo.com"},
        {L"PROXY_PORT", L"1234"},
        {L"PROXY_USER", L"puser"},
        {L"PROXY_PASSWORD", L"ppass"},
        {L"SITE", L"eu"},
        {L"DD_URL", L"https://someurl.datadoghq.com"},
        {L"LOGS_DD_URL", L"https://logs.someurl.datadoghq.com"},
        {L"PROCESS_DD_URL", L"https://process.someurl.datadoghq.com"},
        {L"TRACE_DD_URL", L"https://trace.someurl.datadoghq.com"},
    };
    std::wstring result = replace_yaml_properties(DatadogYaml, propertyRetriever(values));

    auto node = YAML::Load(std::string(result.begin(), result.end()));

    EXPECT_EQ(node["api_key"].as<std::string>(), "testapikey");

    // 'has tags set
    EXPECT_TRUE(node["tags"].IsDefined());
    EXPECT_TRUE(node["tags"].IsSequence());
    auto tags = node["tags"].as<std::vector<std::string>>();
    const auto expectedTags = std::vector<std::string>{"k1:v1", "k2:v2"};
    EXPECT_EQ(tags, expectedTags);

    // 'has CMDPORT set'
    EXPECT_TRUE(node["cmd_port"].IsDefined());
    EXPECT_EQ(node["cmd_port"].as<int>(), 4999);

    // 'has proxy settings'
    EXPECT_TRUE(node["proxy"].IsDefined());
    EXPECT_EQ(node["proxy"]["https"].as<std::string>(), "http://puser:ppass@proxy.foo.com:1234");

    // 'has site settings'
    EXPECT_TRUE(node["site"].IsDefined());
    EXPECT_EQ(node["site"].as<std::string>(), "eu");

    EXPECT_TRUE(node["dd_url"].IsDefined());
    EXPECT_EQ(node["dd_url"].as<std::string>(), "https://someurl.datadoghq.com");

    EXPECT_TRUE(node["logs_config"].IsDefined());
    EXPECT_TRUE(node["logs_config"]["logs_dd_url"].IsDefined());
    EXPECT_EQ(node["logs_config"]["logs_dd_url"].as<std::string>(), "https://logs.someurl.datadoghq.com");

    EXPECT_TRUE(node["process_config"].IsDefined());
    EXPECT_TRUE(node["process_config"]["process_dd_url"].IsDefined());
    EXPECT_EQ(node["process_config"]["process_dd_url"].as<std::string>(), "https://process.someurl.datadoghq.com");

    EXPECT_TRUE(node["apm_config"].IsDefined());
    EXPECT_TRUE(node["apm_config"]["apm_dd_url"].IsDefined());
    EXPECT_EQ(node["apm_config"]["apm_dd_url"].as<std::string>(), "https://trace.someurl.datadoghq.com");
}

TEST_F(ReplaceYamlPropertiesIntegrationTests, dd_agent_no_subservices)
{
    value_map values = {{L"APIKEY", L"testapikey"},
                        {L"LOGS_ENABLED", L"false"},
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
    // EXPECT_TRUE(node["logs_config"].IsDefined());
    EXPECT_TRUE(node["logs_enabled"].IsDefined());
    EXPECT_EQ(node["logs_enabled"].as<std::string>(), "false");

    // 'an Agent with process disabled'
    EXPECT_TRUE(node["process_config"].IsDefined());
    EXPECT_FALSE(node["process_config"]["enabled"].IsDefined());
    EXPECT_TRUE(node["process_config"]["process_collection"].IsDefined());
    EXPECT_TRUE(node["process_config"]["process_collection"]["enabled"].IsDefined());
    EXPECT_EQ(node["process_config"]["process_collection"]["enabled"].as<std::string>(), "false");
}
