#include <stdio.h>
#include <string.h>
#include "xray.h"

static int check_status(int actual, int expected, const char *operation) {
	if (actual == expected) {
		return 0;
	}
	char *error = XrayLastError();
	fprintf(stderr, "%s: status=%d, error=%s\n", operation, actual, error);
	XrayFreeString(error);
	return 1;
}

int main(void) {
	if (check_status(XrayStop(), 0, "stop idle")) return 1;
	if (check_status(XrayStart(NULL), -1, "reject NULL config")) return 1;
	char *saved_error = XrayLastError();
	if (!saved_error || !*saved_error) return 1;
	if (check_status(XrayStartArgs(NULL), -1, "reject NULL args")) return 1;
	if (check_status(XrayStartArgs("[\"--unknown\"]"), -1, "reject unknown flag")) return 1;
	if (check_status(XrayStartArgs("[\"--set\",\"outbounds[0].protocol=freedom\"]"), 0, "start args")) return 1;
	if (check_status(XrayStart("{}"), -1, "reject duplicate start")) return 1;
	if (check_status(XrayStop(), 0, "stop")) return 1;
	if (!*saved_error) return 1;
	XrayFreeString(saved_error);
	char *error = XrayLastError();
	int has_error = strcmp(error, "") != 0;
	XrayFreeString(error);
	XrayFreeString(NULL);
	if (has_error) return 1;
	if (check_status(XrayStart("{\"outbounds\":[{\"protocol\":\"freedom\"}]}"), 0, "restart JSON")) return 1;
	if (check_status(XrayStop(), 0, "stop restarted instance")) return 1;
	if (check_status(XrayStop(), 0, "repeat stop")) return 1;
	puts("Xray C ABI smoke test passed.");
	return 0;
}
