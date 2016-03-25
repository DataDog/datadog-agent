#ifndef _CHECKS_H
#define _CHECKS_H

#define Py_LIMITED_API
#include <Python.h>

void get_checks(char **checks);
void run_check(char *name);

#endif
