/*
 * metrics.h - C library for UDS metrics writing
 */

#ifndef METRICS_H
#define METRICS_H

#ifdef __cplusplus
extern "C" {
#endif

int init_metrics(const char* socket_path);
int write_metrics(void);
void close_metrics(void);

#ifdef __cplusplus
}
#endif

#endif /* METRICS_H */
