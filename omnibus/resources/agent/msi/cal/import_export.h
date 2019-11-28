#pragma once

#ifdef CA_EXPORT_SYMBOLS
#  define CA_API __declspec(dllexport)
#else
#  define CA_API __declspec(dllimport)
#endif
