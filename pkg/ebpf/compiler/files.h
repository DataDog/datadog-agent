#ifndef __FILES_H
#define __FILES_H

#include <map>
#include <string>

class MappedFiles {
  static std::map<std::string, const char *> files_;
 public:
  static const std::map<std::string, const char *> & files() { return files_; }
};

#endif
