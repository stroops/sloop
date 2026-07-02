package tmux

// Send delivers a prompt to a running session without attaching. It is
// non-invasive: send-keys types into your own tmux pane exactly as if you did
// so at the keyboard, sloop never injects into the provider's process or API.

// BuildSendTextArgs types msg literally into a session's active pane. The
// -l flag sends the bytes verbatim (no key-name lookup) and -- ends option
// parsing so a message starting with "-" is still treated as text.
func BuildSendTextArgs(session, msg string) []string {
	return []string{"send-keys", "-t", ExactPane(session), "-l", "--", msg}
}

// BuildSendEnterArgs presses Enter in a session's active pane, submitting
// whatever text precedes it.
func BuildSendEnterArgs(session string) []string {
	return []string{"send-keys", "-t", ExactPane(session), "Enter"}
}

// LaunchSend types msg into the session and submits it with Enter.
func LaunchSend(session, msg string) error {
	if err := Run(BuildSendTextArgs(session, msg)...); err != nil {
		return err
	}
	return Run(BuildSendEnterArgs(session)...)
}
