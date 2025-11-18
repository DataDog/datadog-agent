#include "simple.h"

#include <stdio.h>

#include "builtWithBazel.h"

void simpleFun(void) { printf("simpleFun: %s", bazelSays()); }
