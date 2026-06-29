package commands

// version is the running build version, wired from the cli package at startup
// so the commands layer can compare and report it (e.g. the update check)
// without importing cli, which would create an import cycle.
var version = "dev"

// SetVersion lets the cli package hand the build version to the commands layer.
func SetVersion(v string) { version = v }
