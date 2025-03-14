#include <stdlib.h>
#include "check_wrapper.h"

char *_call_check_run(void *handle);
void _call_check_stop(void *handle);
void _call_check_cancel(void *handle);
char *_call_check_to_string(void *handle);
char *_call_check_loader(void *handle);
int64_t _call_check_interval(void *handle);
char *_call_check_id(void *handle);
char *_call_check_version(void *handle);
char *_call_check_configSource(void *handle);
bool _call_check_isTelemetryEnabled(void *handle);
char *_call_check_initConfig(void *handle);
char *_call_check_instanceConfig(void *handle);
bool _call_check_isHASupported(void *handle);
char *_call_check_configure(void *handle, sender_manager_t *senderManager, uint64_t integrationConfigDigest, char *config, char *initConfig, char *source);

c_check_wrapper_t *newCheckWrapper(void *handle) {
	c_check_wrapper_t *wrapper = malloc(sizeof(c_check_wrapper_t));

	wrapper->handle = handle;

	wrapper->run = _call_check_run;
	wrapper->stop = _call_check_stop;
	wrapper->cancel = _call_check_cancel;
	wrapper->to_string = _call_check_to_string;
	wrapper->loader = _call_check_loader;
	wrapper->configure = _call_check_configure;
	wrapper->interval = _call_check_interval;
	wrapper->id = _call_check_id;
	wrapper->version = _call_check_version;
	wrapper->configSource = _call_check_configSource;
	wrapper->isTelemetryEnabled = _call_check_isTelemetryEnabled;
	wrapper->initConfig = _call_check_initConfig;
	wrapper->instanceConfig = _call_check_instanceConfig;
	wrapper->isHASupported = _call_check_isHASupported;

	return wrapper;
}
