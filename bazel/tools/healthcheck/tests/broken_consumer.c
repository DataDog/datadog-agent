extern int dummybad(void);

int broken_consumer(void) {
    return dummybad() + 1;
}
