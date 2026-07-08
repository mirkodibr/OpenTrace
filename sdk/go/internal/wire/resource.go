package wire

// Resource holds host/process attributes captured once at SDK init and
// attached to every exported event. Capturing at init keeps syscalls off
// the hot path.
type Resource struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Hostname       string
	PID            int
}
