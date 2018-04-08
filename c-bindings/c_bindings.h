#ifndef LIBSYNCTHING_INTERNAL_H
#define LIBSYNCTHING_INTERNAL_H

#include <stddef.h>

// allow registration of callback function
typedef void (*libst_logging_callback_function_t) (int logLevel, const char *msg, size_t msgSize);
extern libst_logging_callback_function_t libst_logging_callback_function;
extern void libst_invoke_logging_callback(int log_level, const char *message, size_t message_size);

#endif // LIBSYNCTHING_INTERNAL_H
