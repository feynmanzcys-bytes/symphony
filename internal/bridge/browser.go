package bridge

// BrowserCommand is the command that opens url in the default browser on goos. ok is false when there is no way
// to: on Linux and the like without a graphical session (no DISPLAY or WAYLAND_DISPLAY) or without xdg-open.
func BrowserCommand(goos, url string, getenv func(string) string, look LookPath) (argv []string, ok bool) {
	switch goos {
	case "windows":
		// hands the address to the default browser without a console window or cmd's quoting rules
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}, true
	case "darwin":
		return []string{"open", url}, true
	}
	if getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return nil, false
	}
	if _, err := look("xdg-open"); err != nil {
		return nil, false
	}
	return []string{"xdg-open", url}, true
}
