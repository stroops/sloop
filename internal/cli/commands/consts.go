package commands

// appName is sloop's public command name — used when building command strings
// that the user or tmux will invoke (hook commands, the popup keybind). It is
// the fixed installed binary name, kept in one place rather than sprinkled.
const appName = "sloop"

// fallbackTool is the default tool when none is detected or specified.
const fallbackTool = "claude"
