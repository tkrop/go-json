//nolint:gocognit,gocyclo,cyclop,nestif,funlen,maintidx // priority speed.
//revive:disable:function-length -- priority speed.
//revive:disable:cognitive-complexity -- priority speed.
//revive:disable:max-control-nesting -- priority speed.
//revive:disable:cyclomatic -- priority speed.
package json

import (
	"bytes"
	"unicode/utf8"
)

// unicodex is an alternative implementation of Unicode that uses bitmask
// comparisons instead of two-sided range checks for surrogate detection:
//
//	high surrogates D800–DBFF: (r & 0xFC00) == 0xD800  (top 6 bits = 110110)
//	low  surrogates DC00–DFFF: (r & 0xFC00) == 0xDC00  (top 6 bits = 110111)
//
// Each range check becomes a single AND + equality, eliminating the second
// comparison entirely when the first condition fails.
func (s *Scanner) unicodex(
	offset int, window []byte,
) (rune, int, []byte, bool) {
	for offset+4 > len(window) {
		if s.reader.extend() == 0 {
			return 0, offset, window, false
		}
		window = s.reader.window()
	}

	r, ok := unihex(window[offset:])
	if !ok {
		return 0, offset, window, false
	}

	offset += 4
	// High surrogate: top 6 bits == 110110 (0xD800–0xDBFF).
	if r&0xFC00 == 0xD800 {
		for offset+6 > len(window) {
			if s.reader.extend() == 0 {
				break
			}
			window = s.reader.window()
		}

		if offset+6 <= len(window) &&
			window[offset] == Escape && window[offset+1] == 'u' {
			if r2, ok := unihex(window[offset+2:]); ok && r2&0xFC00 == 0xDC00 {
				r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
				offset += 6
			}
		}
	}

	return r, offset, window, true
}

// quotedx scans quoted strings using a hybrid approach:
//
//   - Bulk phase: it searches for the next quote or backslash via IndexByte,
//     copying plain chunks with copy.
//   - Escape phase: after decoding one escape sequence, it probes one next
//     byte. If that byte is another backslash, it immediately decodes the next
//     escape; otherwise, it resumes the bulk phase.
//
// This keeps the fast path for long plain runs while reducing overhead for
// dense escape clusters.
func (s *Scanner) quotedx(quote byte) (byte, []byte, int) {
	window := s.reader.window()
	offset, write := 1, 1

	for {
		// Bulk-scan for the next quote or backslash.
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				return EOF, nil, 0
			}
			window = s.reader.window()
		}

		chunk := window[offset:]
		qi := bytes.IndexByte(chunk, quote)
		ei := bytes.IndexByte(chunk, Escape)

		// Take the nearer result; -1 means not found.
		next := qi
		if next < 0 || (ei >= 0 && ei < next) {
			next = ei
		}

		if next < 0 {
			if write < offset {
				copy(window[write:], chunk)
			}
			write += len(chunk)
			offset = len(window)
			continue
		}

		if write < offset {
			copy(window[write:], chunk[:next])
		}
		write += next
		offset += next

		if window[offset] == quote {
			return String, window[1:write], offset + 1
		}
		offset++ // skip past '\\'

		// offset points just past a backslash. Read and decode the escape char.
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
			write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
		} else {
			window[write] = Escape
			window[write+1] = escape
			write += 2
		}

		// Probe the next character after escape.
		for {
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0
				}
				window = s.reader.window()
			}
			char := window[offset]
			if char == quote {
				return String, window[1:write], offset + 1
			}
			if char == Escape {
				// Decode next escape, then probe again.
				offset++
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
					write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
				} else {
					window[write] = Escape
					window[write+1] = escape
					write += 2
				}
				// Continue probing after escape.
				continue
			}
			// Not a backslash: copy and resume bulk scan.
			window[write] = char
			write++
			offset++
			break
		}
	}
}

// quoted_x scans quoted strings with a hybrid two-phase algorithm and returns
// the decoded token plus relative line and char deltas:
//
//   - Bulk phase: find the next quote or backslash with IndexByte and copy the
//     plain chunk in one step while updating line/char counters from the chunk.
//   - Escape/probe phase: decode one escape sequence, then probe subsequent
//     bytes one-by-one. Consecutive escapes stay in this phase; a plain byte is
//     copied and control returns to the bulk phase.
//
// This keeps long plain runs fast while still handling dense escape clusters
// and accurate position tracking.
func (s *Scanner) quoted_x(quote byte) (byte, []byte, int, int, int) {
	window := s.reader.window()
	offset, write, lines, chars := 1, 1, 0, 1

	for {
		// Bulk-scan for the next quote or backslash.
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				return EOF, nil, 0, 0, 0
			}
			window = s.reader.window()
		}

		chunk := window[offset:]
		qi := bytes.IndexByte(chunk, quote)
		ei := bytes.IndexByte(chunk, Escape)

		next := qi
		if next < 0 || (ei >= 0 && ei < next) {
			next = ei
		}

		if next < 0 {
			if write < offset {
				copy(window[write:], chunk)
			}

			partLines, partChars := count(chunk)
			lines += partLines
			if partLines == 0 {
				chars += partChars
			} else {
				chars = partChars
			}

			write += len(chunk)
			offset = len(window)
			continue
		}

		plain := chunk[:next]
		if write < offset {
			copy(window[write:], plain)
		}

		partLines, partChars := count(plain)
		lines += partLines
		if partLines == 0 {
			chars += partChars
		} else {
			chars = partChars
		}

		write += next
		offset += next

		if window[offset] == quote {
			offset++
			chars++

			return String, window[1:write], offset, lines, chars
		}

		offset++ // skip '\\'
		chars++

		// offset points just past a backslash. Read and decode the escape char.
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				return EOF, nil, 0, 0, 0
			}
			window = s.reader.window()
		}
		escape := window[offset]
		offset++
		chars++

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
			chars += offset - start
			write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
		} else {
			window[write] = Escape
			window[write+1] = escape
			write += 2
		}

		// Probe the next character after escape.
		for {
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0, 0, 0
				}
				window = s.reader.window()
			}

			char := window[offset]
			if char == quote {
				offset++
				chars++

				return String, window[1:write], offset, lines, chars
			}

			if char == Escape {
				// Decode next escape, then probe again.
				offset++
				chars++

				for offset >= len(window) {
					if s.reader.extend() == 0 {
						return EOF, nil, 0, 0, 0
					}
					window = s.reader.window()
				}
				escape := window[offset]
				offset++
				chars++

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
					chars += offset - start
					write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
				} else {
					window[write] = Escape
					window[write+1] = escape
					write += 2
				}

				continue
			}

			// Not a backslash: copy one char and resume bulk scan.
			if char == '\n' {
				lines++
				chars = 0
			} else {
				chars++
			}
			window[write] = char
			write++
			offset++

			break
		}
	}
}

// relaxedx returns the offset past the end of the unquoted string token and
// its decoded content using a two-phase approach:
//
//   - Bulk phase: use bytes.IndexAny to locate the next delimiter, space, or
//     escape in one call, copying the plain chunk with copy.
//   - Probe phase: after decoding one escape sequence, scan subsequent bytes
//     one-by-one; consecutive escapes stay in this phase; the first non-escape
//     byte is copied and control returns to the bulk phase.
//
// This keeps long plain runs fast while handling dense escape clusters
// efficiently, matching the strategy used by quotedx.
func (s *Scanner) relaxedx() (byte, []byte, int) {
	window := s.reader.window()
	offset, write, last := 0, 0, 0

	for {
		// Bulk phase: find next delimiter, space, or escape.
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				return String, window[:last], offset
			}
			window = s.reader.window()
		}

		chunk := window[offset:]
		i := index(chunk)
		if i < 0 {
			if write < offset {
				copy(window[write:], chunk)
			}
			write += len(chunk)
			last = trim(window, write)
			offset = len(window)
			continue
		}

		plain := chunk[:i]
		if i > 0 && write < offset {
			copy(window[write:], plain)
		}
		write += i
		if i > 0 {
			last = trim(window, write)
		}
		offset += i

		bytes, _, extend := uspace(window, offset)
		if bytes > 0 {
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

		char := window[offset]
		switch mask[char] & base {
		case delim:
			return String, window[:last], offset
		case space:
			window[write] = char
			write++
			offset++
			continue
		case escape:
			offset++ // skip '\\'
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
				write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
			} else {
				window[write] = Escape
				window[write+1] = esc
				write += 2
			}
			last = write

			// Probe phase: scan one-by-one for further escapes.
			for {
				for offset >= len(window) {
					if s.reader.extend() == 0 {
						return String, window[:last], offset
					}
					window = s.reader.window()
				}

				bytes, _, extend := uspace(window, offset)
				if bytes > 0 {
					copy(window[write:write+bytes],
						window[offset:offset+bytes])
					write += bytes
					offset += bytes
					break
				} else if extend {
					if s.reader.extend() == 0 {
						return EOF, nil, 0
					}
					window = s.reader.window()
					continue
				}

				char := window[offset]
				if mask[char]&base == delim {
					return String, window[:last], offset
				}
				if mask[char]&base == escape {
					offset++ // skip '\\'
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
						write += utf8.EncodeRune(
							window[write:write+utf8.UTFMax], r)
					} else {
						window[write] = Escape
						window[write+1] = esc
						write += 2
					}
					last = write
					continue
				}

				// Any other char (space or plain): copy one and return to bulk.
				window[write] = char
				write++
				if mask[char]&base != space {
					last = write
				}
				offset++
				break
			}

		default:
			window[write] = char
			write++
			last = write
			offset++
			continue
		}
	}
}

// relaxed_x returns the offset past the end of the unquoted string token and
// its decoded content using a two-phase approach with position tracking:
//
//   - Bulk phase: use bytes.IndexAny to locate the next delimiter, space, or
//     escape in one call, copying the plain chunk with copy while counting
//     lines and chars via count.
//   - Probe phase: after decoding one escape sequence, scan subsequent bytes
//     one-by-one; consecutive escapes stay in this phase; the first non-escape
//     byte is copied and control returns to the bulk phase.
//
// relaxed_x is a variant of `relaxedx` that supports calculating the token
// location in the stream, without the need to manually track newlines and
// escapes in the stream. However, this comes with a performance cost.
func (s *Scanner) relaxed_x() (byte, []byte, int, int, int) {
	window := s.reader.window()
	offset, write, lines, chars, last := 0, 0, 0, 0, 0

	for {
		// Bulk phase: find next delimiter, space, or escape.
		for offset >= len(window) {
			if s.reader.extend() == 0 {
				return String, window[:last], offset, lines, chars
			}
			window = s.reader.window()
		}

		chunk := window[offset:]
		i := index(chunk)
		if i < 0 {
			if write < offset {
				copy(window[write:], chunk)
			}
			partLines, partChars := count_(chunk)
			lines += partLines
			chars += partChars
			write += len(chunk)
			last = trim(window, write)
			offset = len(window)
			continue
		}

		plain := chunk[:i]
		if i > 0 && write < offset {
			copy(window[write:], plain)
		}
		partLines, partChars := count_(plain)
		lines += partLines
		chars += partChars
		write += i
		if i > 0 {
			last = trim(window, write)
		}
		offset += i

		bytes, newline, extend := uspace(window, offset)
		if bytes > 0 {
			copy(window[write:write+bytes], window[offset:offset+bytes])
			write += bytes
			offset += bytes
			if newline {
				lines++
				chars = 0
			} else {
				chars++
			}
			continue
		} else if extend {
			if s.reader.extend() == 0 {
				return EOF, nil, 0, 0, 0
			}
			window = s.reader.window()
			continue
		}

		char := window[offset]
		switch mask[char] & base {
		case delim:
			return String, window[:last], offset, lines, chars
		case space:
			if char == '\n' {
				lines++
				chars = 0
			} else {
				chars++
			}
			window[write] = char
			write++
			offset++
			continue
		case escape:
			offset++ // skip '\\'
			chars++
			for offset >= len(window) {
				if s.reader.extend() == 0 {
					return EOF, nil, 0, 0, 0
				}
				window = s.reader.window()
			}
			esc := window[offset]
			offset++
			chars++

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
				chars += offset - start
				write += utf8.EncodeRune(window[write:write+utf8.UTFMax], r)
			} else {
				window[write] = Escape
				window[write+1] = esc
				write += 2
			}
			last = write

			// Probe phase: scan one-by-one for further escapes.
			for {
				for offset >= len(window) {
					if s.reader.extend() == 0 {
						return String, window[:last], offset, lines, chars
					}
					window = s.reader.window()
				}

				bytes, newline, extend := uspace(window, offset)
				if bytes > 0 {
					copy(window[write:write+bytes],
						window[offset:offset+bytes])
					write += bytes
					offset += bytes
					if newline {
						lines++
						chars = 0
					} else {
						chars++
					}
					break
				} else if extend {
					if s.reader.extend() == 0 {
						return EOF, nil, 0, 0, 0
					}
					window = s.reader.window()
					continue
				}

				char := window[offset]
				if mask[char]&base == delim {
					return String, window[:last], offset, lines, chars
				}
				if mask[char]&base == escape {
					offset++ // skip '\\'
					chars++
					for offset >= len(window) {
						if s.reader.extend() == 0 {
							return EOF, nil, 0, 0, 0
						}
						window = s.reader.window()
					}
					esc := window[offset]
					offset++
					chars++
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
						chars += offset - start
						write += utf8.EncodeRune(
							window[write:write+utf8.UTFMax], r)
					} else {
						window[write] = Escape
						window[write+1] = esc
						write += 2
					}
					last = write
					continue
				}
				// Any other char (space or plain): copy one and return to bulk.
				if char == '\n' {
					lines++
					chars = 0
				} else {
					chars++
				}
				window[write] = char
				write++
				if mask[char]&base != space {
					last = write
				}
				offset++
				break
			}

		default:
			window[write] = char
			write++
			last = write
			offset++
			chars++
			continue
		}
	}
}

// index returns the offset of the first delimiter, space, or escape in the
// chunk, or -1 if none is found. Delimiters are defined as any of the
// following characters: space, tab, newline, carriage return, comma, colon,
// closing curly brace, closing square bracket, opening curly brace, opening
// square bracket, backslash, or any of the UTF-8 encoded whitespace
// characters: U+00A0 (0xC2 0xA0), U+FEFF (0xEF 0xBB 0xBF), U+2028 (0xE2 0x80
// 0xA8), and U+2029 (0xE2 0x80 0xA9).
func index(chunk []byte) int {
	const delims = " \t\n\r,:}]{[\\\v\f"

	index := bytes.IndexAny(chunk, delims)
	for _, char := range []byte{0xC2, 0xE2, 0xEF} {
		if next := bytes.IndexByte(chunk, char); next >= 0 &&
			(index < 0 || next < index) {
			index = next
		}
	}

	return index
}

// count counts source newlines and chars since the last newline.
//
// TODO: this looks like a hot spot in quoted_x. Consider optimizing with SIMD
// or a small table of byte->(isNewline, charCount) for 16 or 32 bytes at a
// time.
func count(buffer []byte) (int, int) {
	lines, last := 0, -1

	for index, char := range buffer {
		if char == '\n' {
			lines++
			last = index
		}
	}

	if last >= 0 {
		return lines, len(buffer) - last - 1
	}

	return 0, len(buffer)
}

// count_ counts source newlines and chars, treating
// U+000A/U+2028/U+2029 as line separators.
func count_(buffer []byte) (int, int) {
	lines, chars := 0, 0

	for index := 0; index < len(buffer); {
		if buffer[index] == '\n' {
			lines++
			chars = 0
			index++
			continue
		}

		if index+3 <= len(buffer) &&
			buffer[index] == 0xE2 &&
			buffer[index+1] == 0x80 &&
			(buffer[index+2] == 0xA8 || buffer[index+2] == 0xA9) {
			lines++
			chars = 0
			index += 3
			continue
		}

		if index+3 <= len(buffer) &&
			buffer[index] == 0xEF &&
			buffer[index+1] == 0xBB &&
			buffer[index+2] == 0xBF {
			chars++
			index += 3
			continue
		}

		if index+2 <= len(buffer) &&
			buffer[index] == 0xC2 &&
			buffer[index+1] == 0xA0 {
			chars++
			index += 2
			continue
		}

		chars++
		index++
	}

	return lines, chars
}

// trim trims trailing ASCII space and supported UTF-8 spaces from end.
func trim(buffer []byte, end int) int {
	for end > 0 {
		if mask[buffer[end-1]]&base == space {
			end--
			continue
		}

		if end >= 2 &&
			buffer[end-2] == 0xC2 &&
			buffer[end-1] == 0xA0 {
			end -= 2
			continue
		}

		if end >= 3 &&
			buffer[end-3] == 0xEF &&
			buffer[end-2] == 0xBB &&
			buffer[end-1] == 0xBF {
			end -= 3
			continue
		}

		if end >= 3 &&
			buffer[end-3] == 0xE2 &&
			buffer[end-2] == 0x80 &&
			(buffer[end-1] == 0xA8 || buffer[end-1] == 0xA9) {
			end -= 3
			continue
		}

		break
	}

	return end
}
