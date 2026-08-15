package json

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkrop/go-testing/test"
)

// token represents a scanned token with its type and value.
type token struct {
	typ   byte
	token string
}

// fileParams holds the parameters for a file test case, including the path to
// the file, the expected number of tokens, the expected number of raw bytes,
// and the expected number of space characters.
type fileParams struct {
	path   string
	chars  int
	tokens int
	space  int
}

// fileTestCase is a map of file test cases, where each key is the name of the
// test case and the value is a fileParams struct containing the parameters for
// the test case. The test cases include various JSON files with their expected
// token counts, raw byte counts, and space character counts.
var fileTestCase = map[string]fileParams{
	// from https://github.com/miloyip/nativejson-benchmark
	"canada": {
		path: "canada", tokens: 334373, chars: 2251003, space: 33,
	},
	"catalog": {
		path: "catalog", tokens: 135990, chars: 447089, space: 1226905,
	},
	"twitter": {
		path: "twitter", tokens: 55263, chars: 429480, space: 164608,
	},
	"code": {
		path: "code", tokens: 396293, chars: 1735570, space: 3,
	},

	// from https://raw.githubusercontent.com/mailru/easyjson/master/benchmark/example.json
	"example": {
		path: "example", tokens: 1297, chars: 8058, space: 4122,
	},

	// from https://github.com/ultrajson/ultrajson/blob/master/tests/sample.json
	"sample": {
		path: "sample", tokens: 8677, chars: 160587, space: 518187,
	},
}

// getFixtureReader returns a `Reader` for the contents of path.
func getFixtureReader(t test.Reporter, path string) *bytes.Reader {
	file := test.Must(os.Open(filepath.Join("fixture", path+".json.gz")))
	unzip := test.Must(gzip.NewReader(file))
	buffer := test.Must(io.ReadAll(unzip))
	require.NoError(t, unzip.Close())
	require.NoError(t, file.Close())

	return bytes.NewReader(buffer)
}

// TestReader is a test helper that reads the entire contents of a file and
// returns it as a byte slice.

// TestReader is an `io.Reader` that reads small chunks of data.
type TestReader struct {
	reader io.Reader
	modulo int
	num    int
}

// NewTestReader returns a new `testReader` that reads from reader in small chunks
// of data using modulo.
func NewTestReader(reader io.Reader, modulo int) io.Reader {
	return &TestReader{reader: reader, modulo: modulo, num: 0}
}

// next returns the next chunk size.
func (r *TestReader) next() int {
	r.num = r.num + 7
	return r.num%(r.modulo) + 1
}

// Read reads up to len(buf) bytes into buf.
func (r *TestReader) Read(buf []byte) (int, error) {
	return r.reader.Read(buf[:r.min(r.next(), len(buf))])
}

// min returns the minimum of a and b.
func (*TestReader) min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ScanTracker is a helper struct that tracks the expected context of the next
// token to be scanned by the Scanner. It maintains a stack of scan frames,
// where each frame represents a specific context (e.g., root, object, array)
// and the expected context for the next token.
//
// The ScanTracker provides methods to push and pop frames from the stack,
// advance the context based on the scanned token type, and retrieve the
// current expected context.

// ScanKind represents the kind of a scan frame, indicating whether it is
// a root, object, or array context.
type ScanKind byte

// scanFrameKind values are used to indicate the kind of a scan frame in the
// ScanTracker stack. The kind determines the valid token types and parsing
// rules for the next token.
const (
	// ScanRoot represents a root context, where the next token is expected to
	// be a value in root position.
	ScanRoot ScanKind = iota
	// ScanObject represents an object context, where the next token is
	// expected to be an object property key or value.
	ScanObject
	// ScanArray represents an array context, where the next token is expected
	// to be an array element value.
	ScanArray
)

// ScanFrame represents a single frame in the scan tracker stack, containing
// the kind of the frame and the expected context for the next token.
type ScanFrame struct {
	kind ScanKind
	ctx  ScanContext
}

// ScanTracker is a helper struct that tracks the expected context of the next
// token to be scanned by the Scanner. It maintains a stack of scan frames,
// where each frame represents a specific context (e.g., root, object, array)
// and the expected context for the next token.
type ScanTracker struct {
	stack []ScanFrame
}

// Ctx returns the current expected context of the next token to be scanned
// by the Scanner. If the stack is empty, it returns RootValue, indicating that
// the next token is expected to be a value in root position. Otherwise, it
// returns the context of the top frame on the stack.
func (t *ScanTracker) Ctx() ScanContext {
	if len(t.stack) == 0 {
		return RootValue
	}

	return t.stack[len(t.stack)-1].ctx
}

// Advance updates the expected context of the next token based on the scanned
// token type. It modifies the top frame on the stack or pushes/pops frames as
// necessary to reflect the new context after scanning the token. The typ
// parameter specifies the type of the scanned token.
func (t *ScanTracker) Advance(typ byte) {
	if len(t.stack) == 0 {
		switch typ {
		case ObjectStart:
			t.push(ScanObject, ObjectKey)
		case ArrayStart:
			t.push(ScanArray, ArrayValue)
		}

		return
	}

	top := &t.stack[len(t.stack)-1]
	switch typ {
	case ObjectStart:
		t.push(ScanObject, ObjectKey)
	case ArrayStart:
		t.push(ScanArray, ArrayValue)
	case ObjectEnd, ArrayEnd:
		t.pop()
	case Comma:
		switch top.kind {
		case ScanObject:
			top.ctx = ObjectKey
		case ScanArray:
			top.ctx = ArrayValue
		}
	case Colon:
		if top.kind == ScanObject {
			top.ctx = ObjectValue
		}
	default:
		if top.kind == ScanObject && top.ctx == ObjectKey {
			top.ctx = ObjectValue
		}
	}
}

// push adds a new scan frame to the top of the stack, representing a specific
// context (e.g., root, object, array) and the expected context for the next
// token. The kind parameter specifies the kind of the frame, and the ctx
// parameter specifies the expected context for the next token.
func (t *ScanTracker) push(kind ScanKind, ctx ScanContext) {
	t.stack = append(t.stack, ScanFrame{kind: kind, ctx: ctx})
}

// pop removes the top scan frame from the stack, effectively returning to the
// previous context. If the stack is empty, it does nothing.
func (t *ScanTracker) pop() {
	if len(t.stack) == 0 {
		return
	}

	t.stack = t.stack[:len(t.stack)-1]
}

// trackerParams holds the parameters for a scan tracker test case, including the
// sequence of scanned token types and the expected contexts after each token is
// processed.
type trackerParams struct {
	steps []byte
	ctxs  []ScanContext
}

// trackerTestCases is a map of test cases for the scanTracker struct, where each
// key is the name of the test case and the value is a trackerParams struct.
var trackerTestCases = map[string]trackerParams{
	"root-object-array": {
		steps: []byte{
			ObjectStart, String, Colon, ArrayStart, Integer, Comma,
			ObjectStart, String, Colon, True, ObjectEnd, ArrayEnd, ObjectEnd,
		},
		ctxs: []ScanContext{
			RootValue,
			ObjectKey,
			ObjectValue,
			ObjectValue,
			ArrayValue,
			ArrayValue,
			ArrayValue,
			ObjectKey,
			ObjectValue,
			ObjectValue,
			ObjectValue,
			ArrayValue,
			ObjectValue,
			RootValue,
		},
	},
	"array-only": {
		steps: []byte{ArrayStart, Integer, Comma, String, ArrayEnd},
		ctxs:  []ScanContext{RootValue, ArrayValue, ArrayValue, ArrayValue, ArrayValue, RootValue},
	},
	"empty-object": {
		steps: []byte{ObjectStart, ObjectEnd},
		ctxs:  []ScanContext{RootValue, ObjectKey, RootValue},
	},
}

// TestScanTracker tests the scanTracker struct by simulating the scanning of
// various JSON structures and verifying that the expected context of the next
// token is correctly tracked. It uses a set of predefined test cases that
// specify the sequence of scanned token types and the expected contexts after
// each token is processed.
func TestScanTracker(t *testing.T) {
	test.Map(t, trackerTestCases).
		Run(func(t test.Test, param trackerParams) {
			var tracker ScanTracker

			assert.Equal(t, param.ctxs[0], tracker.Ctx())
			for index, typ := range param.steps {
				tracker.Advance(typ)
				assert.Equal(t, param.ctxs[index+1], tracker.Ctx())
			}
		})
}

//
// Scanning unicode hex digits (for \u and \U sequences).
//

// unhexParams holds the parameters for a unihex test case, including the input
// byte slice, the expected rune result, and a boolean indicating whether the
// input is valid.
type unhexParams struct {
	input  []byte
	expect rune
	ok     bool
}

// unihexTestCases is a map of test cases for the unihex function.
var unihexTestCases = map[string]unhexParams{
	"lower-digits":   {input: []byte("a1b2"), expect: 0xa1b2, ok: true},
	"upper-digits":   {input: []byte("A1B2"), expect: 0xa1b2, ok: true},
	"decimal-digits": {input: []byte("0123"), expect: 0x0123, ok: true},
	"min":            {input: []byte("0000"), expect: 0x0000, ok: true},
	"max":            {input: []byte("ffff"), expect: 0xffff, ok: true},
	"mixed-case":     {input: []byte("aAbB"), expect: 0xaabb, ok: true},
	"extra-bytes":    {input: []byte("0041XY"), expect: 0x0041, ok: true},
	"bmp-heart":      {input: []byte("2764"), expect: 0x2764, ok: true},
	"surrogate-high": {input: []byte("D800"), expect: 0xD800, ok: true},
	"surrogate-low":  {input: []byte("DFFF"), expect: 0xDFFF, ok: true},
	// Invalid character in each position.
	"invalid-pos-0": {input: []byte("g123"), ok: false},
	"invalid-pos-1": {input: []byte("0g23"), ok: false},
	"invalid-pos-2": {input: []byte("00g3"), ok: false},
	"invalid-pos-3": {input: []byte("000g"), ok: false},
	// Non-hex ASCII characters.
	"invalid-space":  {input: []byte(" 000"), ok: false},
	"invalid-colon":  {input: []byte("00:0"), ok: false},
	"invalid-at":     {input: []byte("004@"), ok: false},
	"invalid-grave":  {input: []byte("00`0"), ok: false},
	"invalid-lbrace": {input: []byte("00{0"), ok: false},
}

// testScanUnihex tests the unihex function with various input cases, including
// valid hexadecimal digit sequences and invalid sequences, ensuring that the
// function correctly decodes valid sequences and returns false for invalid
// ones.
func testScanUnihex(
	t *testing.T, name string,
	call func([]byte) (rune, bool),
) {
	test.Map(t, unihexTestCases).
		Prefix("method=" + name + "/test=").
		Run(func(t test.Test, param unhexParams) {
			// When
			r, ok := call(param.input)

			// Then
			assert.Equal(t, param.ok, ok)
			if param.ok {
				assert.Equal(t, param.expect, r)
			}
		})
}

// TestScanUnihex tests the unihex function with various input cases.
func TestScanUnihex(t *testing.T) {
	testScanUnihex(t, "unihex", unihex)
}

// benchmarkScanUnihex benchmarks the unihex function with various input cases,
// measuring the performance of decoding hexadecimal digit sequences.
func benchmarkScanUnihex(
	b *testing.B, name string, call func([]byte) (rune, bool),
) {
	test.Map(test.Benchmark(b), unihexTestCases).
		Prefix("method=" + name + "/test=").
		Benchmark(func(b *testing.B, param unhexParams) func(*testing.B) {
			// Setup
			b.SetBytes(4)

			// Loop
			return func(*testing.B) {
				r, ok := call(param.input)

				runtime.KeepAlive(r)
				runtime.KeepAlive(ok)
			}
		})
}

// BenchmarkScanUnihex benchmarks the unihex function with various input cases.
func BenchmarkScanUnihex(b *testing.B) {
	benchmarkScanUnihex(b, "unihex", unihex)
}

// uspaceParams holds parameters for a uspace test case.
type uspaceParams struct {
	window []byte
	offset int
	size   int
	line   bool
	extend bool
}

// uspaceTestCases is a map of test cases for the uspace function.
var uspaceTestCases = map[string]uspaceParams{
	"no-space": {
		window: []byte("x"), offset: 0,
		size: 0, line: false, extend: false,
	},
	"offset-c2-a0": {
		window: []byte{'x', 0xC2, 0xA0}, offset: 1,
		size: 2, line: false, extend: false,
	},
	"c2-a0": {
		window: []byte{0xC2, 0xA0}, offset: 0,
		size: 2, line: false, extend: false,
	},
	"c2-partial": {
		window: []byte{0xC2}, offset: 0,
		size: 0, line: false, extend: true,
	},
	"c2-other": {
		window: []byte{0xC2, 0xA1}, offset: 0,
		size: 0, line: false, extend: false,
	},
	"ef-bb-bf": {
		window: []byte{0xEF, 0xBB, 0xBF}, offset: 0,
		size: 3, line: false, extend: false,
	},
	"ef-partial-1": {
		window: []byte{0xEF}, offset: 0,
		size: 0, line: false, extend: true,
	},
	"ef-partial-2": {
		window: []byte{0xEF, 0xBB}, offset: 0,
		size: 0, line: false, extend: true,
	},
	"ef-other": {
		window: []byte{0xEF, 0xBA, 0xBF}, offset: 0,
		size: 0, line: false, extend: false,
	},
	"e2-80-a8": {
		window: []byte{0xE2, 0x80, 0xA8}, offset: 0,
		size: 3, line: true, extend: false,
	},
	"e2-80-a9": {
		window: []byte{0xE2, 0x80, 0xA9}, offset: 0,
		size: 3, line: true, extend: false,
	},
	"e2-partial-1": {
		window: []byte{0xE2}, offset: 0,
		size: 0, line: false, extend: true,
	},
	"e2-partial-2": {
		window: []byte{0xE2, 0x80}, offset: 0,
		size: 0, line: false, extend: true,
	},
	"e2-other": {
		window: []byte{0xE2, 0x80, 0xAA}, offset: 0,
		size: 0, line: false, extend: false,
	},
}

// TestUSpace tests UTF-8 whitespace detection with uspace.
func TestUSpace(t *testing.T) {
	test.Map(t, uspaceTestCases).
		Run(func(t test.Test, param uspaceParams) {
			// When
			size, line, extend := uspace(param.window, param.offset)

			// Then
			assert.Equal(t, param.size, size)
			assert.Equal(t, param.line, line)
			assert.Equal(t, param.extend, extend)
		})
}
