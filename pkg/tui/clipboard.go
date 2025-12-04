package tui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard copies text to the system clipboard.
// It supports Linux (xclip, xsel, wl-copy) and macOS (pbcopy).
func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// If DISPLAY is set, we're likely in an X11 or XWayland context
		// (e.g., X11 terminals like Terminator running on Wayland).
		// In this case, prefer xclip which works with X11 clipboard.
		// Only use wl-copy for native Wayland terminals without X11.
		hasDisplay := os.Getenv("DISPLAY") != ""
		hasWayland := os.Getenv("WAYLAND_DISPLAY") != ""

		if hasDisplay {
			// X11 or XWayland - use xclip/xsel
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			} else if _, err := exec.LookPath("xsel"); err == nil {
				cmd = exec.Command("xsel", "--clipboard", "--input")
			}
		} else if hasWayland {
			// Native Wayland only - use wl-copy
			if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.Command("wl-copy")
			}
		}

		// Fallback if nothing matched above
		if cmd == nil {
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			} else if _, err := exec.LookPath("xsel"); err == nil {
				cmd = exec.Command("xsel", "--clipboard", "--input")
			} else if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.Command("wl-copy")
			} else {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			}
		}
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}

	cmd.Stdin = strings.NewReader(text)

	return cmd.Run()
}
