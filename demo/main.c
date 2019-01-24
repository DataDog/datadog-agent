#include <stdio.h>

#include <datadog_agent_six.h>


static six_t *six2, *six3;

static six_pyobject_t *print_foo() {
    printf("I'm extending Python!\n\n");
    return get_none(six2);
}


int main(int argc, char *argv[]) {
    six2 = make2();
    if (!six2) {
        return 1;
    }

    add_module_func_noargs(six2, "my_module", "print_foo", print_foo);
    init(six2, NULL);
    printf("Embedding Python version %s\n", get_py_version(six2));
    printf("\n");

    run_any_file(six2, "./demo/main.py");

    six3 = make3();
    if (!six3) {
        return 1;
    }
    init(six3, NULL);
    printf("Embedding Python version %s\n", get_py_version(six3));
    printf("\n");

    destroy2(six2);
    destroy3(six3);
    printf("All cleaned up\n");
}
