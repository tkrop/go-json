package json

import (
	"bytes"
	"io"
)

// Tuning constants for Reader.
const (
	// newBufferSize is the number of bytes to allocate for a new buffer when
	// extending the buffer. It is used to avoid frequent small allocations.
	newBufferSize = 4096
	// minReadSize is the minimum number of bytes to read from the underlying
	// reader when extending the buffer. It is used to avoid frequent small
	// reads.
	minReadSize = newBufferSize >> 2
)

// Position is a stream position tuple counted by the reader containing the
// number of bytes and lines from the start of the input data, and the number
// of characters read from the start of the line.
type Position struct {
	// Number of bytes read from the start of the input data.
	Byte int
	// Number of lines read from the start of the input data.
	Line int
	// Number of characters read since the start of the current line.
	Char int
}

// Reader implements a dynamic buffered `Reader` that reads from an underlying
// `io.Reader` as soon as the buffer is exhausted, while tracking the current
// stream position in bytes, lines, and characters since line start. The buffer
// is extended as needed dynamically when more bytes are required to store the
// current token.
//
// The `Reader` supports the following two modes:
//
//  1. In a breathing mode, where it keeps a dynamic input buffer containing
//     primarily the current token and its surrounding context, and
//  2. In a growing only mode where, it keeps the entire input data in memory
//     and allows to access any part of it at any time.
//
// The Reader is used by the Scanner to read the input data and track the
// current stream position.
type Reader struct {
	// buffer is the dynamic input buffer containing the current token and its
	// surrounding context. It is extended as needed when reading from the
	// underlying reader.
	buffer []byte
	// reader is the underlying io.Reader from which the Reader reads data.
	reader io.Reader
	// breath indicates whether the reader is in breathing or in growing only
	// mode.
	breath bool

	// new is the number of bytes to allocate for a new buffer when extending
	// the buffer. It is used to avoid frequent small allocations.
	new int
	// min is the minimum number of bytes to read from the underlying reader
	// when extending the buffer. It is used to avoid frequent small reads.
	min int

	// offset is the current offset in the buffer, indicating the start of the
	// current token. It is updated when releasing bytes from the buffer.
	offset int
	// byte is the current byte position in the stream, counting from the start
	// of the input data. It is updated when releasing bytes from the buffer.
	byte int
	// line is the current line position in the stream, counting from the start
	// of the input data. It is updated when releasing bytes from the buffer.
	line int
	// char is the current character position in the stream, counting from the
	// start of the line. It is updated when releasing bytes from the buffer.
	char int
	// err is the last error encountered when reading from the underlying
	// reader.
	err error
}

// NewReader creates a new dynamic buffered `Reader` that reads from the given
// `io.Reader` as soon as the buffer is exhausted, while tracking the current
// stream position in bytes, lines, and characters since line start. The buffer
// is extended as needed dynamically when more bytes are required to store the
// current token.
//
// The `Reader` supports the following two modes:
//
//  1. In a breathing mode, where it keeps a dynamic input buffer containing
//     primarily the current token and its surrounding context, and
//  2. In a growing only mode where, it keeps the entire input data in memory
//     and allows to access any part of it at any time.
//
// The `Reader` is used by the Scanner to read the input data and track the
// current stream position.
func NewReader(buffer []byte, reader io.Reader) *Reader {
	return &Reader{
		buffer: buffer, reader: reader, breath: true,
		new: newBufferSize, min: minReadSize,
	}
}

// release discards n bytes from the front of the window, updating the line
// and column position by counting newlines in the released bytes.
func (b *Reader) release(num int) {
	for _, c := range b.buffer[b.offset : b.offset+num] {
		if c == '\n' {
			b.line++
			b.char = 0
		} else {
			b.char++
		}
	}
	b.offset += num
	b.byte += num
}

// advance discards n bytes from the front of the window, applying the
// pre-computed line and char deltas supplied by the caller.
func (b *Reader) advance(offset int) {
	b.offset += offset
	b.byte += offset
}

// advance_ discards n bytes from the front of the window, applying the
// pre-computed line and char deltas supplied by the caller.
func (b *Reader) advance_(offset, line, char int) {
	b.line += line
	if line == 0 {
		b.char += char
	} else {
		b.char = char
	}
	b.offset += offset
	b.byte += offset
}

// position returns the current stream position.
func (b *Reader) position() Position {
	return Position{
		Byte: b.byte,
		Line: b.line,
		Char: b.char,
	}
}

// window returns the current window.
// The window is invalidated by calls to release or extend.
func (b *Reader) window() []byte {
	return b.buffer[b.offset:]
}

// peek returns the byte at the given window offset, extending the buffer as
// needed. It returns false when the underlying reader is exhausted first.
func (b *Reader) peek() (byte, bool) {
	for {
		if b.offset < len(b.buffer) {
			return b.buffer[b.offset], true
		}

		if b.extend() == 0 {
			return EOF, false
		}
	}
}

// get returns the byte at the given window offset, extending the buffer as
// needed. It returns false when the underlying reader is exhausted first.
//
// FIXME: merge with peek?
func (b *Reader) get(offset int) (byte, bool) {
	for {
		index := b.offset + offset
		if index < len(b.buffer) {
			return b.buffer[index], true
		}

		if b.extend() == 0 {
			return EOF, false
		}
	}
}

// compare checks if expect is present at the given window offset, extending
// the buffer as needed to read the full expected slice. It returns false on
// mismatch or early EOF.
func (b *Reader) compare(offset int, expect []byte) bool {
	need := offset + len(expect)
	window := b.window()
	for len(window) < need {
		if b.extend() == 0 {
			return false
		}
		window = b.window()
	}

	return bytes.Equal(window[offset:need], expect)
}

// skip releases leading scanner whitespace from the current window while
// keeping line and character positions in sync with Unicode whitespace and
// return the number of bytes skipped. It extends the window as needed until
// a non-space byte is encountered.
//
//nolint:gocognit // prioritize performance.
func (b *Reader) skip() int {
	offset, line, char := 0, 0, 0
	for {
		window := b.window()

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
			} else if offset > 0 {
				b.advance_(offset, line, char)
			}

			return offset
		}

		if b.extend() == 0 {
			b.advance_(offset, line, char)
			return offset
		}
	}
}

// extend extends the window with data from the underlying reader.
func (b *Reader) extend() int {
	if b.err != nil {
		return 0
	}

	remain := len(b.buffer) - b.offset
	if b.breath { //nolint:nestif // prioritize performance.
		if remain == 0 {
			b.buffer = b.buffer[:0]
			b.offset = 0
		}

		if cap(b.buffer)-len(b.buffer) < b.min {
			if cap(b.buffer)-remain >= b.min {
				// buffer has enough space if we move the data to the front.
				copy(b.buffer, b.buffer[b.offset:])
				b.offset = 0
			} else {
				// otherwise, we must allocate/extend a new buffer
				buffer := make([]byte, max(cap(b.buffer)*2, b.new))
				copy(buffer, b.buffer[b.offset:])
				b.buffer = buffer
				b.offset = 0
			}
		}

		remain += b.offset
	} else {
		if cap(b.buffer)-len(b.buffer) < b.min {
			// otherwise, we must allocate/extend a new buffer
			buffer := make([]byte, max(cap(b.buffer)*2, b.new))
			copy(buffer, b.buffer)
			b.buffer = buffer
		}

		remain = len(b.buffer)
	}

	offset, err := b.reader.Read(b.buffer[remain:cap(b.buffer)])
	// reduce length to the existing plus the data we read.
	b.buffer = b.buffer[:remain+offset]
	b.err = err

	return offset
}
