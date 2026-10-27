package server

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"syscall"
)

// Windows Sockets error codes, which package syscall does not export.
const (
	wsaeacces        syscall.Errno = 10013
	wsaeaddrinuse    syscall.Errno = 10048
	wsaeaddrnotavail syscall.Errno = 10049
)

// listen binds a TCP listener to addr and turns the most common bind
// failures into actionable messages.
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, explainListenError(addr, err)
	}
	return ln, nil
}

func explainListenError(addr string, err error) error {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	_, port, _ := net.SplitHostPort(addr)

	switch {
	case errno == syscall.EADDRINUSE || errno == wsaeaddrinuse:
		return fmt.Errorf("listen on %s: port %s is already in use by another program\n"+
			"  hint: stop that program or pick another port, e.g. --port 8080", addr, port)

	case errno == syscall.EACCES || errno == wsaeacces:
		if runtime.GOOS == "windows" {
			return fmt.Errorf("listen on %s: access denied\n"+
				"  hint: port %s may be reserved by Windows or held by a system service; check\n"+
				"        'netsh interface ipv4 show excludedportrange protocol=tcp' or use --port 8080", addr, port)
		}
		return fmt.Errorf("listen on %s: permission denied\n"+
			"  hint: ports below 1024 need elevated privileges; run as root, grant\n"+
			"        CAP_NET_BIND_SERVICE, or use an unprivileged port such as --port 8080", addr)

	case errno == syscall.EADDRNOTAVAIL || errno == wsaeaddrnotavail:
		return fmt.Errorf("listen on %s: address is not available on this machine\n"+
			"  hint: check the value of --host", addr)
	}
	return fmt.Errorf("listen on %s: %w", addr, err)
}
