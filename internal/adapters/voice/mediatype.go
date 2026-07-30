package voice

import (
	"mime"
	"strings"
)

// IsMediaType reports whether a Content-Type header carries the wanted type.
//
// mime.ParseMediaType is the documented way to read that header: it lowercases
// the type, handles quoted parameters, and refuses a malformed value. The four
// hand-rolled `strings.Split(header, ";")[0]` variants this replaces agreed with
// it on the simple cases and were quietly lenient on the rest.
func IsMediaType(header, want string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return false
	}
	return mediaType == strings.ToLower(want)
}
