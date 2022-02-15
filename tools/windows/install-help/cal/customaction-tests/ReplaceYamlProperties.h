#pragma once

#include <string>
#include <map>
#include <gtest/gtest.h>
#include "PropertyReplacer.h"

class ReplaceYamlPropertiesTests : public testing::Test
{
};

typedef std::map<std::wstring, std::wstring> value_map;
property_retriever propertyRetriever(value_map const &values);
