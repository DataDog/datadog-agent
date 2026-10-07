#include <stdio.h>
#include <string.h>

static void smash(size_t n) {
    char buf[16];
    memset(buf, 'A', n);
    printf("%c\n", buf[0]);
}

int main(int argc, char **argv) {
    (void)argv;
    // Derived from argc so that the overflow is only caught at run time.
    smash((size_t)argc * 64);
    return 0;
}
