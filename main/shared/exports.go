package main

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

// XrayStart starts the singleton using a UTF-8 JSON configuration.
// Returns 0 on success, -1 on failure. NULL is an invalid configuration.
//
//export XrayStart
func XrayStart(config *C.char) C.int {
	return C.int(shared.startJSON(C.GoString(config)))
}

// XrayStartArgs accepts a UTF-8 JSON array of arguments, excluding argv[0].
// Returns 0 on success, -1 on failure.
//
//export XrayStartArgs
func XrayStartArgs(args *C.char) C.int {
	return C.int(shared.startArgs(C.GoString(args)))
}

// XrayStop stops the singleton. Stopping an idle library succeeds.
//
//export XrayStop
func XrayStop() C.int {
	return C.int(shared.stop())
}

// XrayLastError returns an owned UTF-8 copy; release it with XrayFreeString.
// The error is shared between callers and cleared by a successful operation.
//
//export XrayLastError
func XrayLastError() *C.char {
	return C.CString(shared.errorText())
}

// XrayFreeString releases a string returned by XrayLastError. NULL is allowed.
//
//export XrayFreeString
func XrayFreeString(value *C.char) {
	C.free(unsafe.Pointer(value))
}
