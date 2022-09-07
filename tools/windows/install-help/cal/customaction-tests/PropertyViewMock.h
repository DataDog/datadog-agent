#pragma once

#include <gtest/gtest.h>

#include <PropertyView.h>

class TestPropertyView : public PropertyView
{
  public:
    TestPropertyView::TestPropertyView(std::wstring &data)
    {
        parseKeyValueString(data, this->values);
    }
};
