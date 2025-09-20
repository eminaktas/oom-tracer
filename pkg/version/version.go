package version

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

var (
	// version is a constant representing the version tag or branch that
	// generated this build. It should be set during build via -ldflags.
	version string
	// buildDate in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	// It should be set during build via -ldflags.
	buildDate string
	// gitsha1 is a constant representing git sha1 for this build.
	// It should be set during build via -ldflags.
	gitsha1 string
)

// Info holds the information related to descheduler app version.
type Info struct {
	Major      string `json:"major"`
	Minor      string `json:"minor"`
	GitVersion string `json:"gitVersion"`
	GitSha1    string `json:"gitSha1"`
	BuildDate  string `json:"buildDate"`
	GoVersion  string `json:"goVersion"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	majorVersion, minorVersion := splitVersion(version)
	return Info{
		Major:      majorVersion,
		Minor:      minorVersion,
		GitVersion: version,
		GitSha1:    gitsha1,
		BuildDate:  buildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// splitVersion splits the git version to generate major and minor versions needed.
func splitVersion(version string) (string, string) {
	if version == "" {
		return "", ""
	}

	// Version from an automated container build environment for a tag. For example v0.18.0.
	m1, _ := regexp.MatchString(`^v\d+\.\d+\.\d+$`, version)

	// Version from an automated container build environment(not a tag) or a local build. For example v0.18.0-46-g939c1c0.
	m2, _ := regexp.MatchString(`^v\d+\.\d+\.\d+-\w+-\w+$`, version)

	if m1 || m2 {
		return strings.Trim(strings.Split(version, ".")[0], "v"), strings.Split(version, ".")[1] + "." + strings.Split(version, ".")[2]
	}

	// Something went wrong
	return "", ""
}
