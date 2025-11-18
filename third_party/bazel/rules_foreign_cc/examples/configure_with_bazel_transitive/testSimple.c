#include <stdio.h>

#include "builtWithBazel.h"
#include "simple.h"

int main(int argc, char **argv) {
    printf("Call bazelSays() directly: %s\n", bazelSays());
    simpleFun();
    return 0;
}
