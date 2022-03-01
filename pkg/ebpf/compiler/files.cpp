#include "files.h"

const std::vector<FileContent<const char *> > MappedFiles::files = {
    {
        "/virtual/lib/clang/include/stdarg.h",
#include "clang-stdarg.h"
    },
};
