package media

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hearthroom/cli/internal/api"
)

// extensionTypes is the type an upload declares for each file the library
// accepts. The provider still checks the bytes; the declaration matters for
// formats without a signature (JavaScript, JSON), which it accepts only when
// declared. Keep it in step with the provider's accepted list.
var extensionTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".svg":  "image/svg+xml",
	".woff": "font/woff", ".woff2": "font/woff2",
	".mp4": "video/mp4", ".webm": "video/webm",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg",
	".js": "text/javascript", ".mjs": "text/javascript",
	".wasm": "application/wasm",
	".json": "application/json",
}

// ContentType is the type declared for a file name; unknown extensions are
// sent as application/octet-stream and left to the provider to judge.
func ContentType(name string) string {
	if t, ok := extensionTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

func isQuickTimeName(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".mov")
}

func quickTimeMessage(name string) string {
	return fmt.Sprintf("%s is a QuickTime (.mov) video, which the media library does not accept; export it as MP4 (H.264) and upload that file", name)
}

// isQuickTimeRefusal reports the provider's answer to a QuickTime upload:
// 400 invalid_param with detail {"reason":"quicktime"}.
func isQuickTimeRefusal(err error) bool {
	var e *api.Error
	if !errors.As(err, &e) || e.Code != "invalid_param" {
		return false
	}
	d, ok := e.Detail.(map[string]any)
	return ok && d["reason"] == "quicktime"
}
