#ifndef SENDER_H
#define SENDER_H

struct sender_manager_s {
    void *handle;
};

typedef struct sender_manager_s sender_manager_t;

struct sender_s {
    void *handle;
};

typedef struct sender_s sender_t;

sender_manager_t *new_sender_manager(void *handle);

#endif
