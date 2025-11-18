#include "lib.h"

#include <iostream>

#ifndef TRIGGER_ERROR
#error This value must be defined
#endif

#if TRIGGER_ERROR
#error This error intentionally prevents this library from compiling
#endif

void hello_world() { std::cout << "Hello world!" << std::endl; }
