package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"fs/internal/certs"
	"fs/internal/version"
)

// printBanner tells the user what is being served, at which URLs, and with
// which certificate.
func printBanner(w io.Writer, dir string, addr net.Addr, cert certs.Info) {
	fmt.Fprintf(w, "\n  fs %s\n\n", version.String())
	fmt.Fprintf(w, "  Serving:  %s\n", dir)

	tcp, ok := addr.(*net.TCPAddr)
	switch {
	case !ok:
		fmt.Fprintf(w, "  Address:  %s\n", addr)
	case tcp.IP == nil || tcp.IP.IsUnspecified():
		fmt.Fprintf(w, "  Local:    %s\n", siteURL("localhost", tcp.Port))
		for _, ip := range lanIPs() {
			fmt.Fprintf(w, "  Network:  %s\n", siteURL(ip.String(), tcp.Port))
		}
	default:
		fmt.Fprintf(w, "  URL:      %s\n", siteURL(tcp.IP.String(), tcp.Port))
	}

	fmt.Fprintln(w)
	switch {
	case cert.Custom:
		fmt.Fprintf(w, "  Certificate: your own, from %s\n", cert.CertFile)
	case cert.CertFile == "":
		fmt.Fprint(w, "  Certificate: self-signed, kept in memory (a new one is made on every start)\n")
	default:
		state := "self-signed"
		if cert.Generated {
			state = "self-signed, newly created"
		}
		fmt.Fprintf(w, "  Certificate: %s, stored in %s\n", state, cert.CertFile)
		fmt.Fprint(w, "  Browsers show a warning until the certificate is trusted.\n")
		if runtime.GOOS == "windows" {
			fmt.Fprintf(w, "  To trust it on this PC (Edge, Chrome):  certutil -user -addstore Root \"%s\"\n", cert.CertFile)
		}
	}
	fmt.Fprint(w, "\n  Press Ctrl+C to stop.\n\n")
}

// siteURL formats an HTTPS base URL, leaving out the port if it is 443.
func siteURL(host string, port int) string {
	if port == 443 {
		if strings.Contains(host, ":") { // IPv6 literal
			host = "[" + host + "]"
		}
		return "https://" + host
	}
	return "https://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// certHosts returns the names the certificate must cover: localhost, this
// machine's name, its network addresses, and the bind address if specific.
func certHosts(bindHost string) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if name, err := os.Hostname(); err == nil && name != "" {
		hosts = append(hosts, strings.ToLower(name))
	}
	for _, ip := range lanIPs() {
		hosts = append(hosts, ip.String())
	}
	if bindHost != "" {
		if ip := net.ParseIP(bindHost); ip == nil || !ip.IsUnspecified() {
			hosts = append(hosts, bindHost)
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

// lanIPs returns the IPv4 addresses of the network interfaces that are up,
// which is where other machines on the network can reach the server.
func lanIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip := ipnet.IP.To4(); ip != nil && !ip.IsLinkLocalUnicast() {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}
