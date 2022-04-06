#ifndef _CONSTANTS_H
#define _CONSTANTS_H

#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" : "=r"(var))

#endif
