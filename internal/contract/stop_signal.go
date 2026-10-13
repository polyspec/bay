package contract

import "strings"

// StopSignals are the signal names a service's stop_signal may name. Each name
// is accepted only with its SIG prefix and in upper case, as written here.
func StopSignals() []string {
	return []string{"SIGHUP", "SIGINT", "SIGQUIT", "SIGTERM", "SIGUSR1", "SIGUSR2", "SIGKILL"}
}

// stopSignalDescription states the accepted names for the schema.
func stopSignalDescription() string {
	return "Signal sent to stop the container: one of " + strings.Join(StopSignals(), ", ") +
		". Stop, restart, down and replacement pass it to the runtime as --signal."
}
