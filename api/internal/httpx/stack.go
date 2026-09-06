package httpx

import "runtime"

// stack captures a bounded stack trace for panic logging.
func stack() string {
	buf := make([]byte, 8<<10)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
