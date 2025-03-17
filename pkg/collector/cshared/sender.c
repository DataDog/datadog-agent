#include <stdlib.h>
#include "sender.h"

char *_call_sender_manager_get_sender(void *handle, char *id, sender_t **ret_sender);

sender_manager_t *new_sender_manager(void *handle) {
	sender_manager_t *manager = malloc(sizeof(sender_manager_t));

	manager->handle = handle;

	manager->get_sender = _call_sender_manager_get_sender;

	return manager;
}

sender_t *new_sender(void *handle) {
	sender_t *sender = malloc(sizeof(sender_t));

	sender->handle = handle;

	return sender;
}
