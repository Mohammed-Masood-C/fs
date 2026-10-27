package services

import (
	"syscall"
	"time"
)

// Windows constants that package syscall does not export.
const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// openProcess opens process pid with the given access rights, but only if it
// is still running and, when created is not zero, is the same process that
// was recorded rather than a new one that reused the PID.
func openProcess(pid int, created int64, access uint32) (syscall.Handle, bool) {
	h, err := syscall.OpenProcess(access|processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return 0, false
	}
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil || code != stillActive ||
		(created != 0 && creationTime(h) != created) {
		syscall.CloseHandle(h)
		return 0, false
	}
	return h, true
}

func creationTime(h syscall.Handle) int64 {
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return creation.Nanoseconds()
}

// SelfCreated returns the creation time of the current process.
func SelfCreated() int64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	return creationTime(h)
}

// ProcessAlive reports whether the recorded process is still running.
func ProcessAlive(pid int, created int64) bool {
	h, ok := openProcess(pid, created, 0)
	if ok {
		syscall.CloseHandle(h)
	}
	return ok
}

// WaitExit waits up to timeout for the process to exit and reports whether
// it did.
func WaitExit(pid int, created int64, timeout time.Duration) bool {
	h, ok := openProcess(pid, created, syscall.SYNCHRONIZE)
	if !ok {
		return true
	}
	defer syscall.CloseHandle(h)
	event, err := syscall.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	return err == nil && event == syscall.WAIT_OBJECT_0
}

// Kill terminates the process immediately.
func Kill(pid int, created int64) error {
	h, ok := openProcess(pid, created, syscall.PROCESS_TERMINATE)
	if !ok {
		return nil
	}
	defer syscall.CloseHandle(h)
	return syscall.TerminateProcess(h, 1)
}
