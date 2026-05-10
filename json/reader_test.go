package json

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tkrop/go-testing/test"
)

// state represents the internal state of a byteReader, used for testing
// purposes.
type state struct {
	offset int
	bytes  int
	lines  int
	chars  int
}

// NewState creates a state with the given offset, bytes, lines, and chars.
func NewState(offset, bytes, lines, chars int) state {
	return state{
		offset: offset,
		bytes:  bytes,
		lines:  lines,
		chars:  chars,
	}
}

// NewReaderState creates a Reader with the given buffer, growing mode, state, and
// error.
func NewReaderState(
	buffer []byte, breath bool, new, min int, state state, err error,
) Reader {
	return Reader{
		buffer: buffer,
		breath: breath,
		new:    new,
		min:    min,
		offset: state.offset,
		byte:   state.bytes,
		line:   state.lines,
		char:   state.chars,
		err:    err,
	}
}

// stepReader is a simple io.Reader implementation that returns predefined
// data and errors in sequence, used for testing the extend method of byteReader.
type stepReader struct {
	steps []step
	index int
}

// step defines a single read step for the stepReader, containing the data to
// return and the error to return (if any).
type step struct {
	data string
	err  error
}

// Read implements the io.Reader interface for stepReader, returning the data and
// error for the current step, and advancing to the next step.
func (r *stepReader) Read(buffer []byte) (int, error) {
	if r.index >= len(r.steps) {
		return 0, io.EOF
	}

	step := r.steps[r.index]
	r.index++
	return copy(buffer, []byte(step.data)), step.err
}

// fillBuffer creates a byte slice of the specified length and capacity, filled
// with the given byte value.
func fillBuffer(length, capacity int, fill byte) []byte {
	buffer := make([]byte, length, capacity)
	for index := range buffer {
		buffer[index] = fill
	}

	return buffer
}

// releaseParams defines the parameters for testing the release method of
// byteReader.
type releaseParams struct {
	buffer string
	inputs []string
	setup  state
	expect Position
}

// releaseTestCases defines the test cases for testing the release method of
// byteReader.
var releaseTestCases = map[string]releaseParams{
	// cases without newlines.
	"no-newline": {
		inputs: []string{"hello"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 5, Line: 0, Char: 5},
	},
	"cr-only": {
		inputs: []string{"hello\r"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 6, Line: 0, Char: 6},
	},

	// single-call cases with newlines.
	"newline": {
		inputs: []string{"hello\n"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 6, Line: 1, Char: 0},
	},
	"newline-then-bytes": {
		inputs: []string{"hello\nworld"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 11, Line: 1, Char: 5},
	},
	"crlf": {
		inputs: []string{"hello\r\nworld"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 12, Line: 1, Char: 5},
	},
	"multi-newline": {
		inputs: []string{"\n\n\n"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 3, Line: 3, Char: 0},
	},

	// multi-call accumulation.
	"multi-call": {
		inputs: []string{"hello\n", "world"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 11, Line: 1, Char: 5},
	},
	"multi-call-newline": {
		inputs: []string{"a\nb", "c\nd"},
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 6, Line: 2, Char: 1},
	},
	"prefilled-state": {
		buffer: "xy\nzz",
		inputs: []string{"zz"},
		setup:  NewState(3, 10, 2, 4),
		expect: Position{Byte: 12, Line: 2, Char: 6},
	},
}

// TestRelease tests the release method of byteReader, which updates the
// position based on the number of bytes released.
func TestRelease(t *testing.T) {
	test.Map(t, releaseTestCases).
		Run(func(t test.Test, param releaseParams) {
			// Given
			data := param.buffer
			if data == "" {
				data = strings.Join(param.inputs, "")
			}

			reader := NewReaderState([]byte(data),
				true, 4, 8, param.setup, nil)

			// When
			for _, s := range param.inputs {
				reader.release(len(s))
			}

			// Then
			assert.Equal(t, param.expect, reader.position())
		})
}

// advanceParams defines the parameters for testing the advance_ method of
// byteReader.
type advanceParams struct {
	setup             state
	num, lines, chars int
	expect            Position
}

// advanceTestCases defines the test cases for testing the advance_ method of
// byteReader.
var advanceTestCases = map[string]advanceParams{
	"same-line": {
		setup: NewState(5, 7, 2, 3),
		num:   4, lines: 0, chars: 4,
		expect: Position{Byte: 11, Line: 2, Char: 7},
	},
	"line-reset": {
		setup: NewState(2, 9, 1, 8),
		num:   3, lines: 2, chars: 5,
		expect: Position{Byte: 12, Line: 3, Char: 5},
	},
	"line-reset-zero-chars": {
		setup: NewState(0, 0, 0, 0),
		num:   1, lines: 1, chars: 0,
		expect: Position{Byte: 1, Line: 1, Char: 0},
	},
}

// TestAdvance tests the advance_ method of byteReader, which updates the
// position based on the number of bytes, lines, and characters advanced.
func TestAdvance(t *testing.T) {
	test.Map(t, advanceTestCases).
		Run(func(t test.Test, param advanceParams) {
			// Given
			reader := NewReaderState(nil, true, 4, 8, param.setup, nil)

			// When
			reader.advance_(param.num, param.lines, param.chars)

			// Then
			assert.Equal(t, param.expect, reader.position())
		})
}

// advanceOffsetParams defines the parameters for testing the advance method of
// byteReader.
type advanceOffsetParams struct {
	setup  state
	offset int
	expect Position
}

// advanceOffsetTestCases defines the test cases for testing the advance method
// of byteReader.
var advanceOffsetTestCases = map[string]advanceOffsetParams{
	"no-op": {
		setup:  NewState(2, 10, 1, 3),
		offset: 0,
		expect: Position{Byte: 10, Line: 1, Char: 3},
	},
	"advance-bytes": {
		setup:  NewState(1, 5, 2, 4),
		offset: 3,
		expect: Position{Byte: 8, Line: 2, Char: 4},
	},
}

// TestAdvanceOffset tests the advance method of byteReader, which updates only
// byte and buffer offsets without changing line and character counters.
func TestAdvanceOffset(t *testing.T) {
	test.Map(t, advanceOffsetTestCases).
		Run(func(t test.Test, param advanceOffsetParams) {
			// Given
			reader := NewReaderState(nil, true, 4, 8, param.setup, nil)

			// When
			reader.advance(param.offset)

			// Then
			assert.Equal(t, param.offset, reader.offset-param.setup.offset)
			assert.Equal(t, param.expect, reader.position())
		})
}

// positionParams defines the parameters for testing the position method of
// byteReader.
type positionParams struct {
	data              string
	setup             state
	release           []int
	num, lines, chars int
	expect            Position
}

// positionTestCases defines the test cases for testing the position method of
// byteReader.
var positionTestCases = map[string]positionParams{
	"initial-state": {
		setup:  NewState(0, 0, 0, 0),
		expect: Position{Byte: 0, Line: 0, Char: 0},
	},
	"release-then-advance": {
		data:    "a\nbc",
		setup:   NewState(0, 0, 0, 0),
		release: []int{2},
		num:     1, lines: 0, chars: 1,
		expect: Position{Byte: 3, Line: 1, Char: 1},
	},
}

// TestPosition tests the position method of byteReader, which returns the
// current position based on the number of bytes, lines, and characters
// processed.
func TestPosition(t *testing.T) {
	test.Map(t, positionTestCases).
		Run(func(t test.Test, param positionParams) {
			// Given
			reader := NewReaderState([]byte(param.data),
				true, 4, 8, param.setup, nil)

			// When
			for _, num := range param.release {
				reader.release(num)
			}

			if param.num != 0 || param.lines != 0 || param.chars != 0 {
				reader.advance_(param.num, param.lines, param.chars)
			}

			// Then
			assert.Equal(t, param.expect, reader.position())
		})
}

// extendParams defines the parameters for testing the extend method of
// byteReader.
type extendParams struct {
	reader Reader
	steps  []step
	expect Reader
	num    int
}

// extendTestCases defines the test cases for testing the extend method of
// byteReader, which extends the buffer by reading from the underlying reader.
var extendTestCases = map[string]extendParams{
	"existing-error": {
		reader: NewReaderState([]byte("hello"),
			true, 4, 8, NewState(2, 11, 3, 7), io.EOF),
		expect: NewReaderState([]byte("hello"),
			true, 4, 8, NewState(2, 11, 3, 7), io.EOF),
	},
	"extend-buffer": {
		reader: NewReaderState([]byte("ab"),
			true, 4, 8, NewState(0, 0, 0, 0), nil),
		steps: []step{{data: "xy"}},
		expect: NewReaderState(
			append(make([]byte, 0, 4), []byte("abxy")...),
			true, 4, 8, NewState(0, 0, 0, 0), nil),
		num: 2,
	},
	"reset-empty-window": {
		reader: NewReaderState(fillBuffer(3, 3, 'x'),
			true, 4, 8, NewState(3, 9, 1, 4), nil),
		steps: []step{{data: "hi", err: io.EOF}},
		expect: NewReaderState(
			append(make([]byte, 0, 2), []byte("hi")...),
			true, 4, 8, NewState(0, 9, 1, 4), io.EOF),
		num: 2,
	},
	"compact-before-read": {
		reader: NewReaderState(func() []byte {
			buffer := fillBuffer(9, 16, 'x')
			copy(buffer[6:], []byte("abc"))
			return buffer
		}(), true, 4, 8, NewState(6, 4, 0, 4), nil),
		steps: []step{{data: "yz"}},
		expect: NewReaderState(func() []byte {
			return []byte("abcyz")
		}(), true, 4, 8, NewState(0, 4, 0, 4), nil),
		num: 2,
	},
	"grow-before-read": {
		reader: NewReaderState(func() []byte {
			buffer := fillBuffer(9, 13, 'x')
			copy(buffer[1:], []byte("mno"))
			return buffer
		}(), true, 4, 8, NewState(1, 6, 1, 2), nil),
		steps: []step{{data: "z", err: io.EOF}},
		expect: NewReaderState(func() []byte {
			return []byte("mnoxxxxxz")
		}(), true, 4, 8, NewState(0, 6, 1, 2), io.EOF),
		num: 1,
	},
	"shrink-mode-compacts-window": {
		reader: NewReaderState([]byte("prefixtoken"),
			true, 4, 8, NewState(6, 0, 0, 0), nil),
		steps: []step{{data: "X", err: io.EOF}},
		expect: NewReaderState([]byte("tokenX"),
			true, 4, 8, NewState(0, 0, 0, 0), io.EOF),
		num: 1,
	},
	"grow-only-mode-keeps-prefix": {
		reader: NewReaderState(
			append(make([]byte, 0, 20), []byte("prefixtoken")...),
			false, 4, 8, NewState(6, 0, 0, 0), nil),
		steps: []step{{data: "X", err: io.EOF}},
		expect: NewReaderState(func() []byte {
			return []byte("prefixtokenX")
		}(), false, 4, 8, NewState(6, 0, 0, 0), io.EOF),
		num: 1,
	},
	"grow-only-mode-grow-copies-full-buffer": {
		reader: NewReaderState(func() []byte {
			buffer := fillBuffer(9, 13, 'x')
			copy(buffer[1:], []byte("mno"))
			return buffer
		}(), false, 4, 8, NewState(1, 2, 0, 2), nil),
		steps: []step{{data: "z", err: io.EOF}},
		expect: NewReaderState(func() []byte {
			buffer := fillBuffer(26, 26, 0)
			copy(buffer, []byte("xmnoxxxxx"))
			return buffer
		}(),
			false, 4, 8, NewState(1, 2, 0, 2), io.EOF),
		num: 0,
	},
}

// TestExtend tests the extend method of byteReader, which extends the buffer
// by reading from the underlying reader, and updates the position accordingly.
func TestExtend(t *testing.T) {
	test.Map(t, extendTestCases).
		Run(func(t test.Test, param extendParams) {
			// Given
			reader := param.reader
			reader.reader = &stepReader{steps: param.steps}

			// When
			num := reader.extend()

			// Then
			assert.Equal(t, param.num, num)
			reader.reader = nil
			assert.Equal(t, param.expect, reader)

			assert.Equal(t, Position{
				Byte: param.expect.byte,
				Line: param.expect.line,
				Char: param.expect.char,
			}, reader.position())
		})
}

// skipParams defines the parameters for testing the skip method of byteReader.
type skipParams struct {
	reader Reader
	steps  []step
	skip   int
	remain string
	state  Position
	error  error
}

// skipTestCases defines the test cases for testing the skip method of
// byteReader, which skips over a specified number of bytes in the buffer.
var skipTestCases = map[string]skipParams{
	"all": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: " \t\n", err: io.EOF}},
		skip:   3,
		remain: "",
		state:  Position{Byte: 3, Line: 1, Char: 0},
		error:  io.EOF,
	},
	"none": {
		reader: NewReaderState([]byte("abc"),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		skip:   0,
		remain: "abc",
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  nil,
	},
	"offset": {
		reader: NewReaderState([]byte("zz \nabc"),
			false, 4, 8, NewState(2, 5, 1, 2), nil),
		skip:   2,
		remain: "abc",
		state:  Position{Byte: 7, Line: 2, Char: 0},
		error:  nil,
	},
	"spaces": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: " \t\nabc", err: io.EOF}},
		skip:   3,
		remain: "abc",
		state:  Position{Byte: 3, Line: 1, Char: 0},
		error:  io.EOF,
	},
	"unicodes": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "\xE2\x80"}, {data: "\xA8x", err: io.EOF}},
		skip:   3,
		remain: "x",
		state:  Position{Byte: 3, Line: 1, Char: 0},
		error:  io.EOF,
	},
	"mixed": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps: []step{
			{data: " \xC2\xA0\nabc", err: io.EOF},
		},
		skip:   4,
		remain: "abc",
		state:  Position{Byte: 4, Line: 1, Char: 0},
		error:  io.EOF,
	},
}

// TestSkip tests the skip method of byteReader, which skips over a specified
// number of bytes in the buffer, and updates the position accordingly.
func TestSkip(t *testing.T) {
	test.Map(t, skipTestCases).
		Run(func(t test.Test, param skipParams) {
			// Given
			reader := param.reader
			reader.reader = &stepReader{steps: param.steps}

			// When
			skip := reader.skip()

			// Then
			assert.Equal(t, param.remain, string(reader.window()))
			assert.Equal(t, param.skip, skip)
			assert.Equal(t, param.state, reader.position())
			assert.Equal(t, param.error, reader.err)
		})
}

// compareParams defines the parameters for testing the compare method of
// byteReader.
type compareParams struct {
	reader Reader
	steps  []step
	offset int
	expect []byte
	match  bool
	state  Position
	error  error
}

// compareTestCases defines the test cases for testing the compare method of
// byteReader, which compares the buffer content with the expected bytes at a
// given offset.
var compareTestCases = map[string]compareParams{
	"match-buffer": {
		reader: NewReaderState([]byte("prefix"),
			false, 4, 8, NewState(0, 2, 1, 3), nil),
		expect: []byte("pre"),
		match:  true,
		state:  Position{Byte: 2, Line: 1, Char: 3},
	},
	"mismatch-buffer": {
		reader: NewReaderState([]byte("prefix"),
			false, 4, 8, NewState(0, 2, 1, 3), nil),
		expect: []byte("pro"),
		match:  false,
		state:  Position{Byte: 2, Line: 1, Char: 3},
	},
	"match-extend": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "ab"}, {data: "cd", err: io.EOF}},
		expect: []byte("abcd"),
		match:  true,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
	"short-eof": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "ab", err: io.EOF}},
		expect: []byte("abc"),
		match:  false,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
}

// TestCompare tests the compare method of byteReader, which compares the
// buffer content with the expected bytes at a given offset.
func TestCompare(t *testing.T) {
	test.Map(t, compareTestCases).
		Run(func(t test.Test, param compareParams) {
			// Given
			reader := param.reader
			reader.reader = &stepReader{steps: param.steps}

			// When
			match := reader.compare(param.offset, param.expect)

			// Then
			assert.Equal(t, param.match, match)
			assert.Equal(t, param.state, reader.position())
			assert.Equal(t, param.error, reader.err)
		})
}

// accessParams defines the parameters for testing byteReader byte access
// methods such as peek and get.
type accessParams struct {
	reader Reader
	steps  []step
	offset int
	expect byte
	ok     bool
	state  Position
	error  error
}

// accessTestCases defines shared test cases for testing byteReader byte access
// methods.
var accessTestCases = map[string]accessParams{
	"buffer-hit": {
		reader: NewReaderState([]byte("abc"),
			false, 4, 8, NewState(1, 2, 3, 4), nil),
		offset: 0,
		expect: 'b',
		ok:     true,
		state:  Position{Byte: 2, Line: 3, Char: 4},
		error:  nil,
	},
	"extend-hit": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "x", err: io.EOF}},
		offset: 0,
		expect: 'x',
		ok:     true,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
	"eof": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{err: io.EOF}},
		offset: 0,
		expect: EOF,
		ok:     false,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
	"buffer-hit-offset": {
		reader: NewReaderState([]byte("abc"),
			false, 4, 8, NewState(0, 2, 1, 5), nil),
		offset: 2,
		expect: 'c',
		ok:     true,
		state:  Position{Byte: 2, Line: 1, Char: 5},
		error:  nil,
	},
	"buffer-hit-with-reader-offset": {
		reader: NewReaderState([]byte("zzab"),
			false, 4, 8, NewState(2, 4, 0, 4), nil),
		offset: 1,
		expect: 'b',
		ok:     true,
		state:  Position{Byte: 4, Line: 0, Char: 4},
		error:  nil,
	},
	"extend-hit-offset": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "ab"}, {data: "c", err: io.EOF}},
		offset: 2,
		expect: 'c',
		ok:     true,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
	"eof-short": {
		reader: NewReaderState(make([]byte, 0, 64),
			false, 4, 8, NewState(0, 0, 0, 0), nil),
		steps:  []step{{data: "ab", err: io.EOF}},
		offset: 3,
		expect: EOF,
		ok:     false,
		state:  Position{Byte: 0, Line: 0, Char: 0},
		error:  io.EOF,
	},
}

// offsetZero filters byte access cases to the subset that can be tested by
// peek, which has an implicit zero offset.
func offsetZero() test.FilterFunc[accessParams] {
	return func(_ string, param accessParams) bool {
		return param.offset == 0
	}
}

// TestPeek tests the peek method of byteReader for buffered, extend, and EOF
// scenarios.
func TestPeek(t *testing.T) {
	test.Map(t, accessTestCases).
		Filter(offsetZero()).
		Run(func(t test.Test, param accessParams) {
			// Given
			reader := param.reader
			reader.reader = &stepReader{steps: param.steps}

			// When
			actual, ok := reader.peek()

			// Then
			assert.Equal(t, param.expect, actual)
			assert.Equal(t, param.ok, ok)
			assert.Equal(t, param.state, reader.position())
			assert.Equal(t, param.error, reader.err)
		})
}

// TestGet tests the get method of byteReader for buffered, extend, and EOF
// scenarios.
func TestGet(t *testing.T) {
	test.Map(t, accessTestCases).
		Run(func(t test.Test, param accessParams) {
			// Given
			reader := param.reader
			reader.reader = &stepReader{steps: param.steps}

			// When
			actual, ok := reader.get(param.offset)

			// Then
			assert.Equal(t, param.expect, actual)
			assert.Equal(t, param.ok, ok)
			assert.Equal(t, param.state, reader.position())
			assert.Equal(t, param.error, reader.err)
		})
}
