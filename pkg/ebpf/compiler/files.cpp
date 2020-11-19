#include "files.h"

std::map<std::string, const char *> MappedFiles::files_ = {
  {
    "/virtual/lib/clang/include/stdarg.h",
    #include "clang-stdarg.h"
  },
};
