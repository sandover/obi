package app

import (
	"runtime/debug"
)

var (
	semver = "0.2.0"
	gitSHA string
)

// Version returns the semantic version plus the current git SHA (if known).
// Override via -ldflags "-X github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/app.semver=x.y.z -X github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/app.gitSHA=abcdef1".
func Version() string {
	if sha := resolveGitSHA(); sha != "" {
		return semver + " (" + sha + ")"
	}
	return semver
}

func resolveGitSHA() string {
	if gitSHA != "" {
		return shortSHA(gitSHA)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return shortSHA(setting.Value)
		}
	}
	return ""
}

func shortSHA(val string) string {
	if len(val) >= 7 {
		return val[:7]
	}
	return val
}
