// Command fs is a static file server with a built-in file explorer. It serves
// exactly one folder, and nothing outside of it can be reached.
//
// Usage:
//
//	fs [options] <folder>
//
// Run "fs --help" for the list of options.
package main

import (
	"os"

	"fs/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
