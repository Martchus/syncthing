#include "c_bindings.h"

libst_logging_callback_function_t libst_logging_callback_function = NULL;

void libst_invoke_logging_callback(int log_level, const char *message, size_t message_size)
{
	if (!libst_logging_callback_function) {
		return;
	}
	libst_logging_callback_function(log_level, message, message_size);
}
