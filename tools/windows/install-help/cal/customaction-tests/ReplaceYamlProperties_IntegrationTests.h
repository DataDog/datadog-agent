#pragma once

#include "precompiled/stdafx.h"
#include <yaml-cpp/yaml.h>
#include <fstream>
#include <filesystem>
#include "gtest/gtest.h"
#include "PropertyReplacer.h"

class ReplaceYamlPropertiesIntegrationTests : public testing::Test
{
  public:
    std::wstring DatadogYaml;

  protected:
    void SetUp() override
    {
        std::cout << "Executing tests in " << std::filesystem::current_path() << std::endl;
        std::wifstream inputConfigStream("datadog.yaml");
        EXPECT_TRUE(inputConfigStream.is_open());
        inputConfigStream.seekg(0, std::ios::end);
        const size_t fileSize = inputConfigStream.tellg();
        if (fileSize <= 0)
        {
            assert(false);
        }
        DatadogYaml.reserve(fileSize);
        inputConfigStream.seekg(0, std::ios::beg);

        DatadogYaml.assign(std::istreambuf_iterator<wchar_t>(inputConfigStream), std::istreambuf_iterator<wchar_t>());
    }
};
