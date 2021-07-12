#ifndef __FILES_H
#define __FILES_H

#include <map>
#include <string>

class MappedFiles {
public:
  static const std::map<std::string, const char *> files;
};

#endif
