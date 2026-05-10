package json

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tkrop/go-testing/test"
)

var (
	quotedx   = wrapQuoted((*Scanner).quotedx)
	quotedOx  = wrapQuoted_((*Scanner).quoted_x)
	relaxedx  = wrapRelaxed((*Scanner).relaxedx)
	relaxedOx = wrapRelaxed_((*Scanner).relaxed_x)
)

// scannerStringFailExtParams holds parameters for extension string fail paths.
type scannerStringFailExtParams struct {
	input   string
	quoted  func(*Scanner, byte) (byte, []byte, int)
	relaxed func(*Scanner) (byte, []byte, int)
}

// scannerStringFailExtTestCases defines extension-only string failure paths.
var scannerStringFailExtTestCases = map[string]scannerStringFailExtParams{
	// Invalid unicode in a quotedx (bulk-scan) string.
	"quotedx-unicode-fail": {
		input: `"\u!!!!"`, quoted: (*Scanner).quotedx,
	},
	// Invalid unicode in a quotedox string.
	"quotedox-unicode-fail": {
		input: `"\u!!!!"`, quoted: quotedOx,
	},
	// EOF in the probe-phase escape sub-loop of quotedx (no closing quote).
	"quotedx-probe-eof-escape": {
		input: "\"hello\\n\\", quoted: (*Scanner).quotedx,
	},
	// Invalid unicode in the probe-phase escape of quotedx.
	"quotedx-probe-unicode-fail": {
		input: "\"hello\\n\\u!!!!", quoted: (*Scanner).quotedx,
	},
	// EOF in the probe-phase escape sub-loop of quotedox.
	"quotedox-probe-eof-escape": {
		input: "\"hello\\n\\", quoted: quotedOx,
	},
	// Invalid unicode in the probe-phase escape of quotedox.
	"quotedox-probe-unicode-fail": {
		input: "\"hello\\n\\u!!!!", quoted: quotedOx,
	},
	// EOF in the escape sub-loop of a relaxedx string.
	"relaxedx-eof-escape": {
		input: `hello\`, relaxed: (*Scanner).relaxedx,
	},
	// EOF after first byte of UTF-8 whitespace in relaxedx bulk phase.
	"relaxedx-eof-uspace-bulk": {
		input: "hello\xC2", relaxed: (*Scanner).relaxedx,
	},
	// EOF after first byte of UTF-8 whitespace in relaxedx probe phase.
	"relaxedx-eof-uspace-probe": {
		input: "hello\\n\xC2", relaxed: (*Scanner).relaxedx,
	},
	// Invalid unicode in a relaxedx string.
	"relaxedx-unicode-fail": {
		input: `hello\u!!!!,`, relaxed: (*Scanner).relaxedx,
	},
	// EOF in the escape sub-loop of a relaxedox string.
	"relaxedox-eof-escape": {
		input: "hello\\", relaxed: relaxedOx,
	},
	// EOF after first byte of UTF-8 whitespace in relaxedox bulk phase.
	"relaxedox-eof-uspace-bulk": {
		input: "hello\xC2", relaxed: relaxedOx,
	},
	// EOF after first byte of UTF-8 whitespace in relaxedox probe phase.
	"relaxedox-eof-uspace-probe": {
		input: "hello\\n\xC2", relaxed: relaxedOx,
	},
	// Invalid unicode in a relaxedox string.
	"relaxedox-unicode-fail": {
		input: `hello\u!!!!,`, relaxed: relaxedOx,
	},
}

// TestScanStringFailExt exercises extension string scanner failure paths.
func TestScanStringFailExt(t *testing.T) {
	test.Map(t, scannerStringFailExtTestCases).
		Run(func(t test.Test, param scannerStringFailExtParams) {
			// Given
			scanner := NewScanner(
				strings.NewReader(param.input),
				make([]byte, 0, 64),
			)
			scanner.reader.extend()
			window := scanner.reader.window()
			typ, token, offset := byte(0), []byte(nil), 0

			// When
			if window[0] == Quote || window[0] == String {
				if param.quoted != nil {
					typ, token, offset = param.quoted(scanner, window[0])
				}
			} else if param.relaxed != nil {
				typ, token, offset = param.relaxed(scanner)
			}

			// Then
			assert.Equal(t, 0, offset)
			assert.Nil(t, token)
			assert.Equal(t, EOF, typ)
		})
}

// relaxedxEdgeParams holds parameters for relaxedx chunked-reader edge tests.
type relaxedxEdgeParams struct {
	input  string
	output string // expected decoded token; empty means failure (nil token)
}

// relaxedxEdgeTestCases defines test cases for relaxedx paths that only trigger
// when input arrives in small chunks (simulating a streaming reader).
var relaxedxEdgeTestCases = map[string]relaxedxEdgeParams{
	// Long plain string forces extend in the outer bulk loop.
	"long-plain": {
		input:  strings.Repeat("a", 70),
		output: strings.Repeat("a", 70),
	},
	// Unicode as first escape: with chunked delivery the backslash lands at a
	// chunk boundary, triggering the window extend in the bulk-phase escape
	// loop (covers window = s.reader.window() in bulk escape handler).
	"unicode-bulk": {
		input:  `hi\u0041bye`,
		output: "hiAbye",
	},
	// NBSP in plain content exercises bulk-phase uspace bytes path.
	"bulk-uspace-nbsp": {
		input:  "hi" + "\u00a0" + "there,",
		output: "hi" + "\u00a0" + "there",
	},
	// U+2028 in plain content exercises newline-aware uspace path.
	"bulk-uspace-line-separator": {
		input:  "hi" + "\u2028" + "there,",
		output: "hi" + "\u2028" + "there",
	},
	// Escape followed by plain bytes exercises the probe-then-bulk path.
	"probe-plain": {
		input:  `hello\nworld`,
		output: "hello\nworld",
	},
	// NBSP after an escape exercises probe-phase uspace bytes path.
	"probe-uspace-nbsp": {
		input:  "hello\\n" + "\u00a0" + "world,",
		output: "hello\n" + "\u00a0" + "world",
	},
	// U+2028 after an escape exercises probe-phase newline branch.
	"probe-uspace-line-separator": {
		input:  "hello\\n" + "\u2028" + "world,",
		output: "hello\n" + "\u2028" + "world",
	},
	// Escape followed by a delimiter exercises the probe-phase delim branch.
	"probe-delim": {
		input:  `hello\n,`,
		output: "hello\n",
	},
	// Escape followed by a literal newline exercises the probe-phase newline path.
	"probe-space-newline": {
		input:  "hello\\t\nworld",
		output: "hello\t\nworld",
	},
	// Escape followed by a space exercises the probe-phase space copy.
	"probe-space": {
		input:  "hello\\n world",
		output: "hello\n world",
	},
	// Two consecutive escapes exercise the probe-phase back-to-back escapes.
	"probe-escape": {
		input:  `hello\n\tworld`,
		output: "hello\n\tworld",
	},
	// Unknown escape in probe phase exercises the else branch (chunked).
	"probe-unknown-escape": {
		input:  `hello\n\qworld`,
		output: "hello\n\\qworld",
	},
	// Unicode in probe phase: with chunked delivery the second backslash lands
	// at a chunk boundary, triggering window extend in the probe-phase escape
	// loop (covers window = s.reader.window() in probe escape handler).
	"probe-unicode": {
		input:  `hi\n\u0041bye`,
		output: "hi\nAbye",
	},
	// EOF in the probe-phase escape sub-loop.
	"probe-eof-escape": {
		input:  `hello\n\`,
		output: "",
	},
	// Invalid unicode in the probe-phase escape.
	"probe-unicode-fail": {
		input:  `hello\n\u!!!!`,
		output: "",
	},
	// NBSP (U+00A0) before and after token should be treated as whitespace.
	"unicode-nbsp-delim": {
		input:  "\u00a0" + `hello` + "\u00a0" + `,`,
		output: "hello",
	},
}

// TestScanRelaxedxEdge exercises relaxedx paths that only trigger when input
// arrives in small chunks (simulating a streaming reader).
func TestScanRelaxedxEdge(t *testing.T) {
	test.Map(t, relaxedxEdgeTestCases).
		Run(func(t test.Test, param relaxedxEdgeParams) {
			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(param.input), 5),
				make([]byte, 0, 64))
			scanner.reader.skip()

			// When
			typ, token, offset := scanner.relaxedx()

			// Then
			if param.output == "" {
				assert.Equal(t, 0, offset)
				assert.Nil(t, token)
				assert.Equal(t, EOF, typ)
			} else {
				assert.Equal(t, param.output, string(token))
			}
		})
}

// TestScanRelaxedxEdge_ exercises relaxed_x paths that only trigger when input
// arrives in small chunks (simulating a streaming reader).
func TestScanRelaxedxEdge_(t *testing.T) {
	test.Map(t, relaxedxEdgeTestCases).
		Run(func(t test.Test, param relaxedxEdgeParams) {
			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(param.input), 5),
				make([]byte, 0, 64))
			scanner.reader.skip()

			// When
			typ, token, offset, _, _ := scanner.relaxed_x()

			// Then
			if param.output == "" {
				assert.Equal(t, 0, offset)
				assert.Nil(t, token)
				assert.Equal(t, EOF, typ)
			} else {
				assert.Equal(t, param.output, string(token))
			}
		})
}

// relaxedxDirectParams holds parameters for relaxedx paths that are exercised
// without chunked reads, ensuring the bytes>0 uspace branch is used.
type relaxedxDirectParams struct {
	input  string
	output string
	lines  int
	chars  int
}

// relaxedxDirectTestCases defines test cases for relaxedx paths that are
// exercised without chunked reads, ensuring the bytes>0 uspace branch is used.
var relaxedxDirectTestCases = map[string]relaxedxDirectParams{
	"bulk-uspace-nbsp": {
		input:  "ab" + "\u00a0" + "cd,",
		output: "ab" + "\u00a0" + "cd",
		chars:  5,
	},
	"bulk-trailing-uspace-nbsp": {
		input:  "a" + "\u00a0" + ",",
		output: "a",
		chars:  2,
	},
	"bulk-uspace-line-separator": {
		input:  "ab" + "\u2028" + "cd,",
		output: "ab" + "\u2028" + "cd",
		lines:  1,
		chars:  2,
	},
	"bulk-trailing-uspace-line-separator": {
		input:  "a" + "\u2028" + ",",
		output: "a",
		lines:  1,
		chars:  0,
	},
	// Invalid UTF-8 lead byte should fall through the top-level default branch
	// after indexRelaxedDelim flags it and uspace rejects it.
	"bulk-invalid-uspace-lead": {
		input:  "a\xC2b,",
		output: "a\xC2b",
		chars:  3,
	},
}

// TestScanRelaxedxDirect exercises complete UTF-8 whitespace handling in
// relaxedx without chunked reads, ensuring the bytes>0 uspace branch is used.
func TestScanRelaxedxDirect(t *testing.T) {
	test.Map(t, relaxedxDirectTestCases).
		Run(func(t test.Test, param relaxedxDirectParams) {
			// Given
			scanner := NewScanner(
				strings.NewReader(param.input),
				make([]byte, 0, 64),
			)
			scanner.reader.skip()

			// When
			_, token, offset := scanner.relaxedx()

			// Then
			assert.Equal(t, param.output, string(token))
			assert.Equal(t, len(param.input)-1, offset)
		})
}

// TestScanRelaxedxDirect_ exercises complete UTF-8 whitespace handling in
// relaxed_x without chunked reads, including line and char counters.
func TestScanRelaxedxDirect_(t *testing.T) {
	test.Map(t, relaxedxDirectTestCases).
		Run(func(t test.Test, param relaxedxDirectParams) {
			// Given
			scanner := NewScanner(
				strings.NewReader(param.input),
				make([]byte, 0, 64),
			)
			scanner.reader.skip()

			// When
			_, token, offset, lines, chars := scanner.relaxed_x()

			// Then
			assert.Equal(t, param.output, string(token))
			assert.Equal(t, len(param.input)-1, offset)
			assert.Equal(t, param.lines, lines)
			assert.Equal(t, param.chars, chars)
		})
}

// countParams holds parameters for count tests.
type countParams struct {
	input []byte
	lines int
	chars int
}

// countTestCases defines the test cases for count with various inputs,
// ensuring correct line and char counts.
var countTestCases = map[string]countParams{
	"empty": {
		input: []byte{},
	},
	"plain": {
		input: []byte("abc"),
		chars: 3,
	},
	"newline": {
		input: []byte("ab\ncd"),
		lines: 1,
		chars: 2,
	},
	"mixed-newline": {
		input: []byte("a\nb\ncd"),
		lines: 2,
		chars: 2,
	},
}

// TestScanCount exercises count with various inputs, ensuring correct line
// and char counts.
func TestScanCount(t *testing.T) {
	test.Map(t, countTestCases).
		Run(func(t test.Test, param countParams) {
			// Given

			// When
			lines, chars := count(param.input)

			// Then
			assert.Equal(t, param.lines, lines)
			assert.Equal(t, param.chars, chars)
		})
}

// count_TestCases defines test cases for count_ with extended UTF-8 whitespace
// characters and incomplete sequences, ensuring the bytes>0 uspace branch is
// used and that incomplete UTF-8 sequences are treated as non-whitespace.
var count_TestCases = map[string]countParams{
	"empty": {
		input: []byte{},
	},
	"newline": {
		input: []byte("ab\ncd"),
		lines: 1,
		chars: 2,
	},
	"line-separator": {
		input: []byte("ab\xE2\x80\xA8cd"),
		lines: 1,
		chars: 2,
	},
	"paragraph-separator": {
		input: []byte("ab\xE2\x80\xA9cd"),
		lines: 1,
		chars: 2,
	},
	"nbsp": {
		input: []byte("a\xC2\xA0b"),
		chars: 3,
	},
	"bom": {
		input: []byte("a\xEF\xBB\xBFb"),
		chars: 3,
	},
	"incomplete-line-separator": {
		input: []byte("a\xE2\x80b"),
		chars: 4,
	},
	"incomplete-nbsp": {
		input: []byte("a\xC2b"),
		chars: 3,
	},
	"incomplete-bom": {
		input: []byte("a\xEF\xBBb"),
		chars: 4,
	},
	"mixed-separators": {
		input: []byte("a\n\xE2\x80\xA8\xE2\x80\xA9z"),
		lines: 3,
		chars: 1,
	},
}

// TestScanCount_ exercises count_ with extended UTF-8 whitespace characters
// and incomplete sequences, ensuring the bytes>0 uspace branch is used and
// that incomplete UTF-8 sequences are treated as non-whitespace.
func TestScanCount_(t *testing.T) {
	test.Map(t, count_TestCases).
		Run(func(t test.Test, param countParams) {
			// Given

			// When
			lines, chars := count_(param.input)

			// Then
			assert.Equal(t, param.lines, lines)
			assert.Equal(t, param.chars, chars)
		})
}

// trimParams holds parameters for trim tests.
type trimParams struct {
	input  []byte
	end    int
	expect int
}

// trimTestCases defines test cases for trim with ASCII and UTF-8 whitespace
// suffixes, plus a mixed suffix that exercises all trimming branches.
var trimTestCases = map[string]trimParams{
	"ascii-space": {
		input:  []byte("a "),
		end:    2,
		expect: 1,
	},
	"nbsp": {
		input:  []byte("a\xC2\xA0"),
		end:    3,
		expect: 1,
	},
	"bom": {
		input:  []byte("a\xEF\xBB\xBF"),
		end:    4,
		expect: 1,
	},
	"line-separator": {
		input:  []byte("a\xE2\x80\xA8"),
		end:    4,
		expect: 1,
	},
	"paragraph-separator": {
		input:  []byte("a\xE2\x80\xA9"),
		end:    4,
		expect: 1,
	},
	"mixed-suffix": {
		input:  []byte("a \xC2\xA0\xEF\xBB\xBF\xE2\x80\xA8"),
		end:    10,
		expect: 1,
	},
	"no-trim": {
		input:  []byte("ab"),
		end:    2,
		expect: 2,
	},
}

// TestScanTrim exercises trim across all supported ASCII and UTF-8
// whitespace suffix branches.
func TestScanTrim(t *testing.T) {
	test.Map(t, trimTestCases).
		Run(func(t test.Test, param trimParams) {
			// Given

			// When
			end := trim(param.input, param.end)

			// Then
			assert.Equal(t, param.expect, end)
		})
}
