package json

// Mode defines the decoding mode for the decoder/scanner/reader.
type Mode int

// Mode values are used to configure the behavior of the decoder/scanner/reader.
const (
	// Evict forces the decoder/scanner/reader to evict processed data from the
	// buffer as soon as possible.
	Evict Mode = 0x0000
	// Retain forces the decoder/scanner/reader to retain all data in the
	// buffer, while activating decoder optimizations based on the buffering.
	Retain Mode = 0x0001

	// StrictJson forces the decoder/scanner/reader to strictly follow the
	// JSON specification and throw errors on any non-JSON-standard input (not
	// implemented).
	StrictJson Mode = 0x0002

	// Strict forces the decoder/scanner/reader to strictly follow the JSON5
	// specification and throw errors on any non-standard input.
	Strict Mode = 0x0000
	// Relaxed allows the decoder/scanner/reader to accept a more relaxed
	// JSON5 input missing quotes around object keys without throwing errors.
	Relaxed Mode = 0x0010
	// Extended allows the decoder/scanner/reader to accept non-standard
	// JSON5 input using extensions, e.g. parsing of complex numbers.
	Extended Mode = 0x0020

	// Native forces the decoder/scanner/reader to use native Go types for
	// numbers.
	Native Mode = 0x0000
	// Precise forces the decoder/scanner/reader to use arbitrary precision
	// types for numbers.
	Precise Mode = 0x0100

	// CaseIgnore forces the decoder/scanner/reader to ignore case when
	// comparing object keys.
	CaseIgnore Mode = 0x1000

	// CaseDelimSkip forces the decoder/scanner/reader to skip hyphens and
	// underscores when comparing object keys.
	CaseDelimSkip Mode = 0x2000 // FIXME: Not implemented yet.
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
	// True `t` - the literal true token.
	True byte = 't'
	// False `f` - the literal false token.
	False byte = 'f'
	// Null `n` - the literal null token.
	Null byte = 'n'
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

// ScanContext describes the parser position expected by Scanner.Next methods.
type ScanContext byte

// ScanContext values are used to indicate the expected context of the next token
// to be scanned by the Scanner. The context is used to determine the valid
// token types and parsing rules for the next token.
const (
	// RootValue scans a value in root position.
	RootValue ScanContext = iota
	// ObjectKey scans an object property key.
	ObjectKey
	// ObjectValue scans an object property value.
	ObjectValue
	// ArrayValue scans an array element value.
	ArrayValue
)

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
//nolint:gocognit // prioritize performance.
//revive:disable-next-line:cognitive-complexity // prioritize performance.
func uspace(window []byte, offset int) (int, bool, bool) {
	if window[offset] == 0xC2 {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0xA0 {
			return 2, false, false
		}
	}

	//nolint:nestif // prioritize performance.
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

	//nolint:nestif // prioritize performance.
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
