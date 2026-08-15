package json

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkrop/go-testing/test"
)

// Mode extensions for testing purposes.
const (
	// Mock mode enables testing using the mock scanner.
	Mock Mode = 0x0004
)

// Token represents a scanned Token with its type and value.
type Token struct {
	ctx   ScanContext
	typ   byte
	token string
}

// Wrapper wraps a `Scanner` and adds test assertions for token sequences.
type Wrapper struct {
	Scanner
	t      test.Test
	tokens []Token
	pos    []Position
	tindex int
	pindex int
}

// NewWrapper creates a new Wrapper instance that wraps the given scanner and
// adds test assertions for token sequences and positions.
func NewWrapper(
	scanner Scanner, t test.Test,
	tokens []Token, pos []Position,
) *Wrapper {
	return &Wrapper{
		Scanner: scanner,
		t:       t,
		tokens:  tokens,
		pos:     pos,
	}
}

// Next wraps the `Next` method of the scanner and asserts the expected token
// sequence.
func (s *Wrapper) Next(ctx ScanContext) (byte, []byte) {
	typ, token := s.Scanner.Next(ctx)
	assert.Equal(s.t, s.tokens[s.tindex], Token{
		ctx: ctx, typ: typ, token: string(token),
	}, "token mismatch at index %d", s.tindex)
	s.tindex++

	return typ, token
}

// Position wraps the `Position` method of the `Scanner` and asserts the
// expected position.
func (s *Wrapper) Position() Position {
	pos := s.Scanner.Position()
	if s.pindex < len(s.pos) {
		assert.Equal(s.t, s.pos[s.pindex], pos,
			"position mismatch at index %d", s.pindex)
	} else if len(s.pos) > 0 {
		assert.Fail(s.t, "unexpected position call at index %d", s.pindex)
	}
	s.pindex++

	return pos
}

func (s *Wrapper) Validate() {
	assert.Equal(s.t, len(s.tokens), s.tindex, "not all tokens were consumed")
	assert.Equal(s.t, len(s.pos), s.pindex, "not all positions were consumed")
}

// copyShallow creates a shallow copy of the given value if it is a non-nil
// pointer. For maps, it initializes a new empty map. Otherwise, it returns
// the original value.
func copyShallow(value any) any {
	if value == nil {
		return nil
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return value
	}

	copy := reflect.New(val.Type().Elem())
	if copy.Elem().Kind() == reflect.Map {
		copy.Elem().Set(reflect.MakeMap(copy.Elem().Type()))
	}

	return copy.Interface()
}

// fileParams holds scanner token, delimiter, byte, and scanner whitespace
// counts for a fixture file.
type fileParams struct {
	name   string
	chars  int
	tokens int
	delims int
	spaces int
}

// fileTestCase is a map of file test cases, where each key is the name of the
// test case and the value is a fileParams struct containing the parameters for
// the test case. The test cases include various JSON files with their expected
// token counts, raw byte counts, and space character counts.
var fileTestCase = map[string]fileParams{
	// from https://github.com/miloyip/nativejson-benchmark
	"canada": {
		name:   "canada",
		tokens: 334373, delims: 111137, chars: 2251003, spaces: 33,
	},
	"catalog": {
		name:   "catalog",
		tokens: 135990, delims: 50955, chars: 447089, spaces: 1226905,
	},
	"twitter": {
		name:   "twitter",
		tokens: 55263, delims: 25690, chars: 429480, spaces: 164608,
	},
	"code": {
		name:   "code",
		tokens: 396293, delims: 178586, chars: 1735570,
	},

	// from https://raw.githubusercontent.com/mailru/easyjson/master/benchmark/example.json
	"example": {
		name:   "example",
		tokens: 1297, delims: 587, chars: 8058, spaces: 4122,
	},

	// from https://github.com/ultrajson/ultrajson/blob/master/tests/sample.json
	"sample": {
		name:   "sample",
		tokens: 8677, delims: 3401, chars: 160587, spaces: 518187,
	},
}

// getFixtureReader returns a `Reader` for the contents of path.
func getFixtureReader(t test.Reporter, name string) *bytes.Reader {
	file := test.Must(os.Open(filepath.Join("fixture", name+".json.gz")))
	unzip := test.Must(gzip.NewReader(file))
	buffer := test.Must(io.ReadAll(unzip))
	require.NoError(t, unzip.Close())
	require.NoError(t, file.Close())

	return bytes.NewReader(buffer)
}

// countFile counts the number of delimiters, characters, and spaces in a JSON file
// using the provided scanner. It returns the counts for delimiters, characters,
// and spaces. The function stops counting when it reaches the end of the file (EOF).
func countFile(scanner *scanner) (delims, chars, spaces int) {
	for {
		typ, token := scanner.Next_(RootValue)
		switch typ {
		case EOF:
			return delims, chars, spaces
		case Space:
			spaces += utf8.RuneCount(token)
		default:
			chars += len(token)
			if typ == Comma || typ == Colon {
				delims++
			}
		}
	}
}

// TestFiles tests the counting of delimiters, characters, and spaces in JSON files
// using the countFile function and the ScanTracker.
func TestFiles(t *testing.T) {
	test.Map(t, fileTestCase).
		Run(func(t test.Test, param fileParams) {
			// Given
			scanner := test.Cast[*scanner](NewScannerSource(
				getFixtureReader(t, param.name), make([]byte, 0, 8<<10)))
			scanner.SetMode(Json)

			// When
			delims, chars, spaces := countFile(scanner)

			// Then
			assert.Equal(t, param.delims, delims, "delims")
			assert.Equal(t, param.chars, chars, "chars")
			assert.Equal(t, param.spaces, spaces, "spaces")
			assert.Equal(t, io.EOF, scanner.Error(), "error")
		})
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

	return r.num%r.modulo + 1
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

type normcomplexParams struct {
	input  string
	output string
	expect complex128
	error  error
}

var normcomplexTestCases = map[string]normcomplexParams{
	"real-first": {
		input: "1+2i", output: "1+2i", expect: 1 + 2i,
	},
	"real-first-imaginary-unit": {
		input: "1+i", output: "1+1i", expect: 1 + 1i,
	},
	"real-first-negative-unit": {
		input: "-1-i", output: "-1-1i", expect: -1 - 1i,
	},
	"imaginary-unit": {
		input: "i", output: "1i", expect: 1i,
	},
	"negative-imaginary-unit": {
		input: "-i", output: "-1i", expect: -1i,
	},
	"imaginary-first": {
		input: "2i+1", output: "1+2i", expect: 1 + 2i,
	},
	"imaginary-first-unit": {
		input: "i+1", output: "1+1i", expect: 1 + 1i,
	},
	"imaginary-first-negative-real": {
		input: "2i-1", output: "-1+2i", expect: -1 + 2i,
	},
	"imaginary-first-negative-imaginary": {
		input: "-2i+1", output: "1-2i", expect: 1 - 2i,
	},
	"imaginary-first-negative-unit": {
		input: "-i+1", output: "1-1i", expect: 1 - 1i,
	},
	"imaginary-first-both-negative": {
		input: "-2i-1", output: "-1-2i", expect: -1 - 2i,
	},
	"imaginary-first-exponents": {
		input: "1.2e-3i+4e+2", output: "4e+2+1.2e-3i",
		expect: 400 + 0.0012i,
	},
	"real-first-exponents": {
		input: "1e-2+3e+4i", output: "1e-2+3e+4i",
		expect: 0.01 + 30000i,
	},
	"invalid-leading-i": {
		input: "invalid", output: "nvalid+1i",
		error: &strconv.NumError{
			Func: "ParseComplex", Num: "nvalid+1i", Err: strconv.ErrSyntax,
		},
	},
	"invalid-imaginary-suffix": {
		input: "1+2ix", output: "x+1+2i",
		error: &strconv.NumError{
			Func: "ParseComplex", Num: "x+1+2i", Err: strconv.ErrSyntax,
		},
	},
	"invalid-extra-component": {
		input: "1+2i+3", output: "3+1+2i",
		error: &strconv.NumError{
			Func: "ParseComplex", Num: "3+1+2i", Err: strconv.ErrSyntax,
		},
	},
	"invalid-missing-real": {
		input: "1i+", output: "1i+",
		error: &strconv.NumError{
			Func: "ParseComplex", Num: "1i+", Err: strconv.ErrSyntax,
		},
	},
}

func TestNormcomplex(t *testing.T) {
	test.Map(t, normcomplexTestCases).
		Run(func(t test.Test, param normcomplexParams) {
			// When
			output := normcomplex(param.input)
			expect, err := strconv.ParseComplex(output, 128)

			// Then
			assert.Equal(t, param.output, output)
			assert.Equal(t, param.expect, expect)
			assert.Equal(t, param.error, err)
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
