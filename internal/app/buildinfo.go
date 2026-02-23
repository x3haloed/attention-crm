package app

// These variables may be overridden at build time via -ldflags.
var (
	BuildVersion = "dev"
	BuildCommit  = ""
	BuildTime    = ""
)
