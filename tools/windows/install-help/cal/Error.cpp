#include "stdafx.h"

std::wstring FormatErrorMessage(DWORD error)
{
    std::wstring buf;
    buf.resize(1024);
    FormatMessage(FORMAT_MESSAGE_FROM_SYSTEM, nullptr, error, 0, buf.data(), 1023, nullptr);
    std::wstringstream sstream;
    buf.erase(std::remove(buf.begin(), buf.end(), '\n'), buf.end());
    buf.erase(std::remove(buf.begin(), buf.end(), '\r'), buf.end());
    sstream << buf.data() << L" (" << std::hex << L"0x" << error << L")" << std::endl;
    return sstream.str();
}
