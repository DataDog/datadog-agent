#include "files.h"

const std::map<std::string, const char *> MappedFiles::files = {
  {
    "/virtual/lib/clang/include/stdarg.h",
    #include "clang-stdarg.h"
  },
};
