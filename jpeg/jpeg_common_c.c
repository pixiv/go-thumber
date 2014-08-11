#include "_cgo_export.h"

void error_panic(j_common_ptr cinfo) {
	struct { const char *p; } a;
	char buffer[JMSG_LENGTH_MAX];
	(*cinfo->err->format_message) (cinfo, buffer);
	goPanic(buffer);
}
