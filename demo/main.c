// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <stdio.h>
#include <stdlib.h>

#include <datadog_agent_six.h>


static six_t *six2, *six3;

static six_pyobject_t *print_foo() {
    printf("I'm extending Python!\n\n");
    return get_none(six2);
}

char* read_file(const char* path)
{
    FILE *f = fopen(path, "rb");
    fseek(f, 0, SEEK_END);
    long fsize = ftell(f);
    fseek(f, 0, SEEK_SET);

    char *string = malloc(fsize + 1);
    long read = fread(string, fsize, 1, f);
    if (read < 1) {
        fprintf(stderr, "Error reading file!\n");
    }
    fclose(f);

    string[fsize] = 0;

    return string;
}

int main(int argc, char *argv[]) {
    six2 = make2();
    if (!six2) {
        return 1;
    }

    add_module_func(six2, DATADOG_AGENT_SIX_DATADOG_AGENT, DATADOG_AGENT_SIX_NOARGS,
                    "print_foo", print_foo);
    init(six2, NULL);
    printf("Embedding Python version %s\n", get_py_version(six2));
    printf("\n");

    char *code = read_file("./demo/main.py");
    run_simple_string(six2, code);
    free(code);

    six3 = make3();
    if (!six3) {
        return 1;
    }
    init(six3, NULL);
    printf("Embedding Python version %s\n", get_py_version(six3));
    printf("\n");

    printf("Also embedded Python version %s\n", get_py_version(six2));
    printf("\n");

    destroy2(six2);
    destroy3(six3);
    printf("All cleaned up\n");
}
