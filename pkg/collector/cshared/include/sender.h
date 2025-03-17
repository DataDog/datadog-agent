#ifndef SENDER_H
#define SENDER_H

// sender

struct sender_s {
    void *handle;

    void (*gauge)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*count)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*rate)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*monotonic_count)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*histogram)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*historate)(void *handle, char *metric, double value, char *hostname, char **tags);
    void (*service_check)(void *handle, char *service, int status, char *hostname, char **tags, char *message);
    void (*commit)(void *handle);
    void (*event_platform_event)(void *handle, char *eventType, char *rawEvent);
};

typedef struct sender_s sender_t;

sender_t *new_sender(void *handle);

// sender manager

struct sender_manager_s {
    void *handle;

    char *(*get_sender)(void *handle, char *id, sender_t **ret_sender);
};

typedef struct sender_manager_s sender_manager_t;

sender_manager_t *new_sender_manager(void *handle);

#endif
