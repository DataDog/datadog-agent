#ifndef __FILES_H
#define __FILES_H

#include <vector>
#include <string>

template <class T>
struct FileContent {
    std::string path;
    T content;
};

class MappedFiles {
public:
    static const std::vector<FileContent<const char *> > files;
};

#endif
