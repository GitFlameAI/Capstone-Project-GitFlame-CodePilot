package observability

import "runtime"

// Build identity, injected at link time:
//
//	go build -ldflags "-X gitflame-codepilot/backend/internal/observability.Commit=$(git rev-parse --short HEAD)"
//
// The values stay "unknown" for a plain `go build`, which is honest: a binary
// that cannot tell you what it was built from should say so rather than lie.
var (
	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// BuildInfo is what GET /version returns. Answering "what exactly is deployed"
// is the first step of every incident, and after handover nobody will have the
// context to answer it from memory.
type BuildInfo struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

func Build(service string) BuildInfo {
	return BuildInfo{
		Service:   service,
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}
