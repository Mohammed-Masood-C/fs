package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"

	"fs/internal/certs"
	"fs/internal/services"
	"fs/internal/version"
)

// printBanner tells the user what is being served, at which URLs, and with
// which certificate:
//
//	fs v2.0.0  service #1
//
//	→ Local:    https://localhost:8080
//	→ Network:  https://192.168.1.20:8080
//
//	  Folder    C:\www
//	  Login     admin:secret
//	  Index     on
//	  ...
func printBanner(w io.Writer, rec *services.Record, addr net.Addr, cert certs.Info) {
	p := newPainter(w)
	fmt.Fprintf(w, "\n  %s %s  %s\n\n", p.bold(p.cyan("fs")), p.dim(version.String()), p.dim(fmt.Sprintf("service #%d", rec.ID)))

	url := func(label, u string) {
		fmt.Fprintf(w, "  %s %s %s\n", p.green("→"), p.bold(fmt.Sprintf("%-9s", label+":")), p.cyan(u))
	}
	tcp, ok := addr.(*net.TCPAddr)
	switch {
	case !ok:
		url("Address", addr.String())
	case tcp.IP == nil || tcp.IP.IsUnspecified():
		url("Local", siteURL("localhost", tcp.Port))
		for _, ip := range lanIPs() {
			url("Network", siteURL(ip.String(), tcp.Port))
		}
	default:
		url("URL", siteURL(tcp.IP.String(), tcp.Port))
	}
	fmt.Fprintln(w)

	field := func(label, value string) {
		fmt.Fprintf(w, "    %s %s\n", p.dim(fmt.Sprintf("%-11s", label)), value)
	}
	field("Folder", rec.Folder)
	if rec.User != "" {
		field("Login", rec.User+":"+rec.Password)
	} else {
		field("Login", p.dim("none (anyone who can connect has access)"))
	}
	if rec.NoIndex {
		field("Index", "off (always the file explorer)")
	} else {
		field("Index", "on (index.html is served as a website)")
	}
	if rec.Verbose {
		field("Logging", "every request")
	} else {
		field("Logging", "warnings and errors")
	}

	switch {
	case cert.Custom:
		field("Certificate", "your own, from "+cert.CertFile)
	case cert.CertFile == "":
		field("Certificate", "self-signed, temporary (a new one is made on every start)")
	default:
		state := "self-signed"
		if cert.Generated {
			state = "self-signed, newly created"
		}
		field("Certificate", state)
		fmt.Fprintf(w, "    %s %s\n", strings.Repeat(" ", 11),
			p.dim(`Trust it once to avoid browser warnings: certutil -user -addstore Root "`+cert.CertFile+`"`))
	}

	if rec.Background {
		fmt.Fprintf(w, "\n  %s\n\n", p.dim(fmt.Sprintf("Stop with: fs stop %d", rec.ID)))
	} else {
		fmt.Fprintf(w, "\n  %s\n\n", p.dim(fmt.Sprintf("Press Ctrl+C to stop, or run: fs stop %d", rec.ID)))
	}
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
