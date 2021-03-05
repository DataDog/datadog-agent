#pragma once

#include "stdafx.h"
#include "PropertyReplacer.h"
#include "gtest/gtest.h"

class ReplaceYamlPropertiesTests : public testing::Test
{
};

typedef std::map<std::wstring, std::wstring> value_map;
property_retriever propertyRetriever(value_map const &values);
