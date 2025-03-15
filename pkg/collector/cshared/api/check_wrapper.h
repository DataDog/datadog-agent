#ifndef CHECK_WRAPPER_H
#define CHECK_WRAPPER_H

#include <stdbool.h>
#include <stdint.h>
#include "sender.h"

struct c_check_wrapper_s {
	void *handle;

	char *(*run)(void *handle);
	void (*stop)(void *handle);
	void (*cancel)(void *handle);
	char *(*to_string)(void *handle);
	char *(*loader)(void *handle);
	char *(*configure)(void *handle, sender_manager_t *senderManager, uint64_t integrationConfigDigest, char *config, char *initConfig, char *source);
	int64_t (*interval)(void *handle);
	char *(*id)(void *handle);
	//void (*getWarnings)(void *handle, void *warnings, int *warningsLen);
	//void (*getSenderStats)(void *handle, void *stats);
	char *(*version)(void *handle);
	char *(*configSource)(void *handle);
	bool (*isTelemetryEnabled)(void *handle);
	char *(*initConfig)(void *handle);
	char *(*instanceConfig)(void *handle);
	//void (*getDiagnoses)(void *handle, void *diagnoses, int *diagnosesLen);
	bool (*isHASupported)(void *handle);
};

typedef struct c_check_wrapper_s c_check_wrapper_t;

c_check_wrapper_t *newCheckWrapper(void *handle);

#endif
