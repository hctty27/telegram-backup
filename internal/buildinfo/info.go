// Package buildinfo describes the binary actually being executed.
package buildinfo

import "runtime"

// These strings are stamped by the Go linker for published images.
var (
 Version = "local"
 Commit = "unknown"
 BuildTime = ""
)

type Info struct {
 Service string `json:"service"`
 Version string `json:"version"`
 Commit string `json:"commit"`
 BuildTime string `json:"build_time"`
 GoVersion string `json:"go_version"`
 Platform string `json:"platform"`
}

func Current() Info {
 return Info{
  Service: "telegram-backup",
  Version: Version,
  Commit: Commit,
  BuildTime: BuildTime,
  GoVersion: runtime.Version(),
  Platform: runtime.GOOS + "/" + runtime.GOARCH,
 }
}
