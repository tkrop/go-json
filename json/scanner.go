//nolint:gocognit,gocyclo,cyclop,nestif,funlen,maintidx // priority speed.
//revive:disable:function-length -- priority speed.
//revive:disable:cognitive-complexity -- priority speed.
//revive:disable:max-control-nesting -- priority speed.
//revive:disable:cyclomatic -- priority speed.
package json

import (
	"io"
	"unicode/utf8"
)

// Character types for JSON scanning.
const (
	// Base types.
	base    byte = 0x07
	space   byte = 0x01
	escape  byte = 0x02
	delim   byte = 0x03
	strng   byte = 0x04
	keyword byte = 0x05
	number  byte = 0x06
	comment byte = 0x07
	// Delim sub-types.
	sep   byte = 0x08
	open  byte = 0x10
	close byte = 0x20
	// Numeric sub-type.
	sign  byte = 0x08
	digit byte = 0x10
	exp   byte = 0x20
	hex   byte = 0x40
	hexs  byte = 0x80 // available.
)

// mask is the character type lookup table for JSON scanning.
var mask = [256]byte{
	'\t': space, // horizontal tab
	'\n': space, // line feed (U+000A)
	'\v': space, // vertical tab (U+000B)
	'\f': space, // form feed (U+000C)
	'\r': space, // carriage return (U+000D)
	' ':  space, // normal space (U+0020)
	'\\': escape,
	'/':  comment,
	'\'': strng,
	'"':  strng,
	':':  delim | sep,
	',':  delim | sep,
	'{':  delim | open,
	'}':  delim | close,
	'[':  delim | open,
	']':  delim | close,
	'n':  keyword,
	't':  keyword,
	'f':  keyword | hex,
	'I':  keyword,
	'N':  keyword,
	'0':  number | digit | hex,
	'1':  number | digit | hex,
	'2':  number | digit | hex,
	'3':  number | digit | hex,
	'4':  number | digit | hex,
	'5':  number | digit | hex,
	'6':  number | digit | hex,
	'7':  number | digit | hex,
	'8':  number | digit | hex,
	'9':  number | digit | hex,
	'+':  number | sign,
	'-':  number | sign,
	'.':  number,
	'i':  number,
	'a':  hex,
	'b':  hex,
	'c':  hex,
	'd':  hex,
	'e':  hex | exp,
	'x':  hex | hexs,
	'A':  hex,
	'B':  hex,
	'C':  hex,
	'D':  hex,
	'E':  hex | exp,
	'F':  hex,
	'X':  hex | hexs,
}

// predefined keyword byte slices for optimized scanning of JSON keywords and
// special tokens.
var (
	_True_     = []byte("true")
	_False_    = []byte("false")
	_Null_     = []byte("null")
	_Infinity_ = []byte("Infinity")
	_NaN_      = []byte("NaN")
)

// keywords maps keyword start bytes to their expected suffixes.
var keywords = [256][]byte{
	't': _True_,
	'f': _False_,
	'n': _Null_,
	'I': _Infinity_,
	'N': _NaN_,
}

// Scanner implements a relaxed JSON5 scanner producing a stream of tokens
// from an underlying io.Reader.
type Scanner struct {
	// The underlying JSON5 reader.
	reader Reader
	// The mode used for scanning.
	mode Mode

	// offset is the current scanner offset in the reader window.
	offset int
	// iscomplex indicates if the current number token is a complex number.
	iscomplex bool
}

// NewScanner returns a new Scanner reading from the supplied `io.Reader` using
// the supplied buffer for scanning. The Scanner operates in-place on the
// buffer, producing a stream of tokens via `Next`, expressed as []byte slices.
// The []byte slices reference the internal buffer of the Scanner and are valid
// until the next call to `Next`.
func NewScanner(reader io.Reader, buffer []byte) *Scanner {
	return &Scanner{
		reader: *NewReader(buffer, reader),
		mode:   Relaxed | Extended,
	}
}

// Mode sets the mode used for scanning and reading.
func (s *Scanner) Mode(mode Mode) *Scanner {
	s.reader.Mode(mode)
	s.mode = mode
	return s
}

// Position returns the current scanner stream position. When used with
// `Next`, only the byte position is updated. Using `Next_` also the line
// and character positions are updated. However, since this comes with a
// performance cost of roughly 20%, this is only recommended when the token
// location is required for debugging.
func (s *Scanner) Position() Position {
	return s.reader.position()
}

// Error returns the first error encountered. When the underlying reader is
// exhausted, Error returns io.EOF.
func (s *Scanner) Error() error {
	return s.reader.err
}

// Next returns the next lexical token in the stream consisting of a token type
// and a token slice containing the extracted token content. The token content
// is only provided for correctly parsed strings and numbers and only valid
// until the next token is parsed.
//
// A valid token types are:
//
// * `{` - the object start.
// * `[` - the array start.
// * `}` - the object end.
// * `]` - the array end.
// * `,` - the literal comma.
// * `:` - the literal colon.
// * `n` - the literal `null` value.
// * `t` - the literal `true` value.
// * `f` - the literal `false` value.
// * `"` - a string, possibly containing backslash escaped entities.
// * `I` - the numeric `[+|-]Infinity` value.
// * `N` - the numeric `NaN` value.
// * `ì` - a numeric integer number.
// * `d` - a numeric decimal (floating point) number.
// * `c` - a numeric complex number.
// * `x` - a numeric hexadecimal number.
//
// Note: Due to the relaxed nature of the parser allowing to write strings
// without quotes, the scanner skips empty strings unless explicitly quoted.
func (s *Scanner) Next(ctx ScanContext) (byte, []byte) {
	_ = ctx

	if skip := s.skip(0); skip != 0 {
		s.reader.advance(skip)
	}

	char, ok := s.reader.peek()
	if !ok {
		return EOF, nil
	}

	// TODO: check if-optimization?
	switch mask[char] & base {
	case delim:
		return s.commit(char, []byte{char}, 1)

	case keyword:
		if offset, _ := s.keyword(char); offset != 0 {
			token := s.reader.window()[:offset]

			return s.commit(char, token, offset)
		}

	case number:
		if typ, offset, _ := s.number(0, s.reader.window()); typ != EOF {
			token := s.reader.window()[:offset]

			return s.commit(typ, token, offset)
		}

	case strng:
		return s.commit(s.quoted(char))
	}

	if s.mode&Relaxed != 0 {
		return s.commit(s.relaxed())
	}

	return EOF, nil
}

// Next_ returns the next lexical token in the stream consisting of a token
// type and a token slice containing the extracted token content. The token
// content is only provided for correctly parsed strings and numbers and only
// valid until the next token is parsed.
//
// A valid token types are:
//
// * `{` - the object start.
// * `[` - the array start.
// * `}` - the object end.
// * `]` - the array end.
// * `,` - the literal comma.
// * `:` - the literal colon.
// * `n` - the literal `null` value.
// * `t` - the literal `true` value.
// * `f` - the literal `false` value.
// * `"` - a string, possibly containing backslash escaped entities.
// * `I` - the numeric `[+|-]Infinity` value.
// * `N` - the numeric `NaN` value.
// * `ì` - a numeric integer number.
// * `d` - a numeric decimal (floating point) number.
// * `c` - a numeric complex number.
// * `x` - a numeric hexadecimal number.
// * ` ` - a white space token.
// * `/` - a comment token (`//` or `/*...*/`).
// * `*` - a multi-line comment token (`/*...*/`).
//
// Note: Due to the relaxed nature of the parser allowing to write strings
// without quotes, the scanner skips empty strings unless explicitly quoted.
//
// Next_ is a variant of `Next` that calculates the token location in the
// stream, without the need to manually track newlines and escapes in the
// stream. However, this comes with a performance cost of roughly 20%.
func (s *Scanner) Next_(ctx ScanContext) (byte, []byte) {
	_ = ctx

	offset, line, char, token := s.space_()
	if offset != 0 {
		return s.commit_(Space, token, offset, line, char)
	}

	ch, ok := s.reader.peek()
	if !ok {
		return EOF, nil
	}

	// TODO: check if-optimization?
	switch mask[ch] & base {
	case delim:
		return s.commit_(ch, []byte{ch}, 1, 0, 1)

	case comment:
		if typ, token, offset, lines, char := s.comment_(0); offset != 0 {
			return s.commit_(typ, token, offset, lines, char)
		}

	case keyword:
		if offset, _ := s.keyword(ch); offset != 0 {
			token := s.reader.window()[:offset]

			return s.commit_(ch, token, offset, 0, offset)
		}

	case number:
		if typ, offset, _ := s.number(0, s.reader.window()); typ != EOF {
			token := s.reader.window()[:offset]

			return s.commit_(typ, token, offset, 0, offset)
		}

	case strng:
		return s.commit_(s.quoted_(ch))
	}

	if s.mode&Relaxed != 0 {
		return s.commit_(s.relaxed_())
	}

	return EOF, nil
}

// commit is a convenience function that advances the reader by the given
// offset and resets the scanner's offset to 0. It returns the token type and
// token value.
func (s *Scanner) commit(
	typ byte, token []byte, offset int,
) (byte, []byte) {
	s.reader.advance(offset)
	s.offset = 0

	return typ, token
}

// commit_ is a convenience function that advances the reader by the given
// offset, lines, and char position, and resets the scanner's offset to 0. It
// returns the token type and token value.
func (s *Scanner) commit_(
	typ byte, token []byte, offset, line, char int,
) (byte, []byte) {
	s.reader.advance_(offset, line, char)
	s.offset = 0

	return typ, token
}

// space_ parses leading whitespace from the current reader window and returns
// the consumed token and position deltas. It supports ASCII and UTF-8 encoded
// JSON whitespace and may extend the reader window to complete multi-byte
// sequences. It returns zero values when the next byte is not whitespace.
func (s *Scanner) space_() (int, int, int, []byte) {
	offset, line, char := 0, 0, 0

	for {
		window := s.reader.window()
		for offset < len(window) {
			if mask[window[offset]]&base == space {
				if window[offset] == '\n' {
					line++
					char = 0
				} else {
					char++
				}
				offset++
				continue
			}

			size, lbreak, extend := uspace(window, offset)
			if size > 0 {
				offset += size
				if lbreak {
					line++
					char = 0
				} else {
					char++
				}
				continue
			}

			if extend {
				break
			}

			if offset == 0 {
				return 0, 0, 0, nil
			}

			return offset, line, char, window[:offset]
		}

		if s.reader.extend() == 0 {
			if offset == 0 {
				return 0, 0, 0, nil
			}

			return offset, line, char, window[:offset]
		}
	}
}

// comment parses JSON5 comments (`//`, `/*...*/`) starting at the given offset.
// It returns the token offset past the comment, or zero if not a valid comment
// is found.
func (s *Scanner) comment(offset int) int {
	window := s.reader.window()
	offset += 2

	for len(window) < offset {
		if s.reader.extend() == 0 {
			return 0
		}
		window = s.reader.window()
	}

	switch window[offset-1] {
	case '/':
		for {
			window = s.reader.window()
			for offset < len(window) {
				char := window[offset]
				if char == '\n' || char == '\r' {
					return offset
				}

				if size, line, extend := uspace(window, offset); size != 0 {
					if line {
						return offset
					}
					offset += size
					continue
				} else if extend {
					break
				}

				offset++
			}

			if s.reader.extend() == 0 {
				return offset
			}
		}

	case '*':
		for {
			window = s.reader.window()
			for offset < len(window) {
				if window[offset] == '*' {
					if offset+1 >= len(window) {
						break
					}
					if window[offset+1] == '/' {
						offset += 2

						return offset
					}
				}

				offset++
			}

			if s.reader.extend() == 0 {
				return offset
			}
		}
	}

	return 0
}

// comment_ parses JSON5 comments (`//`, `/*...*/`) starting at the given
// offset and tracks line/char deltas for debug position reporting. It returns
// the token offset past the comment, or zero if not a valid comment is found.
func (s *Scanner) comment_(offset int) (byte, []byte, int, int, int) {
	window := s.reader.window()
	offset += 2

	for len(window) < offset {
		if s.reader.extend() == 0 {
			return EOF, nil, 0, 0, 0
		}
		window = s.reader.window()
	}

	switch window[offset-1] {
	case '/':
		line, char := 0, 2

		for {
			window = s.reader.window()
			for offset < len(window) {
				ch := window[offset]
				if ch == '\n' || ch == '\r' {
					return Comment, window[2:offset], offset, line, char
				}

				if size, lbreak, extend := uspace(window, offset); size != 0 {
					if lbreak {
						return Comment, window[2:offset], offset, line, char
					}
					offset += size
					char++
					continue
				} else if extend {
					break
				}

				offset++
				char++
			}

			if s.reader.extend() == 0 {
				window = s.reader.window()

				return Comment, window[2:offset], offset, line, char
			}
		}

	case '*':
		line, char := 0, 2

		for {
			window = s.reader.window()
			for offset < len(window) {
				if window[offset] == '*' {
					if offset+1 >= len(window) {
						break
					}
					if window[offset+1] == '/' {
						return CommentMulti, window[2:offset],
							offset + 2, line, char + 2
					}
				}

				if window[offset] == '\n' {
					offset++
					line++
					char = 0
					continue
				}

				if size, lbreak, extend := uspace(window, offset); size != 0 {
					offset += size
					if lbreak {
						line++
						char = 0
					} else {
						char++
					}
					continue
				} else if extend {
					break
				}

				offset++
				char++
			}

			if s.reader.extend() == 0 {
				window = s.reader.window()

				return CommentMulti, window[2:offset], offset, line, char
			}
		}
	}

	return EOF, nil, 0, 0, 0
}

// keyword checks for the JSON keyword values `true`, `false`, and `null`, as
// well as the special tokens `Infinity` and `NaN` in JSON5. If a valid keyword
// is found, the function returns the offset of the next character after the
// keyword and the number of bytes to skip until the next delimiter value.
//
// TODO: check optimization using a map of first char to expected token string.
// TODO: check optimization returning (typ, token)
func (s *Scanner) keyword(ch byte) (offset, skip int) {
	//nolint:staticcheck // priority speed.
	if ch == 'n' {
		return s.token(1, _Null_[1:])
	} else if ch == 'f' {
		return s.token(1, _False_[1:])
	} else if ch == 't' {
		return s.token(1, _True_[1:])
	} else if ch == 'N' {
		return s.token(1, _NaN_[1:])
	} else if ch == 'I' {
		return s.token(1, _Infinity_[1:])
	}

	return 0, 0
}

// keywordx checks for JSON keywords using a lookup table instead of a switch
// statement. This is the optimized variant for benchmarking against keyword().
func (s *Scanner) keywordx(ch byte) (offset, skip int) {
	return s.token(1, keywords[ch][1:])
}

// token checks that the expected token suffix is present at the given offset
// in the current window and that the full token is followed by a valid token
// boundary. It returns the token end offset and the number of skipped bytes
// before the following delimiter, if matching, or 0 otherwise.
func (s *Scanner) token(offset int, expect []byte) (int, int) {
	last := offset + len(expect)
	if !s.reader.compare(offset, expect) {
		return 0, 0
	}

	if skip := s.skip(last); skip >= 0 {
		char, ok := s.reader.get(last + skip)
		if !ok || mask[char]&base == delim {
			return last, skip
		}
	}

	return 0, 0
}

// skip peeks forward from offset in the reader window, skipping whitespace
// and comments. The function returns the number of bytes skipped until the
// next non-whitespace character outside comments, or null if nothing is
// skipped. The function may extend the reader window as needed to complete
// multi-byte sequences or comments.
func (s *Scanner) skip(offset int) int {
	start := offset

	for {
		window := s.reader.window()
		for offset < len(window) {
			char := window[offset]
			kind := mask[char] & base
			if kind == space {
				offset++
				continue
			}

			size, _, extend := uspace(window, offset)
			if size > 0 {
				offset += size
				continue
			} else if extend {
				break
			}

			if kind == comment {
				next := s.comment(offset)
				if next == 0 {
					return 0
				}
				offset = next
				window = s.reader.window()
				continue
			}

			return offset - start
		}

		if s.reader.extend() == 0 {
			return offset - start
		}
	}
}

// number parses a number returning the number type and the offset of the next
// token splitted into the bytes used for the number and the white space bytes.
// The function parses for integer ('i'), decimal ('d'), and hexadecimal ('x')
// numbers, as well as the special tokens `NaN` and `Infinity`. If the value
// is not a number, the function returns the type EOF, an offset of 0, and 0
// skipped bytes.
func (s *Scanner) numbers(
	offset int, window []byte,
) (byte, int, int) {
	const (
		begin byte = iota
		zero
		digit1
		dot1
		digit2
		exp1
		expsign1
		digit3
		hex1
	)

	state, stop, skip := begin, false, 0

next:
	for {
		for offset < len(window) {
			ch := window[offset]
			mask := mask[ch]
			if mask&base == delim {
				stop = true
				break
			} else if mask&base == space {
				offset++
				skip++
				continue
			}

			bytes, _, extend := uspace(window, offset)
			if bytes > 0 {
				offset += bytes
				skip += bytes
				continue
			} else if extend {
				if s.reader.extend() == 0 {
					return EOF, 0, 0
				}
				window = s.reader.window()
				continue next
			} else if skip > 0 {
				return EOF, 0, 0
			}

			switch state {
			case begin:
				if mask&digit != 0 {
					if ch == '0' {
						state = zero
					} else {
						state = digit1
					}
					break
				} else if mask&sign != 0 {
					break
				} else if ch == '.' {
					state = dot1
					break
				} else if ch == 'I' {
					offset, skip := s.token(offset+1, _Infinity_[1:])
					if offset != 0 {
						return Infinity, offset, skip
					}
				} else if ch == 'N' {
					offset, skip := s.token(offset+1, _NaN_[1:])
					if offset != 0 {
						return NaN, offset, skip
					}
				}
				return EOF, 0, 0

			case digit1:
				if mask&digit != 0 {
					break
				}
				fallthrough

			case zero:
				if ch == '.' {
					state = dot1
					break
				} else if mask&exp != 0 {
					state = exp1
					break
				} else if mask&hexs != 0 {
					state = hex1
					break
				}
				return EOF, 0, 0

			case dot1:
				if mask&digit != 0 {
					state = digit2
					break
				}
				return EOF, 0, 0

			case digit2:
				if mask&digit != 0 {
					break
				} else if mask&exp != 0 {
					state = exp1
					break
				}
				return EOF, 0, 0

			case exp1:
				if mask&sign != 0 {
					state = expsign1
					break
				}
				fallthrough

			case expsign1:
				if mask&digit != 0 {
					state = digit3
					break
				}
				return EOF, 0, 0

			case digit3:
				if mask&digit != 0 {
					break
				}
				return EOF, 0, 0

			case hex1:
				if mask&hex != 0 {
					break
				}
				return EOF, 0, 0
			}

			offset++
		}

		if stop || s.reader.extend() == 0 {
			switch state {
			case zero, digit1:
				return Integer, offset - skip, skip
			case digit2, digit3:
				return Decimal, offset - skip, skip
			case hex1:
				return HexaDecimal, offset - skip, skip
			case dot1:
				if offset-skip > 1 {
					return Decimal, offset - skip, skip
				}
				fallthrough
			default:
				return EOF, 0, 0
			}
		}

		window = s.reader.window()
	}
}

// number parses a number returning the number type and the offset of the next
// token splitted into the bytes used for the number and the white space bytes.
// The function parses for integer ('i'), decimal ('d'), hexadecimal ('x'), and
// complex ('c') numbers, as well as the special tokens `NaN` and `Infinity`.
// If the value is not a number, the function returns the type EOF, an offset
// of 0, and 0 skipped bytes.
func (s *Scanner) number(
	offset int, window []byte,
) (byte, int, int) {
	const (
		begin byte = iota
		zero
		digit1
		dot1
		digit2
		exp1
		expsign1
		digit3
		hex1
		complex
	)

	state, stop, skip := begin, false, 0

next:
	for {
		for offset < len(window) {
			ch := window[offset]
			mask := mask[ch]
			if mask&base == delim {
				stop = true
				break
			} else if mask&base == space {
				offset++
				skip++
				continue
			}

			bytes, _, extend := uspace(window, offset)
			if bytes > 0 {
				offset += bytes
				skip += bytes
				continue
			} else if extend {
				if s.reader.extend() == 0 {
					return EOF, 0, 0
				}
				window = s.reader.window()
				continue next
			} else if skip > 0 {
				return EOF, 0, 0
			}

			switch state {
			case begin:
				if mask&digit != 0 {
					if ch == '0' {
						state = zero
					} else {
						state = digit1
					}
					break
				} else if mask&sign != 0 {
					break
				} else if ch == '.' {
					state = dot1
					break
				} else if ch == 'I' {
					offset, skip := s.token(offset+1, _Infinity_[1:])
					if offset != 0 {
						return Infinity, offset, skip
					}
				} else if ch == 'N' {
					offset, skip := s.token(offset+1, _NaN_[1:])
					if offset != 0 {
						return NaN, offset, skip
					}
				} else if ch == 'i' {
					state = complex
					break
				}
				return EOF, 0, 0

			case digit1:
				if mask&digit != 0 {
					break
				}

				fallthrough
			case zero:
				if ch == '.' {
					state = dot1
					break
				} else if mask&exp != 0 {
					state = exp1
					break
				} else if mask&hexs != 0 {
					state = hex1
					break
				} else if mask&sign != 0 {
					return s.complex(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}
				return EOF, 0, 0

			case dot1:
				if mask&digit != 0 {
					state = digit2
					break
				}
				return EOF, 0, 0

			case digit2:
				if mask&digit != 0 {
					break
				} else if mask&exp != 0 {
					state = exp1
					break
				} else if mask&sign != 0 {
					return s.complex(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}
				return EOF, 0, 0

			case exp1:
				if mask&sign != 0 {
					state = expsign1
					break
				}
				fallthrough

			case expsign1:
				if mask&digit != 0 {
					state = digit3
					break
				}
				return EOF, 0, 0

			case digit3:
				if mask&digit != 0 {
					break
				} else if mask&sign != 0 {
					return s.complex(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}
				return EOF, 0, 0

			case hex1:
				if mask&hex != 0 {
					break
				}
				return EOF, 0, 0
			}

			offset++
		}

		if stop || s.reader.extend() == 0 {
			switch state {
			case zero, digit1:
				return Integer, offset - skip, skip
			case digit2, digit3:
				return Decimal, offset - skip, skip
			case hex1:
				return HexaDecimal, offset - skip, skip
			case complex:
				return Complex, offset - skip, skip
			case dot1:
				if offset-skip > 1 {
					return Decimal, offset - skip, skip
				}
				fallthrough
			default:
				return EOF, 0, 0
			}
		}

		window = s.reader.window()
	}
}

// complex is a helper method that handles the additional imaginary part of a
// complex number following a real part. The function adds a imaginary flag to
// the scanner to mark the real part of the complex number. If the imaginary
// part is successfully parsed, the function returns the complex token type and
// the offset of the next token. If the imaginary part is not correctly parsed,
// the function returns EOF and an offset of 0, indicating that the number is
// not a complex number.
func (s *Scanner) complex(
	offset int, window []byte,
) (byte, int, int) {
	if s.mode&Extended == 0 || s.iscomplex {
		return EOF, 0, 0
	}

	s.iscomplex = true
	ch, offset, skip := s.number(offset, window)
	s.iscomplex = false

	if ch == Complex {
		return Complex, offset, skip
	}

	return EOF, 0, 0
}

// number parses a number returning the number type and the offset of the next
// token splitted into the bytes used for the number and the white space bytes.
// The function parses for integer ('i'), decimal ('d'), hexadecimal ('x'), and
// complex ('c') numbers, as well as the special tokens `NaN` and `Infinity`.
// If the value is not a number, the function returns the type EOF, an offset
// of 0, and 0 skipped bytes.
func (s *Scanner) numberx(
	offset int, window []byte,
) (byte, int, int) {
	const (
		begin byte = iota
		zero
		digit1
		dot1
		digit2
		exp1
		expsign1
		digit3
		hex1
		complex
	)

	state, stop, skip := begin, false, 0

next:
	for {
		for offset < len(window) {
			ch := window[offset]
			mask := mask[ch]
			if mask&base == delim {
				stop = true
				break
			} else if mask&base == space {
				offset++
				skip++
				continue
			}

			bytes, _, extend := uspace(window, offset)
			if bytes > 0 {
				offset += bytes
				skip += bytes
				continue
			} else if extend {
				if s.reader.extend() == 0 {
					return EOF, 0, 0
				}
				window = s.reader.window()
				continue next
			} else if skip > 0 {
				return EOF, 0, 0
			}

			//nolint:staticcheck // priority speed.
			if state == digit1 {
				if mask&digit != 0 {
					goto advance
				} else if ch == '.' {
					state = dot1
					goto advance
				} else if mask&exp != 0 {
					state = exp1
					goto advance
				} else if mask&hexs != 0 {
					state = hex1
					goto advance
				} else if mask&sign != 0 {
					return s.complexx(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == digit2 {
				if mask&digit != 0 {
					goto advance
				} else if mask&exp != 0 {
					state = exp1
					goto advance
				} else if mask&sign != 0 {
					return s.complexx(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == digit3 {
				if mask&digit != 0 {
					goto advance
				} else if mask&sign != 0 {
					return s.complexx(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == hex1 {
				if mask&hex != 0 {
					goto advance
				}

				return EOF, 0, 0
			} else if state == zero {
				if ch == '.' {
					state = dot1
					goto advance
				} else if mask&exp != 0 {
					state = exp1
					goto advance
				} else if mask&hexs != 0 {
					state = hex1
					goto advance
				} else if mask&sign != 0 {
					return s.complexx(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == begin {
				if mask&digit != 0 {
					if ch == '0' {
						state = zero
					} else {
						state = digit1
					}
					goto advance
				} else if mask&sign != 0 {
					goto advance
				} else if ch == '.' {
					state = dot1
					goto advance
				} else if ch == 'I' {
					offset, skip := s.token(offset+1, _Infinity_[1:])
					if offset != 0 {
						return Infinity, offset, skip
					}
				} else if ch == 'N' {
					offset, skip := s.token(offset+1, _NaN_[1:])
					if offset != 0 {
						return NaN, offset, skip
					}
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == dot1 {
				if mask&digit != 0 {
					state = digit2
					goto advance
				}

				return EOF, 0, 0
			} else if state == exp1 {
				if mask&sign != 0 {
					state = expsign1
					goto advance
				} else if mask&digit != 0 {
					state = digit3
					goto advance
				}

				return EOF, 0, 0
			} else if state == expsign1 {
				if mask&digit != 0 {
					state = digit3
					goto advance
				}

				return EOF, 0, 0
			}

		advance:
			offset++
		}

		if stop || s.reader.extend() == 0 {
			if state == zero || state == digit1 {
				return Integer, offset - skip, skip
			} else if state == digit2 || state == digit3 {
				return Decimal, offset - skip, skip
			} else if state == hex1 {
				return HexaDecimal, offset - skip, skip
			} else if state == complex {
				return Complex, offset - skip, skip
			} else if state == dot1 && offset-skip > 1 {
				return Decimal, offset - skip, skip
			}

			return EOF, 0, 0
		}

		window = s.reader.window()
	}
}

// complexx is a helper method that handles the additional imaginary  part of a
// complex number following a real part. The function adds a imaginary flag to
// the scanner to mark the real part of the complex number. If the imaginary
// part is successfully parsed, the function returns the complex token type and
// the offset of the next token. If the imaginary part is not correctly parsed,
// the function returns EOF and an offset of 0, indicating that the number is
// not a complex number.
func (s *Scanner) complexx(
	offset int, window []byte,
) (byte, int, int) {
	if s.mode&Extended == 0 || s.iscomplex {
		return EOF, 0, 0
	}

	s.iscomplex = true
	ch, offset, skip := s.numberx(offset, window)
	s.iscomplex = false

	if ch == Complex {
		return Complex, offset, skip
	}

	return EOF, 0, 0
}

// numbery is a variant of number that uses composite mask checks to
// distinguish numeric sub-types from other token kinds sharing high bits.
func (s *Scanner) numbery(
	offset int, window []byte,
) (byte, int, int) {
	const (
		begin byte = iota
		zero
		digit1
		dot1
		digit2
		exp1
		expsign1
		digit3
		hex1
		complex
	)

	state, stop, skip := begin, false, 0

next:
	for {
		for offset < len(window) {
			ch := window[offset]
			mask := mask[ch]
			kind := mask & base
			if kind == delim {
				stop = true
				break
			} else if kind == space {
				offset++
				skip++
				continue
			}

			bytes, _, extend := uspace(window, offset)
			if bytes > 0 {
				offset += bytes
				skip += bytes
				continue
			} else if extend {
				if s.reader.extend() == 0 {
					return EOF, 0, 0
				}
				window = s.reader.window()
				continue next
			} else if skip > 0 {
				return EOF, 0, 0
			}

			switch state {
			case begin:
				if mask&(number|digit) == number|digit {
					if ch == '0' {
						state = zero
					} else {
						state = digit1
					}
					break
				} else if mask&(number|sign) == number|sign {
					break
				} else if ch == '.' {
					state = dot1
					break
				} else if ch == 'I' {
					offset, skip := s.token(offset+1, _Infinity_[1:])
					if offset != 0 {
						return Infinity, offset, skip
					}
				} else if ch == 'N' {
					offset, skip := s.token(offset+1, _NaN_[1:])
					if offset != 0 {
						return NaN, offset, skip
					}
				} else if ch == 'i' {
					state = complex
					break
				}

				return EOF, 0, 0

			case digit1:
				if mask&(number|digit) == number|digit {
					break
				}

				fallthrough
			case zero:
				if ch == '.' {
					state = dot1
					break
				} else if mask&(base|exp) == exp {
					state = exp1
					break
				} else if mask&(base|hexs) == hexs {
					state = hex1
					break
				} else if mask&(number|sign) == number|sign {
					return s.complexy(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}

				return EOF, 0, 0

			case dot1:
				if mask&(number|digit) == number|digit {
					state = digit2
					break
				}

				return EOF, 0, 0

			case digit2:
				if mask&(number|digit) == number|digit {
					break
				} else if mask&(base|exp) == exp {
					state = exp1
					break
				} else if mask&(number|sign) == number|sign {
					return s.complexy(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}
				return EOF, 0, 0

			case exp1:
				if mask&(number|sign) == number|sign {
					state = expsign1
					break
				}
				fallthrough

			case expsign1:
				if mask&(number|digit) == number|digit {
					state = digit3
					break
				}

				return EOF, 0, 0

			case digit3:
				if mask&(number|digit) == number|digit {
					break
				} else if mask&(number|sign) == number|sign {
					return s.complexy(offset, window)
				} else if ch == 'i' {
					state = complex
					break
				}

				return EOF, 0, 0

			case hex1:
				if mask&hex != 0 && (kind == 0 ||
					kind == number || ch == 'f') {
					break
				}

				return EOF, 0, 0
			}

			offset++
		}

		if stop || s.reader.extend() == 0 {
			switch state {
			case zero, digit1:
				return Integer, offset - skip, skip
			case digit2, digit3:
				return Decimal, offset - skip, skip
			case hex1:
				return HexaDecimal, offset - skip, skip
			case complex:
				return Complex, offset - skip, skip
			case dot1:
				if offset-skip > 1 {
					return Decimal, offset - skip, skip
				}
				fallthrough
			default:
				return EOF, 0, 0
			}
		}

		window = s.reader.window()
	}
}

// complexy is a helper method that handles the additional imaginary part of a
// complex number following a real part. The function adds a imaginary flag to
// the scanner to mark the real part of the complex number. If the imaginary
// part is successfully parsed, the function returns the complex token type and
// the offset of the next token. If the imaginary part is not correctly parsed,
// the function returns EOF and an offset of 0, indicating that the number is
// not a complex number.
//
//nolint:misspell // intentional test name.
func (s *Scanner) complexy(
	offset int, window []byte,
) (byte, int, int) {
	if s.mode&Extended == 0 || s.iscomplex {
		return EOF, 0, 0
	}

	s.iscomplex = true
	ch, offset, skip := s.numbery(offset, window)
	s.iscomplex = false

	if ch == Complex {
		return Complex, offset, skip
	}

	return EOF, 0, 0
}

// numberz combines numberx control flow (if/else and goto advance) with
// numbery composite mask checks to allow additional marker bits in the table.
func (s *Scanner) numberz(
	offset int, window []byte,
) (byte, int, int) {
	const (
		begin byte = iota
		zero
		digit1
		dot1
		digit2
		exp1
		expsign1
		digit3
		hex1
		complex
	)

	state, stop, skip := begin, false, 0

next:
	for {
		for offset < len(window) {
			ch := window[offset]
			mask := mask[ch]
			kind := mask & base
			if kind == delim {
				stop = true
				break
			} else if kind == space {
				offset++
				skip++
				continue
			}

			bytes, _, extend := uspace(window, offset)
			if bytes > 0 {
				offset += bytes
				skip += bytes
				continue
			} else if extend {
				if s.reader.extend() == 0 {
					return EOF, 0, 0
				}
				window = s.reader.window()
				continue next
			} else if skip > 0 {
				return EOF, 0, 0
			}

			//nolint:staticcheck // priority speed.
			if state == digit1 {
				if mask&(number|digit) == number|digit {
					goto advance
				} else if ch == '.' {
					state = dot1
					goto advance
				} else if mask&(base|exp) == exp {
					state = exp1
					goto advance
				} else if mask&(base|hexs) == hexs {
					state = hex1
					goto advance
				} else if mask&(number|sign) == number|sign {
					return s.complexz(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == digit2 {
				if mask&(number|digit) == number|digit {
					goto advance
				} else if mask&(base|exp) == exp {
					state = exp1
					goto advance
				} else if mask&(number|sign) == number|sign {
					return s.complexz(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == digit3 {
				if mask&(number|digit) == number|digit {
					goto advance
				} else if mask&(number|sign) == number|sign {
					return s.complexz(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == hex1 {
				if mask&hex != 0 && (kind == 0 ||
					kind == number || ch == 'f') {
					goto advance
				}

				return EOF, 0, 0
			} else if state == zero {
				if ch == '.' {
					state = dot1
					goto advance
				} else if mask&(base|exp) == exp {
					state = exp1
					goto advance
				} else if mask&(base|hexs) == hexs {
					state = hex1
					goto advance
				} else if mask&(number|sign) == number|sign {
					return s.complexz(offset, window)
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == begin {
				if mask&(number|digit) == number|digit {
					if ch == '0' {
						state = zero
					} else {
						state = digit1
					}
					goto advance
				} else if mask&(number|sign) == number|sign {
					goto advance
				} else if ch == '.' {
					state = dot1
					goto advance
				} else if ch == 'I' {
					offset, skip := s.token(offset+1, _Infinity_[1:])
					if offset != 0 {
						return Infinity, offset, skip
					}
				} else if ch == 'N' {
					offset, skip := s.token(offset+1, _NaN_[1:])
					if offset != 0 {
						return NaN, offset, skip
					}
				} else if ch == 'i' {
					state = complex
					goto advance
				}

				return EOF, 0, 0
			} else if state == dot1 {
				if mask&(number|digit) == number|digit {
					state = digit2
					goto advance
				}

				return EOF, 0, 0
			} else if state == exp1 {
				if mask&(number|sign) == number|sign {
					state = expsign1
					goto advance
				} else if mask&(number|digit) == number|digit {
					state = digit3
					goto advance
				}

				return EOF, 0, 0
			} else if state == expsign1 {
				if mask&(number|digit) == number|digit {
					state = digit3
					goto advance
				}

				return EOF, 0, 0
			}

		advance:
			offset++
		}

		if stop || s.reader.extend() == 0 {
			if state == zero || state == digit1 {
				return Integer, offset - skip, skip
			} else if state == digit2 || state == digit3 {
				return Decimal, offset - skip, skip
			} else if state == hex1 {
				return HexaDecimal, offset - skip, skip
			} else if state == complex {
				return Complex, offset - skip, skip
			} else if state == dot1 && offset-skip > 1 {
				return Decimal, offset - skip, skip
			}

			return EOF, 0, 0
		}

		window = s.reader.window()
	}
}

// complexz is a helper for numberz, handling the imaginary part recursion.
func (s *Scanner) complexz(
	offset int, window []byte,
) (byte, int, int) {
	if s.mode&Extended == 0 || s.iscomplex {
		return EOF, 0, 0
	}

	s.iscomplex = true
	ch, offset, skip := s.numberz(offset, window)
	s.iscomplex = false

	if ch == Complex {
		return Complex, offset, skip
	}

	return EOF, 0, 0
}

// escapes maps JSON/JSON5 escape characters to their decoded byte values. A
// zero entry means the escape requires special handling (e.g. 'u' for Unicode,
// or relaxed pass-through for unrecognised sequences).
var escapes = [256]byte{
	'"':  '"',
	'\'': '\'',
	'\\': '\\',
	'/':  '/',
	'b':  '\b',
	't':  '\t',
	'n':  '\n',
	'v':  '\v',
	'f':  '\f',
	'r':  '\r',
}

// quoted returns the offset past the closing quote and the unescaped content
// of the quoted string, operating entirely in-place on the reader's window
// buffer. Two indices are maintained throughout:
//
//   - offset: the next byte to be consumed from the raw input.
//   - write: the next position to emit a decoded byte to.
//
// The invariant write ≤ offset always holds because every escape sequence
// occupies ≥ 2 bytes in the input and decodes to ≤ 4 bytes of output (UTF-8),
// so the write pointer can never overtake the offset pointer. The returned
// slice is a direct sub-slice of the window buffer (zero allocation).
func (s *Scanner) quoted(quote byte) (byte, []byte, int) {
	window := s.reader.window()
	offset, write := 1, 1

	for {
		if offset >= len(window) {
			if s.reader.extend() == 0 {
				return EOF, nil, 0
			}
			window = s.reader.window()
			continue
		}

		char := window[offset]
		offset++

		switch char {
		case quote:
			return String, window[1:write], offset

		case Escape:
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0
				}
				window = s.reader.window()
			}
			escape := window[offset]
			offset++

			if decoded := escapes[escape]; decoded != 0 {
				window[write] = decoded
				write++
			} else if escape == 'u' {
				var r rune
				var ok bool
				r, offset, window, ok = s.unicode(offset, window)
				if !ok {
					return EOF, nil, 0
				}
				// write + utf8.UTFMax ≤ read: safe to encode in-place.
				write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
			} else {
				// Relaxed: unrecognised escape passes through as '\' + byte.
				window[write] = Escape
				window[write+1] = escape
				write += 2
			}

		default:
			window[write] = char
			write++
		}
	}
}

// quoted_ decodes a quoted string in-place and returns:
//
//   - the offset just past the closing quote,
//   - the number of newline characters consumed,
//   - the final character position after the last consumed newline, and
//   - the decoded content of the quoted string, operating in-place on the
//     window buffer of the reader.
//
// The function handles standard escapes, Unicode escapes, and relaxed unknown
// escapes while updating line/char counters as bytes are consumed.
//
// quoted_ is a variant of `quoted` that supports calculating the token
// location in the stream, without the need to manually track newlines and
// escapes in the stream. However, this comes with a performance cost of
// roughly 20%.
func (s *Scanner) quoted_(quote byte) (byte, []byte, int, int, int) {
	window := s.reader.window()
	offset, write, line, char := 1, 1, 0, 1

	for {
		if offset >= len(window) {
			if s.reader.extend() == 0 {
				return EOF, nil, 0, 0, 0
			}
			window = s.reader.window()
			continue
		}

		ch := window[offset]
		offset++
		char++

		switch ch {
		case quote:
			return String, window[1:write], offset, line, char

		case Escape:
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0, 0, 0
				}
				window = s.reader.window()
			}
			escape := window[offset]
			offset++
			char++

			if decoded := escapes[escape]; decoded != 0 {
				window[write] = decoded
				write++
			} else if escape == 'u' {
				var r rune
				var ok bool
				start := offset
				r, offset, window, ok = s.unicode(offset, window)
				if !ok {
					return EOF, nil, 0, 0, 0
				}
				char += offset - start
				// write + utf8.UTFMax ≤ read: safe to encode in-place.
				write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
			} else {
				// Relaxed: unrecognised escape passes through as '\\' + byte.
				window[write] = Escape
				window[write+1] = escape
				write += 2
			}

		case '\n':
			line++
			char = 0
			window[write] = ch
			write++

		default:
			window[write] = ch
			write++
		}
	}
}

// relaxed is a non-standard extension that allows to parse unquoted strings,
// stopping at the first JSON delimiter (e.g. comma, closing brace/bracket) or
// the end of the input. The function returns the
//
//   - the offset past the end of the unquoted string token,
//   - the decoded content of the unquoted string, operating in-place on the
//     window buffer of the reader.
//
// relaxed returns EOF as offset if there is no closing delimiter before the
// end of the byte stream.
//
// Note: relaxed operates in-place on the reader window, stopping at a JSON
// delimiter, stripping trailing whitespace, and decoding escape sequences
// (including \\uXXXX) identically to quoted. Single- and double-quote
// characters are treated as ordinary content characters. Leading whitespace
// must be stripped by the caller before invoking relaxed.
//
// FIXME: check white space and comment handling in relaxed mode.
func (s *Scanner) relaxed() (byte, []byte, int) {
	window := s.reader.window()
	offset, write, last := 0, 0, 0

loop:
	for {
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				break loop
			}
			window = s.reader.window()
		}

		if bytes, _, extend := uspace(window, offset); bytes > 0 {
			copy(window[write:write+bytes], window[offset:offset+bytes])
			write += bytes
			offset += bytes
			continue
		} else if extend {
			if s.reader.extend() == 0 {
				return EOF, nil, 0
			}
			window = s.reader.window()
			continue
		}

		ch := window[offset]
		switch mask[ch] & base {
		case delim:
			break loop

		case space:
			// Copy space (self-copy before first escape, real copy after);
			// do not update last — trailing spaces are excluded from output.
			window[write] = ch
			write++
			offset++

		case escape:
			offset++ // skip '\'
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0
				}
				window = s.reader.window()
			}
			esc := window[offset]
			offset++
			if decoded := escapes[esc]; decoded != 0 {
				window[write] = decoded
				write++
			} else if esc == 'u' {
				var r rune
				var ok bool
				r, offset, window, ok = s.unicode(offset, window)
				if !ok {
					return EOF, nil, 0
				}
				// write + utf8.UTFMax ≤ offset: safe to encode in-place.
				write += utf8.EncodeRune(
					window[write:write+utf8.UTFMax], r)
			} else {
				window[write] = Escape
				window[write+1] = esc
				write += 2
			}
			last = write

		default:
			window[write] = ch
			write++
			last = write
			offset++
		}
	}

	return String, window[:last], offset
}

// relaxed_ is a non-standard extension that allows to parse unquoted strings,
// stopping at the first JSON delimiter (e.g. comma, closing brace/bracket) or
// the end of the input. The function returns the
//
//   - the offset past the end of the unquoted string token,
//   - the number of newline characters consumed,
//   - the final character position after the last consumed newline, and
//   - the decoded content of the unquoted string, operating in-place on the
//     window buffer of the reader.
//
// relaxed_ returns EOF as offset if there is no closing delimiter before the
// end of the byte stream.
//
// Note: relaxed_ operates in-place on the reader window, stopping at a JSON
// delimiter, stripping trailing whitespace, and decoding escape sequences
// (including \\uXXXX) identically to quoted. Single- and double-quote
// characters are treated as ordinary content characters. Leading whitespace
// must be stripped by the caller before invoking relaxed_.
//
// relaxed_ is a variant of `relaxed` that supports calculating the token
// location in the stream, without the need to manually track newlines and
// escapes in the stream. However, this comes with a performance cost of
// roughly 20%.
func (s *Scanner) relaxed_() (byte, []byte, int, int, int) {
	window := s.reader.window()
	offset, write, line, char, last := 0, 0, 0, 0, 0

loop:
	for {
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				break loop
			}
			window = s.reader.window()
		}

		if bytes, newline, extend := uspace(window, offset); bytes > 0 {
			copy(window[write:write+bytes], window[offset:offset+bytes])
			write += bytes
			offset += bytes
			if newline {
				line++
				char = 0
			} else {
				char++
			}
			continue
		} else if extend {
			if s.reader.extend() == 0 {
				return EOF, nil, 0, 0, 0
			}
			window = s.reader.window()
			continue
		}

		ch := window[offset]
		kind := mask[ch] & base
		if kind == delim {
			break loop
		}

		offset++
		char++

		switch kind {
		case space:
			window[write] = ch
			write++
			if ch == '\n' {
				line++
				char = 0
			}

		case escape:
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0, 0, 0
				}
				window = s.reader.window()
			}
			esc := window[offset]
			offset++
			char++

			if decoded := escapes[esc]; decoded != 0 {
				window[write] = decoded
				write++
			} else if esc == 'u' {
				var r rune
				var ok bool
				start := offset
				r, offset, window, ok = s.unicode(offset, window)
				if !ok {
					return EOF, nil, 0, 0, 0
				}
				char += offset - start
				write += utf8.EncodeRune(
					window[write:write+utf8.UTFMax], r)
			} else {
				window[write] = Escape
				window[write+1] = esc
				write += 2
			}
			last = write

		default:
			window[write] = ch
			write++
			last = write
		}
	}

	return String, window[:last], offset, line, char
}

// unicode decodes a \uXXXX escape sequence (and the optional surrogate pairs
// \uXXXX\uXXXX) from the reader window starting at offset. It extends the
// window as needed and returns the decoded rune, the updated offset, the
// (possibly refreshed) window, and an ok flag.
func (s *Scanner) unicode(
	offset int, window []byte,
) (rune, int, []byte, bool) {
	for offset+4 > len(window) {
		if s.reader.extend() == 0 {
			return 0, offset, window, false
		}
		window = s.reader.window()
	}

	rune, ok := unihex(window[offset:])
	if !ok {
		return 0, offset, window, false
	}

	offset += 4
	if rune >= 0xD800 && rune <= 0xDBFF {
		for offset+6 > len(window) {
			if s.reader.extend() == 0 {
				break
			}
			window = s.reader.window()
		}

		if offset+6 <= len(window) &&
			window[offset] == Escape && window[offset+1] == 'u' {
			if r2, ok2 := unihex(window[offset+2:]); ok2 &&
				r2 >= 0xDC00 && r2 <= 0xDFFF {
				rune = 0x10000 + (rune-0xD800)<<10 + (r2 - 0xDC00)
				offset += 6
			}
		}
	}

	return rune, offset, window, true
}
