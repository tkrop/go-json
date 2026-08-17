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
	// mode controls buffer retention and parsing behavior.
	mode Mode

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
//  1. In a default breathing mode, where it keeps a dynamic input buffer
//     containing only the current token and its surrounding context, and
//  2. In a retaining mode where, it keeps the entire input data in memory
//     and allows to access any part of the buffer at any time.
//
// The `Reader` is used by the Scanner to read the input data and track the
// current stream position.
func NewReader(buffer []byte, reader io.Reader) *Reader {
	return &Reader{
		buffer: buffer, reader: reader, mode: Evict,
		new: newBufferSize, min: minReadSize,
	}
}

// Mode sets the mode used for reading.
func (r *Reader) Mode(mode Mode) *Reader {
	r.mode = mode
	return r
}

// release discards n bytes from the front of the window, updating the line
// and column position by counting newlines in the released bytes.
func (r *Reader) release(num int) {
	for _, c := range r.buffer[r.offset : r.offset+num] {
		if c == '\n' {
			r.line++
			r.char = 0
		} else {
			r.char++
		}
	}
	r.offset += num
	r.byte += num
}

// advance discards n bytes from the front of the window, applying the
// pre-computed line and char deltas supplied by the caller.
func (r *Reader) advance(offset int) {
	r.offset += offset
	r.byte += offset
}

// advance_ discards n bytes from the front of the window, applying the
// pre-computed line and char deltas supplied by the caller.
func (r *Reader) advance_(offset, line, char int) {
	r.line += line
	if line == 0 {
		r.char += char
	} else {
		r.char = char
	}
	r.offset += offset
	r.byte += offset
}

// position returns the current stream position.
func (r *Reader) position() Position {
	return Position{
		Byte: r.byte,
		Line: r.line,
		Char: r.char,
	}
}

// window returns the current window.
// The window is invalidated by calls to release or extend.
func (r *Reader) window() []byte {
	return r.buffer[r.offset:]
}

// peek returns the byte at the given window offset, extending the buffer as
// needed. It returns false when the underlying reader is exhausted first.
func (r *Reader) peek() (byte, bool) {
	for {
		if r.offset < len(r.buffer) {
			return r.buffer[r.offset], true
		}

		if r.extend() == 0 {
			return EOF, false
		}
	}
}

// get returns the byte at the given window offset, extending the buffer as
// needed. It returns false when the underlying reader is exhausted first.
//
// FIXME: merge with peek?
func (r *Reader) get(offset int) (byte, bool) {
	for {
		index := r.offset + offset
		if index < len(r.buffer) {
			return r.buffer[index], true
		}

		if r.extend() == 0 {
			return EOF, false
		}
	}
}

// compare checks if expect is present at the given window offset, extending
// the buffer as needed to read the full expected slice. It returns false on
// mismatch or early EOF.
func (r *Reader) compare(offset int, expect []byte) bool {
	need := offset + len(expect)
	window := r.window()
	for len(window) < need {
		if r.extend() == 0 {
			return false
		}
		window = r.window()
	}

	return bytes.Equal(window[offset:need], expect)
}

// skip releases leading scanner whitespace from the current window while
// keeping line and character positions in sync with Unicode whitespace and
// return the number of bytes skipped. It extends the window as needed until
// a non-space byte is encountered.
//
//nolint:gocognit // prioritize performance.
//revive:disable-next-line:cognitive-complexity // prioritize performance.
func (r *Reader) skip() int {
	offset, line, char := 0, 0, 0
	for {
		window := r.window()

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
				r.advance_(offset, line, char)
			}

			return offset
		}

		if r.extend() == 0 {
			r.advance_(offset, line, char)
			return offset
		}
	}
}

// extend extends the window with data from the underlying reader.
func (r *Reader) extend() int {
	if r.err != nil {
		return 0
	}

	remain := len(r.buffer) - r.offset
	//nolint:nestif // prioritize performance.
	if r.mode&Retain == Retain {
		if cap(r.buffer)-len(r.buffer) < r.min {
			// Otherwise, we must allocate/extend a new buffer
			buffer := make([]byte, max(cap(r.buffer)*2, r.new))
			copy(buffer, r.buffer)
			r.buffer = buffer
		}
		remain = len(r.buffer)
	} else {
		if remain == 0 {
			r.buffer = r.buffer[:0]
			r.offset = 0
		}

		if cap(r.buffer)-len(r.buffer) < r.min {
			if cap(r.buffer)-remain >= r.min {
				// buffer has enough space if we move the data to the front.
				copy(r.buffer, r.buffer[r.offset:])
			} else {
				// otherwise, we must allocate/extend a new buffer
				buffer := make([]byte, max(cap(r.buffer)*2, r.new))
				copy(buffer, r.buffer[r.offset:])
				r.buffer = buffer
			}
			r.offset = 0
		}
		remain += r.offset
	}

	offset, err := r.reader.Read(r.buffer[remain:cap(r.buffer)])
	// reduce length to the existing plus the data we read.
	r.buffer = r.buffer[:remain+offset]
	r.err = err

	return offset
}
