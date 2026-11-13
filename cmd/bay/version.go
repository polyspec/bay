package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/polyspec/bay/internal/contract"
)

// buildVersion is the release version, set when a release is built:
// -ldflags "-X main.buildVersion=v0.1.0". Without it the module version from
// "go install ...@version", or the commit of a local build, is printed.
var buildVersion = ""

// versionInfo is what "bay version" prints.
type versionInfo struct {
	Version       string `json:"version"`
	Commit        string `json:"commit,omitempty"`
	Modified      bool   `json:"modified,omitempty"`
	Go            string `json:"go"`
	Platform      string `json:"platform"`
	SchemaVersion int    `json:"schemaVersion"`
}

func readVersion() versionInfo {
	v := versionInfo{Version: buildVersion, Go: runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH, SchemaVersion: contract.SchemaVersion}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v.Version == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v.Version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				v.Commit = s.Value
			case "vcs.modified":
				v.Modified = s.Value == "true"
			}
		}
	}
	if v.Version == "" {
		v.Version = "devel"
	}
	return v
}

// version prints the version of this executable.
func version(args []string) error {
	v := readVersion()
	if len(args) > 0 && (args[0] == "--json" || args[0] == "-json") {
		return json.NewEncoder(os.Stdout).Encode(v)
	}
	line := "bay " + v.Version
	if v.Commit != "" {
		line += " (" + v.Commit[:min(12, len(v.Commit))]
		if v.Modified {
			line += ", modified"
		}
		line += ")"
	}
	fmt.Printf("%s %s %s schema %d\n", line, v.Go, v.Platform, v.SchemaVersion)
	return nil
}
