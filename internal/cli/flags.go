package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

const usage = `fs - serve a folder over HTTP, with a built-in file explorer.

Usage:
  fs [options] <folder>

Only <folder> and its contents are accessible. Nothing outside of it can be
reached, not even through symbolic links.

fs always uses HTTPS, with a self-signed certificate it creates on first
start. Plain http:// requests on the same port are redirected to https://.

Examples:
  fs ./site                          serve ./site on https://localhost:80
  fs --port 443 C:\www               serve C:\www on https://localhost

Options:
  -p, --port <port>          port to listen on; 0 picks a free port (default 80)
      --host <address>       address to bind to (default: all interfaces)
      --no-index             show the explorer even if a folder has an index.html
      --auth <user:pass>     require HTTP basic authentication (default: $FS_AUTH)
      --verbose              log every request
  -v, --version              print the version and exit
  -h, --help                 print this help and exit
`

// options holds the parsed command line.
type options struct {
	folder  string
	port    int
	host    string
	noIndex bool
	auth    string
	verbose bool
	version bool
	help    bool
}

// parseFlags parses and validates the command line. Unlike the flag package's
// default behaviour, options may appear both before and after the folder.
func parseFlags(args []string) (*options, error) {
	o := &options{}
	set := flag.NewFlagSet("fs", flag.ContinueOnError)
	set.SetOutput(io.Discard) // errors are reported by the caller

	set.IntVar(&o.port, "port", 80, "")
	set.IntVar(&o.port, "p", 80, "")
	set.StringVar(&o.host, "host", "", "")
	set.BoolVar(&o.noIndex, "no-index", false, "")
	set.StringVar(&o.auth, "auth", os.Getenv("FS_AUTH"), "")
	set.BoolVar(&o.verbose, "verbose", false, "")
	set.BoolVar(&o.version, "version", false, "")
	set.BoolVar(&o.version, "v", false, "")
	set.BoolVar(&o.help, "help", false, "")
	set.BoolVar(&o.help, "h", false, "")

	positional, err := parseInterleaved(set, args)
	if err != nil {
		return nil, err
	}
	if o.help || o.version {
		return o, nil
	}
	if err := o.validate(positional); err != nil {
		return nil, err
	}
	return o, nil
}

// parseInterleaved parses flags that may be mixed with positional arguments
// and returns the positional ones in order.
func parseInterleaved(set *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := set.Parse(args); err != nil {
			return nil, err
		}
		rest := set.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		// Parse stops at the first non-flag argument, or right after a "--"
		// terminator, in which case everything that follows is positional.
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func (o *options) validate(positional []string) error {
	switch len(positional) {
	case 0:
		return fmt.Errorf("missing the folder to serve")
	case 1:
		o.folder = positional[0]
	default:
		return fmt.Errorf("expected exactly one folder, got %d arguments: %s",
			len(positional), strings.Join(positional, " "))
	}
	if runtime.GOOS == "windows" {
		// Windows argument parsing turns a quoted path that ends in a backslash,
		// such as "C:\www\", into C:\www" because \" is an escaped quote.
		o.folder = strings.TrimSuffix(o.folder, `"`)
	}

	if o.port < 0 || o.port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 0 and 65535", o.port)
	}
	// Accept bracketed IPv6 literals such as [::1].
	o.host = strings.TrimSuffix(strings.TrimPrefix(o.host, "["), "]")

	if o.auth != "" {
		if user, pass, ok := strings.Cut(o.auth, ":"); !ok || user == "" || pass == "" {
			return fmt.Errorf("--auth must have the form user:password")
		}
	}
	return nil
}
