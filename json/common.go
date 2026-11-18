package json

import (
	"strings"
)

// Mode defines the decoding mode for the decoder/scanner/reader.
type Mode int

// Mode values are used to configure the behavior of the decoder/scanner/reader.
const (
	// Evict forces the reader to evict token data from the buffer as soon as
	// the scanner is processing the next token. It is the responsibility of
	// the caller to create a copy of the token for further processing.
	Evict Mode = 0x0000
	// Retain forces the reader to retain all data in the buffer while allowing
	// the decoder/user to reference the buffer memory directly. It is thus the
	// responsibility of the caller to ensure that the buffer is not modified
	// for the lifetime of the escaped data structure.
	Retain Mode = 0x0001

	// Strict forces the scanner to strictly follow the JSON5 specification and
	// throw errors on any non-standard input.
	Strict Mode = 0x0000
	// Relaxed enables the scanner to accept a more relaxed JSON5 input with
	// missing quotes around object keys and missing value tokens evaluated to
	// zero values without throwing errors.
	Relaxed Mode = 0x0010
	// Extended enables the scanner to accept additional non-standard JSON5
	// input, e.g. parsing of complex numbers.
	Extended Mode = 0x0020

	// Json forces a strict standard JSON decoding. While the `Scanner` still
	// tokenizes with the same lexical behavior as `Strict`, the decoder is
	// enforcing the strict standard JSON-specific rules (not well tested,
	// yet).
	Json Mode = 0x0040

	// Native forces the decoder to use native Go types for numbers when the
	// target type is any.
	Native Mode = 0x0000
	// Precise forces the decoder to use arbitrary precision types for numbers
	// when the target type is any.
	Precise Mode = 0x0100

	// IgnoreCase allows the decoder to perform case-insensitive matching when
	// comparing object keys. This is often used in legacy JSON decoders.
	IgnoreCase Mode = 0x1000
	// SkipDelims allows the decoder to skip hyphens and underscores when
	// comparing object keys.
	SkipDelims Mode = 0x2000
)

// Basic JSON token types for scanning.
const (
	// ObjectStart `{` - the object start.
	ObjectStart byte = '{'
	// ObjectEnd `}` - the object end.
	ObjectEnd byte = '}'
	// ArrayStart `[` - the array start.
	ArrayStart byte = '['
	// ArrayEnd `]` - the array end.
	ArrayEnd byte = ']'
	// Comma `,` - the literal comma.
	Comma byte = ','
	// Colon `:` - the literal colon.
	Colon byte = ':'
	// Space ` ` - a white space token.
	Space byte = ' '
	// Comment `/` - a single-line comment token (`//`).
	Comment byte = '/'
	// CommentMulti `*` - a multi-line comment token (`/*...*/`).
	CommentMulti byte = '*'
	// Escape `\` - a backslash escape token.
	Escape byte = '\\'
	// Quote `'` - a single quote string token.
	Quote byte = '\''
	// String `"` - a double quote string token.
	String byte = '"'
	// Plain `` - a non-quote plain string token.
	Plain byte = '_'
	// True `t` - the literal true token.
	True byte = 't'
	// False `f` - the literal false token.
	False byte = 'f'
	// Null `n` - the literal null token.
	Null byte = 'n'
	// Zero `z` - an omitted object value decoded as a zero value.
	Zero byte = 'z'
)

// Special JSON token types for scanning.
const (
	// NaN `N` - the literal NaN token.
	NaN byte = 'N'
	// Infinity `I` - the literal Infinity token.
	Infinity byte = 'I'
	// Integer `i` - the literal integer token.
	Integer byte = 'i'
	// Decimal `d` - the literal decimal token.
	Decimal byte = 'd'
	// Complex `c` - the literal complex token.
	Complex byte = 'c'
	// HexaDecimal `x` - the literal hexadecimal token.
	HexaDecimal byte = 'x'
	// EOF `\0` - the end of a token stream.
	EOF byte = 0x00
)

// ScanContext provides the scanner with a context state, telling it about
// the expected type of the next token to apply the correct parsing rules.
type ScanContext byte

// ScanContext values are used to indicate the context state to the scanner
// when parsing the next token. The context is used to determine the valid
// token types and parsing rules for the next token.
const (
	// Unknown sentinel context value.
	Unknown ScanContext = iota
	// RootValue scans a value in root position.
	RootValue
	// ArrayValue scans an array element value.
	ArrayValue
	// ObjectKey scans an object property key.
	ObjectKey
	// ObjectColon scans the colon between an object key and value.
	ObjectColon
	// ObjectValue scans an object property value.
	ObjectValue
	// RootEnd scans the end of the root value.
	RootEnd
)

// normcomplex normalizes a complex number token into a standard form.
func normcomplex(token string) string {
	index := strings.IndexByte(token, 'i')
	if index >= len(token)-1 {
		if index > 0 && token[index-1] >= '0' && token[index-1] <= '9' {
			return token
		}

		return token[:index] + "1" + token[index:]
	}

	imaginary := token[:index]
	if index == 0 {
		imaginary = "1"
	} else if token[index-1] < '0' || token[index-1] > '9' {
		imaginary += "1"
	}
	if imaginary[0] != '+' && imaginary[0] != '-' {
		imaginary = "+" + imaginary
	}

	real := token[index+1:]
	if real == "+" || real == "-" {
		return token
	}
	if real[0] == '+' {
		real = real[1:]
	}

	return real + imaginary + "i"
}

// hexmap maps every ASCII byte to its 4-bit hex value. Entries outside
// '0'-'9', 'A'-'F', 'a'-'f' are 0xFF (invalid sentinel). Valid values are
// `0x00`-`0x0F`; the OR of any four values can therefore be compared to be
// larger than `0x0F` in a single branch to detect any invalid digit.
var hexmap = func() [256]byte {
	t := [256]byte{}
	for i := range t {
		t[i] = 0xFF
	}
	for c := byte('0'); c <= '9'; c++ {
		t[c] = c - '0'
	}
	for c := byte('A'); c <= 'F'; c++ {
		t[c] = c - 'A' + 10
	}
	for c := byte('a'); c <= 'f'; c++ {
		t[c] = c - 'a' + 10
	}

	return t
}()

// unihex decodes exactly 4 hexadecimal digits from the start of the buffer
// into a rune using a lookup table for branch-free per-digit decoding. The
// function returns (rune, true) on success or (0, false) on bad input. The
// caller must ensure that the buffer has at least 4 bytes. (e.g. by calling
// `s.reader.extend()` before invoking `unihex`).
func unihex(b []byte) (rune, bool) {
	v0, v1, v2, v3 := hexmap[b[0]], hexmap[b[1]], hexmap[b[2]], hexmap[b[3]]

	if v0|v1|v2|v3 > 0x0F {
		return 0, false
	}

	return rune(v0)<<12 | rune(v1)<<8 | rune(v2)<<4 | rune(v3), true
}

// uspace checks for UTF-8 encoded whitespace at the given offset in the
// window. It returns the number of bytes in the whitespace sequence, a boolean
// indicating if it is a line terminator, and a boolean indicating if more
// bytes are needed to determine the whitespace character.
//
//nolint:gocognit // prioritize speed.
//revive:disable-next-line:cognitive-complexity // prioritize speed.
//revive:disable-next-line:confusing-results // priority speed.
func uspace(window []byte, offset int) (int, bool, bool) {
	if window[offset] == 0xC2 {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0xA0 {
			return 2, false, false
		}
	}

	//nolint:nestif // prioritize speed.
	if window[offset] == 0xEF {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0xBB {
			if offset+2 >= len(window) {
				return 0, false, true
			} else if window[offset+2] == 0xBF {
				return 3, false, false
			}
		}
	}

	//nolint:nestif // prioritize speed.
	if window[offset] == 0xE2 {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0x80 {
			if offset+2 >= len(window) {
				return 0, false, true
			} else if window[offset+2] == 0xA8 ||
				window[offset+2] == 0xA9 {
				return 3, true, false
			}
		}
	}

	return 0, false, false
}
