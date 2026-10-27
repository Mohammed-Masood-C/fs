// Package version reports the version of the running fs binary.
package version

import "runtime/debug"

// Version is set at build time:
//
//	go build -ldflags "-X fs/internal/version.Version=v1.0.0"
var Version = ""

// String returns the build version, falling back to the module version
// recorded by the Go toolchain and finally to "dev".
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
