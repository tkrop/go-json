package json

import (
	"bytes"
)

// Unmarshal parses the JSON-encoded data and stores the result in the value
// pointed to by value. If value is nil or not a pointer, Unmarshal returns an
// error.
//
// To unmarshal JSON into a struct, Unmarshal matches incoming object keys to
// the keys used by Marshal (either the struct field name or its tag),
// preferring an exact match but also accepting a case-insensitive match. See
// the documentation for Marshal for details about the conversion between Go
// values and JSON.
func Unmarshal(data []byte, value any) error {
	return NewDecoderBuffer(bytes.NewReader(data),
		make([]byte, 0, len(data))).Decode(value)
}
