
// foo should be stripped, and also appear in the debug symbol outputsymbol output
int foo() {
    return 42;
}

int main(void) {
    return foo();
}
