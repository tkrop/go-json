//nolint:gocognit,cyclop,nestif // validating test precondition.
//revive:disable:function-length -- priority speed.
//revive:disable:cognitive-complexity -- priority speed.
//revive:disable:cyclomatic -- priority speed.
package json

import (
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tkrop/go-testing/test"
)

// JsonType represents the type of JSON parsing.
type JsonType byte

// Enumeration of JSON parsing types.
const (
	// Type for JSON standard parsing.
	Json JsonType = iota
	// Type for JSON5 standard parsing.
	Json5
	// Type for JSON5 relaxed parsing.
	Json5Relax
	// Type for JSON5 extended parsing (complex numbers, etc).
	Json5Ext
)

// TestType identifies a test used to execute scan test cases.
type TestType byte

// Test types used by scan tests.
const (
	// Next is the test type for basic next scanning tests.
	Next TestType = iota
	// Number is the test type for numeric scanning tests.
	Number
	// Quoted is the test type for quoted string scanning tests.
	Quoted
)

// types filters test cases that should not run for a given test type.
func types(runner TestType) test.FilterFunc[scanParams] {
	return func(_ string, p scanParams) bool {
		return !slices.Contains(p.skip, runner)
	}
}

// Unicode whitespace characters as defined in JSON5 specification.
const (
	// Sequence of whitespace characters in JSON.
	uspaces = "\t\n\v\f\r\u00a0\u2028\u2029\ufeff "
	// Reverse sequence of whitespace characters in JSON.
	uspaces_ = " \ufeff\u2029\u2028\u00a0\r\f\n\v\t"
	// Sequence of escaped control chars plus backslash and unicode heart.
	uescapes = "\n\b\f\v\t\r\\\u2764"
	// Sequence of raw control chars plus backslash and unicode heart.
	uescapes_ = `\n\b\f\v\t\r\\\u2764`
)

// filterMaxMode returns a filter that keeps only test cases whose json mode
// is at most max, i.e. it excludes cases that require a higher-level mode.
func filterMaxMode(max JsonType) test.FilterFunc[scanParams] {
	return func(_ string, param scanParams) bool {
		return param.json <= max
	}
}

// assertPosition asserts expected vs actual stream position based on variant.
// For next-debug (excluding quotedx), it compares bytes/line/char position.
// For other variants, it only compares the byte position.
func assertPosition(
	t assert.TestingT, name string, expected, actual Position, msg ...any,
) {
	if strings.HasPrefix(name, "next-debug") &&
		!strings.Contains(name, "quotedx") {
		assert.Equal(t, expected, actual, msg...)
	} else {
		assert.Equal(t, expected.Byte, actual.Byte, msg...)
	}
}

// scanParams represents the parameters for scanning tests.
type scanParams struct {
	input  string
	json   JsonType
	expect []token
	pos    []Position
	// TODO: unused and may be removed.
	skip []TestType
}

// signedHexPrefixRegex is a regular expression to match signed hexadecimal
// prefixes.
var signedHexPrefixRegex = regexp.MustCompile("^[+-]?0x")

// scanTestCases contains test cases for scanning JSON tokens.
var scanTestCases = map[string]scanParams{
	"token-empty": {
		pos:  []Position{{Byte: 0, Line: 0, Char: 0}},
		skip: []TestType{Next, Number, Quoted},
	},

	// Invalid comments.
	"comment-slash": {
		input: "/", json: Json5,
		expect: []token{
			{typ: String, token: "/"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 1, Line: 0, Char: 1},
		},
	},
	"comment-slash-x": {
		input: "/x", json: Json5,
		expect: []token{
			{typ: String, token: "/x"},
		},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},

	// Single-line comments.
	"comment-single": {
		input: "// hello world", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello world"},
		},
		pos: []Position{
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 14, Line: 0, Char: 14},
		},
	},
	"comment-single-newline": {
		input: "// hello\nworld", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello"},
			{typ: String, token: "world"},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 14, Line: 1, Char: 5},
			{Byte: 14, Line: 1, Char: 5},
		},
	},
	"comment-single-return": {
		input: "// hello\rworld", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello"},
			{typ: String, token: "world"},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 14, Line: 0, Char: 14},
		},
	},
	"comment-single-nbsp": {
		input: "// hello\u00a0world\nrest", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello\u00a0world"},
			{typ: String, token: "rest"},
		},
		pos: []Position{
			{Byte: 15, Line: 0, Char: 14},
			{Byte: 20, Line: 1, Char: 4},
			{Byte: 20, Line: 1, Char: 4},
		},
	},
	"comment-single-u2028": {
		input: "// hello\u2028world", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello"},
			{typ: String, token: "world"},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 16, Line: 1, Char: 5},
			{Byte: 16, Line: 1, Char: 5},
		},
	},
	"comment-single-u2029": {
		input: "// hello\u2029world", json: Json5,
		expect: []token{
			{typ: Comment, token: " hello"},
			{typ: String, token: "world"},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 16, Line: 1, Char: 5},
			{Byte: 16, Line: 1, Char: 5},
		},
	},
	"comment-single-x": {
		input: "//x", json: Json5,
		expect: []token{
			{typ: Comment, token: "x"},
		},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},

	// Multi-line comments.
	"comment-multi": {
		input: "/*x*/", json: Json5,
		expect: []token{{typ: CommentMulti, token: "x"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"comment-multi-null": {
		input: "/* hello */null", json: Json5,
		expect: []token{
			{typ: CommentMulti, token: " hello "},
			{typ: Null, token: "null"},
		},
		pos: []Position{
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},
	"comment-multi-newline": {
		input: "/* hello\nworld */", json: Json5,
		expect: []token{{typ: CommentMulti, token: " hello\nworld "}},
		pos: []Position{
			{Byte: 17, Line: 1, Char: 8},
			{Byte: 17, Line: 1, Char: 8},
		},
	},
	"comment-multi-nbsp": {
		input: "/* hello\u00a0world */", json: Json5,
		expect: []token{{typ: CommentMulti, token: " hello\u00a0world "}},
		pos: []Position{
			{Byte: 18, Line: 0, Char: 17},
			{Byte: 18, Line: 0, Char: 17},
		},
	},
	"comment-multi-u2028": {
		input: "/* hello\u2028world */", json: Json5,
		expect: []token{{typ: CommentMulti, token: " hello\u2028world "}},
		pos: []Position{
			{Byte: 19, Line: 1, Char: 8},
			{Byte: 19, Line: 1, Char: 8},
		},
	},
	"comment-multi-u2029": {
		input: "/* hello\u2029world */", json: Json5,
		expect: []token{{typ: CommentMulti, token: " hello\u2029world "}},
		pos: []Position{
			{Byte: 19, Line: 1, Char: 8},
			{Byte: 19, Line: 1, Char: 8},
		},
	},
	// TODO: not sure that muli-line with EOF should be accepted.
	"comment-multi-error": {
		input: "/*error", json: Json5,
		expect: []token{{typ: CommentMulti, token: "error"}},
		pos: []Position{
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 7, Line: 0, Char: 7},
		},
	},

	// Null tokens.
	"token-null": {
		input: "null", json: Json,
		expect: []token{{typ: Null, token: "null"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"token-null-part": {
		input: "nul", json: Json,
		expect: []token{{typ: String, token: "nul"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"token-null-mismatch": {
		input: "nulx", json: Json,
		expect: []token{{typ: String, token: "nulx"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"token-null-extend": {
		input: "nullx", json: Json,
		expect: []token{{typ: String, token: "nullx"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-null-space": {
		input: uspaces + "null" + uspaces_, json: Json,
		expect: []token{{typ: Null, token: "null"}},
		pos: []Position{
			{Byte: 21, Line: 3, Char: 6},
			{Byte: 38, Line: 6, Char: 2},
		},
	},
	"token-null-double": {
		input: "null" + uspaces_ + "null", json: Json,
		expect: []token{
			{typ: String, token: "null" + uspaces_ + "null"},
		},
		pos: []Position{
			{Byte: 25, Line: 3, Char: 6},
			{Byte: 25, Line: 3, Char: 6},
		},
	},
	"token-null-comma": {
		input: "null,", json: Json,
		expect: []token{
			{typ: Null, token: "null"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-null-space-comma": {
		input: "null" + uspaces_ + ",", json: Json,
		expect: []token{
			{typ: Null, token: "null"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 22, Line: 3, Char: 3},
			{Byte: 22, Line: 3, Char: 3},
		},
	},
	"token-null-comment": {
		input: "null" + uspaces_ + "// comment\n" + ",", json: Json5,
		expect: []token{
			{typ: Null, token: "null"},
			{typ: Comment, token: " comment"},
			{typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 31, Line: 3, Char: 12},
			{Byte: 33, Line: 4, Char: 1},
			{Byte: 33, Line: 4, Char: 1},
		},
	},
	"token-null-comment-multi": {
		input: "null" + "/* comment */" + ",", json: Json5,
		expect: []token{
			{typ: Null, token: "null"},
			{typ: CommentMulti, token: " comment "},
			{typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 18, Line: 0, Char: 18},
			{Byte: 18, Line: 0, Char: 18},
		},
	},
	"token-null-comment-invalid": {
		input: "null /* comment */ false", json: Json5,
		expect: []token{
			{typ: String, token: "null /* comment */ false"},
		},
		pos: []Position{
			{Byte: 24, Line: 0, Char: 24},
			{Byte: 24, Line: 0, Char: 24},
		},
	},

	// Boolean tokens.
	"token-bool-true": {
		input: "true", json: Json,
		expect: []token{{typ: True, token: "true"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"token-true-mismatch": {
		input: "trux", json: Json,
		expect: []token{{typ: String, token: "trux"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"token-bool-true-extend": {
		input: "truex", json: Json,
		expect: []token{{typ: String, token: "truex"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-bool-false": {
		input: "false", json: Json,
		expect: []token{{typ: False, token: "false"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-false-mismatch": {
		input: "falsx", json: Json,
		expect: []token{{typ: String, token: "falsx"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-bool-false-extend": {
		input: "falsex", json: Json,
		expect: []token{{typ: String, token: "falsex"}},
		pos: []Position{
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 6, Line: 0, Char: 6},
		},
	},
	"token-bool-space": {
		input: uspaces + "true" + uspaces_, json: Json,
		expect: []token{{typ: True, token: "true"}},
		pos: []Position{
			{Byte: 21, Line: 3, Char: 6},
			{Byte: 38, Line: 6, Char: 2},
		},
	},
	"token-bool-double": {
		input: "true" + uspaces_ + "false", json: Json,
		expect: []token{
			{typ: String, token: "true" + uspaces_ + "false"},
		},
		pos: []Position{
			{Byte: 26, Line: 3, Char: 7},
			{Byte: 26, Line: 3, Char: 7},
		},
	},
	"token-bool-comma": {
		input: "true,", json: Json,
		expect: []token{
			{typ: True, token: "true"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"token-bool-space-comma": {
		input: "false" + uspaces_ + ",", json: Json,
		expect: []token{
			{typ: False, token: "false"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 23, Line: 3, Char: 3},
			{Byte: 23, Line: 3, Char: 3},
		},
	},
	"token-bool-comment": {
		input: "false" + uspaces_ + "// comment\n" + ",", json: Json5,
		expect: []token{
			{typ: False, token: "false"},
			{typ: Comment, token: " comment"},
			{typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 32, Line: 3, Char: 12},
			{Byte: 34, Line: 4, Char: 1},
			{Byte: 34, Line: 4, Char: 1},
		},
	},
	"token-bool-comment-multi": {
		input: "true" + "/* comment */" + ",", json: Json5,
		expect: []token{
			{typ: True, token: "true"},
			{typ: CommentMulti, token: " comment "},
			{typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 18, Line: 0, Char: 18},
			{Byte: 18, Line: 0, Char: 18},
		},
	},
	"token-bool-comment-invalid": {
		input: "true /* comment */ false", json: Json5,
		expect: []token{
			{typ: String, token: "true /* comment */ false"},
		},
		pos: []Position{
			{Byte: 24, Line: 0, Char: 24},
			{Byte: 24, Line: 0, Char: 24},
		},
	},

	// Special numbers.
	"number-nan": {
		input: "NaN", json: Json5,
		expect: []token{{typ: NaN, token: "NaN"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-nan-part": {
		input: "Na", json: Json5,
		expect: []token{{typ: String, token: "Na"}},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"number-nan-error": {
		input: "NaNx", json: Json5,
		expect: []token{{typ: String, token: "NaNx"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-nan-space": {
		input: uspaces + "NaN" + uspaces_, json: Json5,
		expect: []token{{typ: NaN, token: "NaN"}},
		pos: []Position{
			{Byte: 20, Line: 3, Char: 5},
			{Byte: 37, Line: 6, Char: 2},
		},
	},
	"number-nan-comma": {
		input: "NaN,", json: Json5,
		expect: []token{
			{typ: NaN, token: "NaN"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-nan-pos": {
		input: "+NaN", json: Json5,
		expect: []token{{typ: NaN, token: "+NaN"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-nan-neg": {
		input: "-NaN", json: Json5,
		expect: []token{{typ: NaN, token: "-NaN"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-infinity": {
		input: "Infinity", json: Json5,
		expect: []token{{typ: Infinity, token: "Infinity"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"number-infinity-space": {
		input: uspaces + "Infinity" + uspaces_, json: Json5,
		expect: []token{{typ: Infinity, token: "Infinity"}},
		pos: []Position{
			{Byte: 25, Line: 3, Char: 10},
			{Byte: 42, Line: 6, Char: 2},
		},
	},
	"number-infinity-comma": {
		input: "Infinity,", json: Json5,
		expect: []token{
			{typ: Infinity, token: "Infinity"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},
	"number-infinity-neg": {
		input: "-Infinity", json: Json5,
		expect: []token{{typ: Infinity, token: "-Infinity"}},
		pos: []Position{
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},
	"number-infinity-pos": {
		input: "+Infinity", json: Json5,
		expect: []token{{typ: Infinity, token: "+Infinity"}},
		pos: []Position{
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},

	// Zero numbers.
	"number-zero": {
		input: "0", json: Json,
		expect: []token{{typ: Integer, token: "0"}},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 1, Line: 0, Char: 1},
		},
	},
	"number-zero-space": {
		input: uspaces + "0" + uspaces_, json: Json,
		expect: []token{{typ: Integer, token: "0"}},
		pos: []Position{
			{Byte: 18, Line: 3, Char: 3},
			{Byte: 35, Line: 6, Char: 2},
		},
	},
	"number-zero-comma": {
		input: "0,", json: Json5,
		expect: []token{
			{typ: Integer, token: "0"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"number-zero-twice": {
		input: "0 0", json: Json5,
		expect: []token{{typ: String, token: "0 0"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-zero-neg": {
		input: "-0", json: Json5,
		expect: []token{{typ: Integer, token: "-0"}},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"number-zero-pos": {
		input: "+0", json: Json5,
		expect: []token{{typ: Integer, token: "+0"}},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"number-zero-int": {
		input: "0000", json: Json5,
		expect: []token{{typ: String, token: "0000"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-zero-float": {
		input: "0000.0000", json: Json5,
		expect: []token{{typ: String, token: "0000.0000"}},
		pos: []Position{
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},
	"number-zero-hex": {
		input: "0000x1010", json: Json5,
		expect: []token{{typ: String, token: "0000x1010"}},
		pos: []Position{
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},
	"number-zero-complex": {
		input: "0000+0000i", json: Json5,
		expect: []token{{typ: String, token: "0000+0000i"}},
		pos: []Position{
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 10, Line: 0, Char: 10},
		},
	},

	// Integer numbers.
	"number-int": {
		input: "12345678", json: Json,
		expect: []token{{typ: Integer, token: "12345678"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"number-int-space": {
		input: uspaces + "12345678" + uspaces_, json: Json,
		expect: []token{{typ: Integer, token: "12345678"}},
		pos: []Position{
			{Byte: 25, Line: 3, Char: 10},
			{Byte: 42, Line: 6, Char: 2},
		},
	},
	"number-int-comma": {
		input: "12345678,", json: Json,
		expect: []token{
			{typ: Integer, token: "12345678"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 9, Line: 0, Char: 9},
		},
	},
	"number-int-neg": {
		input: "-2197191237916642", json: Json,
		expect: []token{{typ: Integer, token: "-2197191237916642"}},
		pos: []Position{
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"number-int-pos": {
		input: "+2198137762017356", json: Json5,
		expect: []token{{typ: Integer, token: "+2198137762017356"}},
		pos: []Position{
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"number-int-extend": {
		input: "1\xC2\xA0,", json: Json,
		expect: []token{
			{typ: Integer, token: "1"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 4, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 3},
		},
	},
	"number-int-uspace-eof": {
		input: "1\xC2", json: Json,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"number-int-uspace-fallback": {
		input: "1\xC2", json: Json,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},

	// Dot non-decimal numbers.
	"number-dot-alpha": {
		input: ".abc", json: Json5,
		expect: []token{{typ: String, token: ".abc"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-dot-space": {
		input: uspaces + "." + uspaces_, json: Json5Ext,
		expect: []token{{typ: String, token: "."}},
		pos: []Position{
			{Byte: 35, Line: 6, Char: 2},
			{Byte: 35, Line: 6, Char: 2},
		},
	},
	"number-dot-comma": {
		input: ".,", json: Json5Ext,
		expect: []token{
			{typ: String, token: "."}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},

	// Decimal numbers.
	"number-float": {
		input: "12345678901.23456789012", json: Json,
		expect: []token{{typ: Decimal, token: "12345678901.23456789012"}},
		pos: []Position{
			{Byte: 23, Line: 0, Char: 23},
			{Byte: 23, Line: 0, Char: 23},
		},
	},
	"number-float-zero": {
		input: "0.123456789012", json: Json,
		expect: []token{{typ: Decimal, token: "0.123456789012"}},
		pos: []Position{
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 14, Line: 0, Char: 14},
		},
	},
	"number-float-zero-exp": {
		input: "0e5", json: Json5,
		expect: []token{{typ: Decimal, token: "0e5"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-float-dot-stop": {
		input: "12345678901234.", json: Json,
		expect: []token{{typ: Decimal, token: "12345678901234."}},
		pos: []Position{
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},
	"number-float-dot-start": {
		input: ".123456789012e-12", json: Json,
		expect: []token{{typ: Decimal, token: ".123456789012e-12"}},
		pos: []Position{
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"number-float-space": {
		input: uspaces + "12345678901.23456789012e-12" + uspaces_, json: Json,
		expect: []token{{typ: Decimal, token: "12345678901.23456789012e-12"}},
		pos: []Position{
			{Byte: 44, Line: 3, Char: 29},
			{Byte: 61, Line: 6, Char: 2},
		},
	},
	"number-float-comma": {
		input: "12345678901.23456789012e-12,", json: Json,
		expect: []token{
			{typ: Decimal, token: "12345678901.23456789012e-12"},
			{typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 27, Line: 0, Char: 27},
			{Byte: 28, Line: 0, Char: 28},
			{Byte: 28, Line: 0, Char: 28},
		},
	},
	"number-float-neg": {
		input: "-987654321012345.6789098e-12", json: Json,
		expect: []token{{typ: Decimal, token: "-987654321012345.6789098e-12"}},
		pos: []Position{
			{Byte: 28, Line: 0, Char: 28},
			{Byte: 28, Line: 0, Char: 28},
		},
	},
	"number-float-pos": {
		input: "+543.2101234567890123456e+7", json: Json5,
		expect: []token{{typ: Decimal, token: "+543.2101234567890123456e+7"}},
		pos: []Position{
			{Byte: 27, Line: 0, Char: 27},
			{Byte: 27, Line: 0, Char: 27},
		},
	},

	// Invalid decimal numbers.
	"number-float-sign-invalid": {
		input: "0e+a", json: Json5,
		expect: []token{{typ: String, token: "0e+a"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-float-invalid-char": {
		input: "1.2a", json: Json5,
		expect: []token{{typ: String, token: "1.2a"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-float-invalid-sign": {
		input: "1e+a", json: Json5,
		expect: []token{{typ: String, token: "1e+a"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-float-invalid-digit": {
		input: "1e2a", json: Json5,
		expect: []token{{typ: String, token: "1e2a"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-float-invalid-letter": {
		input: "1ea", json: Json5,
		expect: []token{{typ: String, token: "1ea"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},

	// Hexadecimal numbers.
	"number-hex": {
		input: "0x98ca3fe4", json: Json5,
		expect: []token{{typ: HexaDecimal, token: "0x98ca3fe4"}},
		pos: []Position{
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 10, Line: 0, Char: 10},
		},
	},
	"number-hex-space": {
		input: uspaces + "0x98ca3fe4" + uspaces_, json: Json5,
		expect: []token{{typ: HexaDecimal, token: "0x98ca3fe4"}},
		pos: []Position{
			{Byte: 27, Line: 3, Char: 12},
			{Byte: 44, Line: 6, Char: 2},
		},
	},
	"number-hex-comma": {
		input: "0x98ca3fe4,", json: Json5,
		expect: []token{
			{typ: HexaDecimal, token: "0x98ca3fe4"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"number-hex-neg": {
		input: "-0x0101010101010101", json: Json5,
		expect: []token{{typ: HexaDecimal, token: "-0x0101010101010101"}},
		pos: []Position{
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 19, Line: 0, Char: 19},
		},
	},
	"number-hex-pos": {
		input: "+0xefefefefefefefef", json: Json5,
		expect: []token{{typ: HexaDecimal, token: "+0xefefefefefefefef"}},
		pos: []Position{
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 19, Line: 0, Char: 19},
		},
	},
	"number-hex-invalid": {
		input: "0xG", json: Json5,
		expect: []token{{typ: String, token: "0xG"}},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-hex-digit-prefix": {
		input: "1x2,", json: Json5,
		expect: []token{
			{typ: HexaDecimal, token: "1x2"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},

	// Complex numbers.
	"number-complex": {
		input: "123.45+67.89e-2i", json: Json5Ext,
		expect: []token{{typ: Complex, token: "123.45+67.89e-2i"}},
		pos: []Position{
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 16, Line: 0, Char: 16},
		},
	},
	"number-complex-zero": {
		input: "0i,", json: Json5Ext,
		expect: []token{
			{typ: Complex, token: "0i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-complex-one": {
		input: "1i,", json: Json5Ext,
		expect: []token{
			{typ: Complex, token: "1i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 3, Line: 0, Char: 3},
		},
	},
	"number-complex-zero-sign": {
		input: "0+1i,", json: Json5Ext,
		expect: []token{
			{typ: Complex, token: "0+1i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"number-complex-virtual": {
		input: "123.45i", json: Json5Ext,
		expect: []token{{typ: Complex, token: "123.45i"}},
		pos: []Position{
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 7, Line: 0, Char: 7},
		},
	},
	"number-complex-short-comma": {
		input: "1+i,", json: Json5Ext,
		expect: []token{
			{typ: Complex, token: "1+i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"number-complex-space": {
		input: uspaces + "123.45+67.89e-2i" + uspaces_, json: Json5Ext,
		expect: []token{{typ: Complex, token: "123.45+67.89e-2i"}},
		pos: []Position{
			{Byte: 33, Line: 3, Char: 18},
			{Byte: 50, Line: 6, Char: 2},
		},
	},
	"number-complex-comma": {
		input: "123.45+67.89e-2i,", json: Json5Ext,
		expect: []token{
			{typ: Complex, token: "123.45+67.89e-2i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"number-complex-neg": {
		input: "-987.65e-2-43.21e-2i", json: Json5Ext,
		expect: []token{{typ: Complex, token: "-987.65e-2-43.21e-2i"}},
		pos: []Position{
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 20, Line: 0, Char: 20},
		},
	},
	"number-complex-pos": {
		input: "+456.78e+2+98.76e+2i", json: Json5Ext,
		expect: []token{{typ: Complex, token: "+456.78e+2+98.76e+2i"}},
		pos: []Position{
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 20, Line: 0, Char: 20},
		},
	},
	"number-complex-fail-numeric": {
		input: "1+2+3i,", json: Json5Ext,
		expect: []token{
			{typ: String, token: "1+2+3i"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 7, Line: 0, Char: 7},
		},
	},
	"number-complex-fail-invalid": {
		input: "1+2a,", json: Json5Ext,
		expect: []token{
			{typ: String, token: "1+2a"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},

	// Simple strings.
	"string-short": {
		input: "hi", json: Json5,
		expect: []token{{typ: String, token: "hi"}},
		pos: []Position{
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"string-word": {
		input: "hello", json: Json5,
		expect: []token{{typ: String, token: "hello"}},
		pos: []Position{
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 5, Line: 0, Char: 5},
		},
	},
	"string-word-space": {
		input: uspaces + "hello" + uspaces_, json: Json5,
		expect: []token{{typ: String, token: "hello"}},
		pos: []Position{
			{Byte: 39, Line: 6, Char: 2},
			{Byte: 39, Line: 6, Char: 2},
		},
	},
	"string-word-newline": {
		input: "\nhello", json: Json5,
		expect: []token{{typ: String, token: "hello"}},
		pos: []Position{
			{Byte: 6, Line: 1, Char: 5},
			{Byte: 6, Line: 1, Char: 5},
		},
	},
	"string-word-comma": {
		input: " hello ,", json: Json5,
		expect: []token{
			{typ: String, token: "hello"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-words": {
		input: "hello world", json: Json5,
		expect: []token{{typ: String, token: "hello world"}},
		pos: []Position{
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"string-words-space": {
		input: uspaces + "hello world" + uspaces_, json: Json5,
		expect: []token{{typ: String, token: "hello world"}},
		pos: []Position{
			{Byte: 45, Line: 6, Char: 2},
			{Byte: 45, Line: 6, Char: 2},
		},
	},
	"string-words-comma": {
		input: " hello world ,", json: Json5,
		expect: []token{
			{typ: String, token: "hello world"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 14, Line: 0, Char: 14},
		},
	},
	"string-text": {
		input: "A long sentence that represents a typical " +
			"description field without any escaping needed here.",
		json: Json5,
		expect: []token{{
			typ: String,
			token: "A long sentence that represents a typical " +
				"description field without any escaping needed here.",
		}},
		pos: []Position{
			{Byte: 93, Line: 0, Char: 93},
			{Byte: 93, Line: 0, Char: 93},
		},
	},
	"string-text-raw": {
		input: "hello 'world'" + uescapes_, json: Json5,
		expect: []token{{
			typ: String, token: "hello 'world'" + uescapes,
		}},
		pos: []Position{
			{Byte: 33, Line: 0, Char: 33},
			{Byte: 33, Line: 0, Char: 33},
		},
	},
	"string-text-escape": {
		input: "hello 'world'" + uescapes, json: Json5,
		expect: []token{{
			typ: String, token: "hello 'world'" + uescapes,
		}},
		pos: []Position{
			{Byte: 23, Line: 1, Char: 9},
			{Byte: 23, Line: 1, Char: 9},
		},
	},
	"string-text-escape-rare": {
		input: "First segment of about forty characters long. " +
			"'Then' second segment of equal length too.",
		json: Json5,
		expect: []token{{
			typ: String,
			token: "First segment of about forty characters long. " +
				"'Then' second segment of equal length too.",
		}},
		pos: []Position{
			{Byte: 88, Line: 0, Char: 88},
			{Byte: 88, Line: 0, Char: 88},
		},
	},
	"string-text-escape-known": {
		input: "hello\\nworld", json: Json5,
		expect: []token{{typ: String, token: "hello\nworld"}},
		pos: []Position{
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 12, Line: 0, Char: 12},
		},
	},
	"string-text-escape-known-delim": {
		input: "hello\\nworld,", json: Json5,
		expect: []token{
			{typ: String, token: "hello\nworld"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 13, Line: 0, Char: 13},
		},
	},
	"string-text-escape-delim": {
		input: "hello\\n,", json: Json5,
		expect: []token{
			{typ: String, token: "hello\n"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-text-escape-unicode": {
		input: "hi\\u0041bye", json: Json5,
		expect: []token{{typ: String, token: "hiAbye"}},
		pos: []Position{
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"string-text-escape-probe-unknown": {
		input: "hello\\n\\qworld", json: Json5,
		expect: []token{{typ: String, token: "hello\n\\qworld"}},
		pos: []Position{
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 14, Line: 0, Char: 14},
		},
	},

	// Single quoted strings.
	"string-quote-single-short": {
		input: "'hi'", json: Json,
		expect: []token{{typ: String, token: "hi"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"string-quote-single-word": {
		input: "'hello!'", json: Json,
		expect: []token{{typ: String, token: "hello!"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-quote-single-words": {
		input: "'hello \"world\"'", json: Json5,
		expect: []token{{typ: String, token: "hello \"world\""}},
		pos: []Position{
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},
	"string-quote-single-space": {
		input: uspaces + "'hello \\'world\\''" + uspaces_, json: Json5,
		expect: []token{{typ: String, token: "hello 'world'"}},
		pos: []Position{
			{Byte: 34, Line: 3, Char: 19},
			{Byte: 51, Line: 6, Char: 2},
		},
	},
	"string-quote-single-comma": {
		input: "'hello \\'world\\'' ,", json: Json5,
		expect: []token{
			{typ: String, token: "hello 'world'"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 19, Line: 0, Char: 19},
		},
	},
	"string-quote-single-text": {
		input: "'A long sentence that represents a typical " +
			"description field without any escaping needed here.'",
		json: Json,
		expect: []token{{
			typ: String,
			token: "A long sentence that represents a typical " +
				"description field without any escaping needed here.",
		}},
		pos: []Position{
			{Byte: 95, Line: 0, Char: 95},
			{Byte: 95, Line: 0, Char: 95},
		},
	},
	"string-quote-single-raw": {
		input: "'hello \\'world\\'" + uescapes_ + "'", json: Json5Ext,
		expect: []token{{
			typ: String, token: "hello 'world'" + uescapes,
		}},
		pos: []Position{
			{Byte: 37, Line: 0, Char: 37},
			{Byte: 37, Line: 0, Char: 37},
		},
	},
	"string-quote-single-escape": {
		input: "'hello \\'world\\'" + uescapes + "'", json: Json5Ext,
		expect: []token{{
			typ: String, token: "hello 'world'" + uescapes,
		}},
		pos: []Position{
			{Byte: 27, Line: 1, Char: 10},
			{Byte: 27, Line: 1, Char: 10},
		},
	},
	"string-quote-single-escape-rare": {
		input: "'First segment of about forty characters long. " +
			"\\'Then\\' second segment of equal length too.'",
		json: Json,
		expect: []token{{
			typ: String,
			token: "First segment of about forty characters long. " +
				"'Then' second segment of equal length too.",
		}},
		pos: []Position{
			{Byte: 92, Line: 0, Char: 92},
			{Byte: 92, Line: 0, Char: 92},
		},
	},
	"string-quote-single-bulk-unicode": {
		input: "'\\u0041'", json: Json5,
		expect: []token{{typ: String, token: "A"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-quote-single-newline": {
		input: "'hello\nworld'", json: Json5,
		expect: []token{{typ: String, token: "hello\nworld"}},
		pos: []Position{
			{Byte: 13, Line: 1, Char: 6},
			{Byte: 13, Line: 1, Char: 6},
		},
	},
	"string-quote-single-newline-short": {
		input: "'\n'", json: Json5,
		expect: []token{{typ: String, token: "\n"}},
		pos: []Position{
			{Byte: 3, Line: 1, Char: 1},
			{Byte: 3, Line: 1, Char: 1},
		},
	},
	"string-quote-single-eof": {
		input: "'hello", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-single-eof-escape": {
		input: "'hello\\", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-single-eof-probe": {
		input: "'hello\\n", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-single-probe-unknown": {
		input: "'hello\\n\\q'", json: Json5,
		expect: []token{{
			typ: String, token: "hello\n\\q",
		}},
		pos: []Position{
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"string-quote-single-probe-unicode": {
		input: "'hello\\n\\u0041'", json: Json5,
		expect: []token{{
			typ: String, token: "hello\nA",
		}},
		pos: []Position{
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},

	// Double quoted strings.
	"string-quote-double-short": {
		input: "\"hi\"", json: Json,
		expect: []token{{typ: String, token: "hi"}},
		pos: []Position{
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},
	"string-quote-double-word": {
		input: "\"hello!\"", json: Json,
		expect: []token{{typ: String, token: "hello!"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-quote-double-words": {
		input: "\"hello world\"", json: Json,
		expect: []token{{typ: String, token: "hello world"}},
		pos: []Position{
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 13, Line: 0, Char: 13},
		},
	},
	"string-quote-double-space": {
		input: uspaces + "\"hello world\"" + uspaces_, json: Json,
		expect: []token{{typ: String, token: "hello world"}},
		pos: []Position{
			{Byte: 30, Line: 3, Char: 15},
			{Byte: 47, Line: 6, Char: 2},
		},
	},
	"string-quote-double-comma": {
		input: "\"hello world\" ,", json: Json5,
		expect: []token{
			{typ: String, token: "hello world"}, {typ: Comma, token: ","},
		},
		pos: []Position{
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},
	"string-quote-double-text": {
		input: "\"A long sentence that represents a typical " +
			"description field without any escaping needed here.\"",
		json: Json,
		expect: []token{{
			typ: String,
			token: "A long sentence that represents a typical " +
				"description field without any escaping needed here.",
		}},
		pos: []Position{
			{Byte: 95, Line: 0, Char: 95},
			{Byte: 95, Line: 0, Char: 95},
		},
	},
	"string-quote-double-raw": {
		input: "\"hello \\\"world\\\"" + uescapes_ + "\"", json: Json5,
		expect: []token{{
			typ: String, token: "hello \"world\"" + uescapes,
		}},
		pos: []Position{
			{Byte: 37, Line: 0, Char: 37},
			{Byte: 37, Line: 0, Char: 37},
		},
	},
	"string-quote-double-escape": {
		input: "\"hello \\\"world\\\"" + uescapes + "\"", json: Json5,
		expect: []token{{
			typ: String, token: "hello \"world\"" + uescapes,
		}},
		pos: []Position{
			{Byte: 27, Line: 1, Char: 10},
			{Byte: 27, Line: 1, Char: 10},
		},
	},
	"string-quote-double-escape-rare": {
		input: "\"First segment of about forty characters long. " +
			"\\\"Then\\\" second segment of equal length too.\"",
		json: Json,
		expect: []token{{
			typ: String,
			token: "First segment of about forty characters long. " +
				"\"Then\" second segment of equal length too.",
		}},
		pos: []Position{
			{Byte: 92, Line: 0, Char: 92},
			{Byte: 92, Line: 0, Char: 92},
		},
	},
	"string-quote-double-bulk-unicode": {
		input: "\"\\u0041\"", json: Json5,
		expect: []token{{typ: String, token: "A"}},
		pos: []Position{
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 8, Line: 0, Char: 8},
		},
	},
	"string-quote-double-newline": {
		input: "\"hello\nworld\"", json: Json5,
		expect: []token{{typ: String, token: "hello\nworld"}},
		pos: []Position{
			{Byte: 13, Line: 1, Char: 6},
			{Byte: 13, Line: 1, Char: 6},
		},
	},
	"string-quote-double-newline-short": {
		input: "\"\n\"", json: Json5,
		expect: []token{{typ: String, token: "\n"}},
		pos: []Position{
			{Byte: 3, Line: 1, Char: 1},
			{Byte: 3, Line: 1, Char: 1},
		},
	},
	"string-quote-double-eof": {
		input: "\"hello", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-double-eof-escape": {
		input: "\"hello\\", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-double-eof-probe": {
		input: "\"hello\\n", json: Json5,
		expect: []token{{typ: EOF, token: ""}},
		pos: []Position{
			{Byte: 0, Line: 0, Char: 0},
			{Byte: 0, Line: 0, Char: 0},
		},
	},
	"string-quote-double-probe-unknown": {
		input: "\"hello\\n\\q\"", json: Json5,
		expect: []token{{
			typ: String, token: "hello\n\\q",
		}},
		pos: []Position{
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"string-quote-double-probe-unicode": {
		input: "\"hello\\n\\u0041\"", json: Json5,
		expect: []token{{
			typ: String, token: "hello\nA",
		}},
		pos: []Position{
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 15, Line: 0, Char: 15},
		},
	},

	// Simple and complex arrays.
	"array-empty": {
		input: "[]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["}, {typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"array-empty-space": {
		input: uspaces + "[ ]" + uspaces_, json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["}, {typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 18, Line: 3, Char: 3},
			{Byte: 20, Line: 3, Char: 5},
			{Byte: 37, Line: 6, Char: 2},
		},
	},
	"array-token": {
		input: "[null,true,false]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Null, token: "null"},
			{typ: Comma, token: ","},
			{typ: True, token: "true"},
			{typ: Comma, token: ","},
			{typ: False, token: "false"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"array-token-space": {
		input: "[ null , true , false ]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Null, token: "null"},
			{typ: Comma, token: ","},
			{typ: True, token: "true"},
			{typ: Comma, token: ","},
			{typ: False, token: "false"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 23, Line: 0, Char: 23},
			{Byte: 23, Line: 0, Char: 23},
		},
	},
	"array-string": {
		input: "[\"hello\",\"world\"]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: String, token: "hello"},
			{typ: Comma, token: ","},
			{typ: String, token: "world"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"array-string-space": {
		input: "[ \"hello\" , \"world\" ]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: String, token: "hello"},
			{typ: Comma, token: ","},
			{typ: String, token: "world"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 21, Line: 0, Char: 21},
		},
	},
	"array-number": {
		input: "[9,0.123e-4]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Integer, token: "9"},
			{typ: Comma, token: ","},
			{typ: Decimal, token: "0.123e-4"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 12, Line: 0, Char: 12},
		},
	},
	"array-number-space": {
		input: "[ 9 , 0.123e-4 ]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Integer, token: "9"},
			{typ: Comma, token: ","},
			{typ: Decimal, token: "0.123e-4"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 16, Line: 0, Char: 16},
		},
	},
	"array-number-ext": {
		input: "[NaN,Infinity,0x1a3f,1+2i]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: NaN, token: "NaN"},
			{typ: Comma, token: ","},
			{typ: Infinity, token: "Infinity"},
			{typ: Comma, token: ","},
			{typ: HexaDecimal, token: "0x1a3f"},
			{typ: Comma, token: ","},
			{typ: Complex, token: "1+2i"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 25, Line: 0, Char: 25},
			{Byte: 26, Line: 0, Char: 26},
			{Byte: 26, Line: 0, Char: 26},
		},
	},
	"array-number-ext-space": {
		input: "[ NaN , Infinity , 0x1a3f , 1+2i ]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: NaN, token: "NaN"},
			{typ: Comma, token: ","},
			{typ: Infinity, token: "Infinity"},
			{typ: Comma, token: ","},
			{typ: HexaDecimal, token: "0x1a3f"},
			{typ: Comma, token: ","},
			{typ: Complex, token: "1+2i"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 18, Line: 0, Char: 18},
			{Byte: 25, Line: 0, Char: 25},
			{Byte: 27, Line: 0, Char: 27},
			{Byte: 32, Line: 0, Char: 32},
			{Byte: 34, Line: 0, Char: 34},
			{Byte: 34, Line: 0, Char: 34},
		},
	},
	"array-object-empty": {
		input: "[{},{}]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: ObjectStart, token: "{"},
			{typ: ObjectEnd, token: "}"},
			{typ: Comma, token: ","},
			{typ: ObjectStart, token: "{"},
			{typ: ObjectEnd, token: "}"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 7, Line: 0, Char: 7},
		},
	},
	"array-object-minimal": {
		input: "[{\"key\":\"value\"}]", json: Json,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "value"},
			{typ: ObjectEnd, token: "}"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 17, Line: 0, Char: 17},
		},
	},
	"array-object-relaxed": {
		input: "[,{key:value,},]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Comma, token: ","},
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "value"},
			{typ: Comma, token: ","},
			{typ: ObjectEnd, token: "}"},
			{typ: Comma, token: ","},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 14, Line: 0, Char: 14},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 16, Line: 0, Char: 16},
		},
	},

	// Multi-comma array cases.
	"array-comma-leading": {
		input: "[,1,2]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Comma, token: ","},
			{typ: Integer, token: "1"},
			{typ: Comma, token: ","},
			{typ: Integer, token: "2"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 6, Line: 0, Char: 6},
		},
	},
	"array-comma-trailing": {
		input: "[1,2,]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Integer, token: "1"},
			{typ: Comma, token: ","},
			{typ: Integer, token: "2"},
			{typ: Comma, token: ","},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 6, Line: 0, Char: 6},
		},
	},
	"array-comma-multi": {
		input: "[1,,2]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Integer, token: "1"},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: Integer, token: "2"},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 6, Line: 0, Char: 6},
		},
	},
	"array-comma-only": {
		input: "[,,,,]", json: Json5Ext,
		expect: []token{
			{typ: ArrayStart, token: "["},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: ArrayEnd, token: "]"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 6, Line: 0, Char: 6},
		},
	},

	// Simple and complex objects.
	"object-empty": {
		input: "{}", json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"}, {typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 2, Line: 0, Char: 2},
		},
	},
	"object-empty-space": {
		input: uspaces + "{ }" + uspaces_, json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"}, {typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 18, Line: 3, Char: 3},
			{Byte: 20, Line: 3, Char: 5},
			{Byte: 37, Line: 6, Char: 2},
		},
	},
	"object-simple": {
		input: "{\"null\":null,\"key\":\"value\"}", json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "null"},
			{typ: Colon, token: ":"},
			{typ: Null, token: "null"},
			{typ: Comma, token: ","},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "value"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 13, Line: 0, Char: 13},
			{Byte: 18, Line: 0, Char: 18},
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 26, Line: 0, Char: 26},
			{Byte: 27, Line: 0, Char: 27},
			{Byte: 27, Line: 0, Char: 27},
		},
	},
	"object-simple-space": {
		input: "{ \"null\" : null , \"key\" : \"value\" }", json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "null"},
			{typ: Colon, token: ":"},
			{typ: Null, token: "null"},
			{typ: Comma, token: ","},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "value"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 23, Line: 0, Char: 23},
			{Byte: 25, Line: 0, Char: 25},
			{Byte: 33, Line: 0, Char: 33},
			{Byte: 35, Line: 0, Char: 35},
			{Byte: 35, Line: 0, Char: 35},
		},
	},
	"object-array-empty": {
		input: "{\"array\":[]}", json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "array"},
			{typ: Colon, token: ":"},
			{typ: ArrayStart, token: "["},
			{typ: ArrayEnd, token: "]"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 12, Line: 0, Char: 12},
			{Byte: 12, Line: 0, Char: 12},
		},
	},
	"object-array-ext": {
		input: "{:[NaN,Infinity,],:,}", json: Json5Ext,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: Colon, token: ":"},
			{typ: ArrayStart, token: "["},
			{typ: NaN, token: "NaN"},
			{typ: Comma, token: ","},
			{typ: Infinity, token: "Infinity"},
			{typ: Comma, token: ","},
			{typ: ArrayEnd, token: "]"},
			{typ: Comma, token: ","},
			{typ: Colon, token: ":"},
			{typ: Comma, token: ","},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 7, Line: 0, Char: 7},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 18, Line: 0, Char: 18},
			{Byte: 19, Line: 0, Char: 19},
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 21, Line: 0, Char: 21},
		},
	},

	// Multi-comma object cases.
	"object-comma": {
		input: "{,key:val,}", json: Json5Ext,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: Comma, token: ","},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "val"},
			{typ: Comma, token: ","},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
	"object-comma-multi": {
		input: "{key:val,,,other:val}", json: Json5Ext,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: String, token: "key"},
			{typ: Colon, token: ":"},
			{typ: String, token: "val"},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: String, token: "other"},
			{typ: Colon, token: ":"},
			{typ: String, token: "val"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 8, Line: 0, Char: 8},
			{Byte: 9, Line: 0, Char: 9},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 17, Line: 0, Char: 17},
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 21, Line: 0, Char: 21},
		},
	},
	"object-comma-only": {
		input: "{,,}", json: Json5Ext,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: Comma, token: ","},
			{typ: Comma, token: ","},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 2, Line: 0, Char: 2},
			{Byte: 3, Line: 0, Char: 3},
			{Byte: 4, Line: 0, Char: 4},
			{Byte: 4, Line: 0, Char: 4},
		},
	},

	// Object keys that are also values.
	"object-token": {
		input: "{null:null,true:true,false:false}", json: Json,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: Null, token: "null"},
			{typ: Colon, token: ":"},
			{typ: Null, token: "null"},
			{typ: Comma, token: ","},
			{typ: True, token: "true"},
			{typ: Colon, token: ":"},
			{typ: True, token: "true"},
			{typ: Comma, token: ","},
			{typ: False, token: "false"},
			{typ: Colon, token: ":"},
			{typ: False, token: "false"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 15, Line: 0, Char: 15},
			{Byte: 16, Line: 0, Char: 16},
			{Byte: 20, Line: 0, Char: 20},
			{Byte: 21, Line: 0, Char: 21},
			{Byte: 26, Line: 0, Char: 26},
			{Byte: 27, Line: 0, Char: 27},
			{Byte: 32, Line: 0, Char: 32},
			{Byte: 33, Line: 0, Char: 33},
			{Byte: 33, Line: 0, Char: 33},
		},
	},
	"object-complex": {
		input: "{1+2i:1+2i}", json: Json5Ext,
		expect: []token{
			{typ: ObjectStart, token: "{"},
			{typ: Complex, token: "1+2i"},
			{typ: Colon, token: ":"},
			{typ: Complex, token: "1+2i"},
			{typ: ObjectEnd, token: "}"},
		},
		pos: []Position{
			{Byte: 1, Line: 0, Char: 1},
			{Byte: 5, Line: 0, Char: 5},
			{Byte: 6, Line: 0, Char: 6},
			{Byte: 10, Line: 0, Char: 10},
			{Byte: 11, Line: 0, Char: 11},
			{Byte: 11, Line: 0, Char: 11},
		},
	},
}

// setupScanNext returns a scanning function that uses the provided functions
// for scanning quoted strings, numbers, and relaxed tokens. This allows for
// flexible tokenization strategies while maintaining a consistent scanning
// loop.
func setupScanNext(
	quoted func(*Scanner, byte) (byte, []byte, int),
	numbers func(*Scanner, int, []byte) (byte, int, int),
	relaxed func(*Scanner) (byte, []byte, int),
) func(*Scanner) (byte, []byte) {
	return func(s *Scanner) (byte, []byte) {
		if skip := s.skip(0); skip != 0 {
			s.reader.advance(skip)
		}

		char, ok := s.reader.peek()
		if !ok {
			return EOF, nil
		}

		switch mask[char] & base {
		case delim:
			return s.commit(char, []byte{char}, 1)

		case keyword:
			if offset, _ := s.keyword(char); offset != 0 {
				token := s.reader.window()[:offset]

				return s.commit(char, token, offset)
			}

		case number:
			if typ, offset, _ := numbers(s, 0, s.reader.window()); typ != EOF {
				token := s.reader.window()[:offset]

				return s.commit(typ, token, offset)
			}

		case strng:
			return s.commit(quoted(s, char))
		}

		return s.commit(relaxed(s))
	}
}

// setupScanNextx returns a scanning function that uses the provided functions
// for scanning quoted strings, numbers, and relaxed tokens. This allows for
// flexible tokenization strategies while maintaining a consistent scanning
// loop.
func setupScanNextx(
	quoted func(*Scanner, byte) (byte, []byte, int),
	numbers func(*Scanner, int, []byte) (byte, int, int),
	relaxed func(*Scanner) (byte, []byte, int),
) func(*Scanner) (byte, []byte) {
	return func(s *Scanner) (byte, []byte) {
		if skip := s.skip(0); skip != 0 {
			s.reader.advance(skip)
		}

		ch, ok := s.reader.peek()
		if !ok {
			return EOF, nil
		}

		kind := mask[ch] & base
		//nolint:staticcheck // priority speed.
		if kind == delim {
			return s.commit(ch, []byte{ch}, 1)
		} else if kind == strng {
			return s.commit(quoted(s, ch))
		} else if kind == keyword {
			if offset, _ := s.keyword(ch); offset != 0 {
				token := s.reader.window()[:offset]

				return s.commit(ch, token, offset)
			}
		} else if kind == number {
			if typ, offset, _ := numbers(s, 0, s.reader.window()); typ != EOF {
				token := s.reader.window()[:offset]

				return s.commit(typ, token, offset)
			}
		}

		return s.commit(relaxed(s))
	}
}

// setupScanNext_ returns a location aware scanning function that uses the
// provided functions for scanning quoted strings, numbers, and relaxed tokens.
// This allows for flexible tokenization strategies for testing while
// maintaining a consistent scanning loop.
func setupScanNext_(
	quoted func(*Scanner, byte) (byte, []byte, int, int, int),
	numbers func(*Scanner, int, []byte) (byte, int, int),
	relaxed func(*Scanner) (byte, []byte, int, int, int),
) func(*Scanner) (byte, []byte) {
	return func(s *Scanner) (byte, []byte) {
		offset, line, char, token := s.space_()
		if offset != 0 {
			return s.commit_(Space, token, offset, line, char)
		}

		ch, ok := s.reader.peek()
		if !ok {
			return EOF, nil
		}

		switch mask[ch] & base {
		case delim:
			return s.commit_(ch, []byte{ch}, 1, 0, 1)

		case comment:
			typ, token, offset, line, char := s.comment_(0)
			if offset != 0 {
				return s.commit_(typ, token, offset, line, char)
			}

		case keyword:
			if offset, _ := s.keyword(ch); offset != 0 {
				token := s.reader.window()[:offset]

				return s.commit_(ch, token, offset, 0, offset)
			}

		case number:
			ch, offset, _ := numbers(s, 0, s.reader.window())
			if ch != EOF {
				token := s.reader.window()[:offset]

				return s.commit_(ch, token, offset, 0, offset)
			}

		case strng:
			return s.commit_(quoted(s, ch))
		}

		return s.commit_(relaxed(s))
	}
}

// setupScanNext_x returns a location aware scanning function that uses the
// provided functions for scanning quoted strings, numbers, and relaxed tokens.
// This allows for flexible tokenization strategies for testing while
// maintaining a consistent scanning loop.
//
//revive:disable:max-control-nesting // prioritize performance.
func setupScanNext_x(
	quoted func(*Scanner, byte) (byte, []byte, int, int, int),
	numbers func(*Scanner, int, []byte) (byte, int, int),
	relaxed func(*Scanner) (byte, []byte, int, int, int),
) func(*Scanner) (byte, []byte) {
	return func(s *Scanner) (byte, []byte) {
		offset, line, char, token := s.space_()
		if offset != 0 {
			return s.commit_(Space, token, offset, line, char)
		}

		ch, ok := s.reader.peek()
		if !ok {
			return EOF, nil
		}

		kind := mask[ch] & base
		//nolint:staticcheck // priority speed.
		if kind == delim {
			return s.commit_(ch, []byte{ch}, 1, 0, 1)
		} else if kind == strng {
			return s.commit_(quoted(s, ch))
		} else if kind == keyword {
			if offset, _ := s.keyword(ch); offset != 0 {
				token := s.reader.window()[:offset]

				return s.commit_(ch, token, offset, 0, offset)
			}
		} else if kind == number {
			typ, offset, _ := numbers(s, 0, s.reader.window())
			if typ != EOF {
				token := s.reader.window()[:offset]

				return s.commit_(typ, token, offset, 0, offset)
			}
		} else if kind == comment {
			typ, token, offset, line, char := s.comment_(0)
			if offset != 0 {
				return s.commit_(typ, token, offset, line, char)
			}
		}

		return s.commit_(relaxed(s))
	}
}

//revive:enable:max-control-nesting

// testScanNext tests the scanning of tokens using the provided next function.
// It iterates through the expected tokens and positions, comparing them
// against the actual results produced by the scanner. It also checks for the
// EOF error at the end of the input.
func testScanNext(
	t *testing.T, name string, size int, skip bool,
	next func(*Scanner) (byte, []byte),
	filter ...test.FilterFunc[scanParams],
) {
	test.Map(t, scanTestCases).
		Prefix("method=" + name + "/test=").
		Filter(filter...).Filter(types(Next)).
		// Filter(test.Pattern[scanParams]("next/test=token-bool-space$")).
		Run(func(t test.Test, param scanParams) {
			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(param.input), 1),
				make([]byte, 0, size))

			pos := append([]Position{{Byte: 0, Line: 0, Char: 0}}, param.pos...)
			expect := param.expect
			expect = append(expect, token{typ: EOF, token: ""})

			skipped := 0
			for index, expect := range expect {
				if skip && (expect.typ == Space ||
					expect.typ == Comment ||
					expect.typ == CommentMulti) {
					skipped++
					continue
				}

				// When
				before := scanner.Position()
				typ, token := next(scanner)
				after := scanner.Position()

				// Drain Space tokens; they are trivia not in param.expect.
				for !skip && typ == Space {
					typ, token = next(scanner)
					after = scanner.Position()
				}

				// Then
				assert.Equal(t, string(expect.typ), string(typ),
					"type: %d, msg: %s", index, param.input)
				assert.Equal(t, expect.token, string(token),
					"token: %d, msg: %s", index, param.input)
				assertPosition(t, name, pos[index-skipped], before,
					"pos: %d, msg: %s", index, param.input)
				assertPosition(t, name, pos[index+1], after,
					"pos: %d, msg: %s", index+1, param.input)
				skipped = 0
			}

			assert.Equal(t, io.EOF, scanner.Error())
		})
}

// TestScanNext tests the scanning of tokens using various next functions,
// including the location-aware next function.
func TestScanNext(t *testing.T) {
	size := 1 << 6

	testScanNext(t, "next", size+1, true, (*Scanner).Next)
	testScanNext(t, "next-numbers", size+1, true, setupScanNext(
		(*Scanner).quoted, (*Scanner).numbers, (*Scanner).relaxed),
		filterMaxMode(Json5))
	testScanNext(t, "next-quoted", size, true, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	testScanNext(t, "nextx-quoted", size, true, setupScanNextx(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	testScanNext(t, "next-quotedx", size, true, setupScanNext(
		(*Scanner).quotedx, (*Scanner).number, (*Scanner).relaxed))
	testScanNext(t, "next-relaxedx", size, true, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxedx),
		filterMaxMode(Json5))

	testScanNext(t, "next-debug", size+1, false,
		(*Scanner).Next_, test.All[scanParams]())
	testScanNext(t, "next-debug-quoted", size+1, false, setupScanNext_(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	testScanNext(t, "nextx-debug-quoted", size+1, false, setupScanNext_x(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	testScanNext(t, "next-debug-quotedx", size, false, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_))
	testScanNext(t, "next-debug-relaxedx", size, false, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_x))
}

// benchmarkScanNext benchmarks the scanning of tokens using the provided next
// function. It iterates through the expected tokens and positions, comparing
// them against the actual results produced by the scanner. It also checks for
// the EOF error at the end of the input.
func benchmarkScanNext(
	b *testing.B, name string, size int,
	next func(*Scanner) (byte, []byte),
	filter ...test.FilterFunc[scanParams],
) {
	buffer := make([]byte, 0, size)

	test.Map(test.Benchmark(b), scanTestCases).
		Prefix("method=" + name + "/test=").
		Filter(filter...).Filter(types(Next)).
		Benchmark(func(b *testing.B, param scanParams) func(*testing.B) {
			// Setup
			reader := strings.NewReader(param.input)
			scanner := Scanner{}

			b.SetBytes(reader.Size())

			// Loop
			return func(*testing.B) {
				test.Must(reader.Seek(0, 0))
				scanner = Scanner{
					reader: *NewReader(buffer[:0], reader),
				}

				for {
					typ, token := next(&scanner)
					if typ == EOF {
						break
					}
					runtime.KeepAlive(token)
				}
			}
		})
}

// BenchmarkScanNext benchmarks the scanning of tokens using various next
// functions, including the location-aware next function.
func BenchmarkScanNext(b *testing.B) {
	size := 1 << 12

	benchmarkScanNext(b, "next", size+1, (*Scanner).Next)
	benchmarkScanNext(b, "next-numbers", size+1, setupScanNext(
		(*Scanner).quoted, (*Scanner).numbers, (*Scanner).relaxed),
		filterMaxMode(Json5))
	benchmarkScanNext(b, "next-quoted", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanNext(b, "nextx-quoted", size, setupScanNextx(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanNext(b, "next-quotedx", size, setupScanNext(
		(*Scanner).quotedx, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanNext(b, "next-relaxedx", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxedx))

	benchmarkScanNext(b, "next-debug", size+1,
		(*Scanner).Next_, test.All[scanParams]())
	benchmarkScanNext(b, "next-debug-quoted", size, setupScanNext_(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanNext(b, "nextx-debug-quoted", size, setupScanNext_x(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanNext(b, "next-debug-quotedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanNext(b, "next-debug-relaxedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_x))
}

// testScanFile tests the scanning of tokens from a file using the provided next
// function. It counts the number of tokens produced by the scanner and compares
// it against the expected number of tokens for the given file. It also checks
// for any errors encountered during scanning.
func testScanFile(
	t *testing.T, name string, size int,
	next func(*Scanner) (byte, []byte),
) {
	test.Map(t, fileTestCase).
		Prefix("method=" + name + "/test=").
		Run(func(t test.Test, param fileParams) {
			// Given
			scanner := NewScanner(
				getFixtureReader(t, param.path),
				make([]byte, 0, size),
			)

			tokens, chars, spaces := 0, 0, 0
			for {
				// When
				typ, token := next(scanner)

				// Then
				if typ == EOF {
					break
				} else if typ == Space {
					spaces += len(token)
				} else {
					chars += len(token)
					tokens++
				}
			}

			assert.Equal(t, param.tokens, tokens, "tokens")
			assert.Equal(t, param.chars, chars, "chars")
			if spaces != 0 {
				assert.Equal(t, param.space, spaces, "spaces")
			}
			assert.Equal(t, io.EOF, scanner.Error(), "error")
		})
}

// TestScanFile tests the scanning of tokens from various files using different
// next functions.
func TestScanFile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	size := 1 << 12

	testScanFile(t, "next", size, (*Scanner).Next)
	testScanFile(t, "next-numbers", size+1, setupScanNext(
		(*Scanner).quoted, (*Scanner).numbers, (*Scanner).relaxed))
	testScanFile(t, "next-quoted", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	testScanFile(t, "next-quotedx", size, setupScanNext(
		(*Scanner).quotedx, (*Scanner).number, (*Scanner).relaxed))
	testScanFile(t, "next-relaxedx", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxedx))

	testScanFile(t, "next-debug", size, (*Scanner).Next_)
	testScanFile(t, "next-debug-quoted", size, setupScanNext_(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	testScanFile(t, "next-debug-quotedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_))
	testScanFile(t, "next-debug-relaxedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_x))
}

// benchmarkScanFile benchmarks the scanning of tokens from a file using the
// provided next function. It counts the number of tokens produced by the
// scanner and compares it against the expected number of tokens for the given
// file. It also checks for any errors encountered during scanning.
func benchmarkScanFile(
	b *testing.B, name string, size int,
	next func(*Scanner) (byte, []byte),
) {
	test.Map(test.Benchmark(b), fileTestCase).
		Prefix("method=" + name + "/test=").
		Benchmark(func(b *testing.B, param fileParams) func(*testing.B) {
			// Setup
			buffer := make([]byte, size)
			reader := getFixtureReader(b, param.path)
			scanner := Scanner{}

			b.SetBytes(reader.Size())

			// Loop
			return func(*testing.B) {
				test.Must(reader.Seek(0, 0))
				scanner = Scanner{
					reader: *NewReader(buffer[:0], reader),
				}

				tokens, chars, spaces := 0, 0, 0
				for {
					// When
					typ, token := next(&scanner)

					// Then
					if typ == EOF {
						break
					} else if typ == Space {
						spaces += len(token)
					} else {
						chars += len(token)
						tokens++
					}
				}

				if tokens != param.tokens {
					b.Fatalf("expected %v tokens, got %v",
						param.tokens, tokens)
				} else if chars != param.chars {
					b.Fatalf("expected %v chars, got %v",
						param.chars, chars)
				}
			}
		})
}

// BenchmarkScanFile benchmarks the scanning of tokens from various files using
// different next functions. It measures the performance of the scanning process
// for each file and next function combination.
func BenchmarkScanFile(b *testing.B) {
	size := 8 << 10

	benchmarkScanFile(b, "next", size+1, (*Scanner).Next)
	benchmarkScanFile(b, "next-numbers", size+1, setupScanNext(
		(*Scanner).quoted, (*Scanner).numbers, (*Scanner).relaxed))
	benchmarkScanFile(b, "next-quoted", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanFile(b, "nextx-quoted", size, setupScanNextx(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanFile(b, "next-quotedx", size, setupScanNext(
		(*Scanner).quotedx, (*Scanner).number, (*Scanner).relaxed))
	benchmarkScanFile(b, "next-relaxedx", size, setupScanNext(
		(*Scanner).quoted, (*Scanner).number, (*Scanner).relaxedx))

	benchmarkScanFile(b, "next-debug", size+1, (*Scanner).Next_)
	benchmarkScanFile(b, "next-debug-quoted", size, setupScanNext_(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanFile(b, "nextx-debug-quoted", size, setupScanNext_x(
		(*Scanner).quoted_, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanFile(b, "next-debug-quotedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_))
	benchmarkScanFile(b, "next-debug-relaxedx", size, setupScanNext_(
		(*Scanner).quoted_x, (*Scanner).number, (*Scanner).relaxed_x))
}

//
// Scanning JSON keywords (true, false, null, Infinity, NaN).
//

// keywordParams defines the parameters for testing the scanning of JSON
// keywords.
type keywordParams struct {
	input  string
	char   byte
	expect int
	skip   int
	ok     bool
}

// keywordTestCases defines a set of test cases for scanning JSON keywords.
var keywordTestCases = map[string]keywordParams{
	"invalid": {
		input: "x", char: 'x',
		expect: 0, skip: 0, ok: false,
	},
	"true": {
		input: "true", char: 't',
		expect: 4, skip: 0, ok: true,
	},
	"true-invalid": {
		char: 't', input: "trux",
		expect: 0, skip: 0, ok: false,
	},
	"false": {
		char: 'f', input: "false",
		expect: 5, skip: 0, ok: true,
	},
	"false-invalid": {
		char: 'f', input: "falsx",
		expect: 0, skip: 0, ok: false,
	},
	"null": {
		char: 'n', input: "null",
		expect: 4, skip: 0, ok: true,
	},
	"null-invalid": {
		char: 'n', input: "nulx",
		expect: 0, skip: 0, ok: false,
	},
	"infinity": {
		char: 'I', input: "Infinity",
		expect: 8, skip: 0, ok: true,
	},
	"infinity-invalid": {
		char: 'I', input: "Infinitx",
		expect: 0, skip: 0, ok: false,
	},
	"nan": {
		char: 'N', input: "NaN",
		expect: 3, skip: 0, ok: true,
	},
	"nan-invalid": {
		char: 'N', input: "NaX",
		expect: 0, skip: 0, ok: false,
	},
}

// testScanKeyword tests the scanning of JSON keywords using the provided
// function. It iterates through the test cases, checking both valid and
// invalid inputs to ensure correct behavior.
func testScanKeyword(
	t *testing.T, name string,
	call func(*Scanner, byte) (int, int),
) {
	test.Map(t, keywordTestCases).
		Prefix("method=" + name + "/test=").
		Filter(func(name string, p keywordParams) bool {
			if strings.Contains(name, "method=keywordx/") {
				return keywords[p.char] != nil
			}
			return true
		}).
		Run(func(t test.Test, param keywordParams) {
			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(param.input), 1),
				make([]byte, 0, 64))
			scanner.reader.extend()

			// When
			offset, skip := call(scanner, param.char)

			// Then
			if param.ok {
				assert.Equal(t, param.expect, offset)
				assert.Equal(t, param.skip, skip)
			} else {
				assert.Equal(t, 0, offset)
				assert.Equal(t, 0, skip)
			}
		})
}

// benchmarkScanKeyword benchmarks the scanning of JSON keywords using the
// provided function. It iterates through the test cases, measuring the
// performance of the keyword scanning function for valid and invalid inputs.
func benchmarkScanKeyword(
	b *testing.B, name string,
	call func(*Scanner, byte) (int, int),
) {
	test.Map(test.Benchmark(b), keywordTestCases).
		Prefix("method=" + name + "/test=").
		Filter(func(_ string, p keywordParams) bool { return p.ok }).
		Benchmark(func(b *testing.B, param keywordParams) func(*testing.B) {
			// Setup
			scanner := NewScanner(
				strings.NewReader(param.input),
				make([]byte, 0, 64),
			)
			scanner.reader.extend()

			b.SetBytes(int64(len(param.input)))

			// Loop
			return func(*testing.B) {
				offset, skip := call(scanner, param.char)

				runtime.KeepAlive(offset)
				runtime.KeepAlive(skip)
			}
		})
}

// TestScanKeyword tests the scanning of JSON keywords using both the standard
// keyword function and the extended keywordx function, ensuring that they
// correctly identify valid keywords and reject invalid ones.
func TestScanKeyword(t *testing.T) {
	testScanKeyword(t, "keyword", (*Scanner).keyword)
	testScanKeyword(t, "keywordx", (*Scanner).keywordx)
}

// BenchmarkScanKeyword benchmarks the scanning of JSON keywords using both the
// standard keyword function and the extended keywordx function, measuring their
// performance for valid keyword inputs.
func BenchmarkScanKeyword(b *testing.B) {
	benchmarkScanKeyword(b, "keyword", (*Scanner).keyword)
	benchmarkScanKeyword(b, "keywordx", (*Scanner).keywordx)
}

//
// Scanning numbers.
//

func setupValidNumber(
	t test.Test, param scanParams, fail ...JsonType,
) (string, []token) {
	input, expect := param.input, slices.Clone(param.expect)
	name := t.Name()[strings.LastIndex(t.Name(), "=")+1:]

	switch expect[0].typ {
	case Integer:
		_, ok := new(big.Int).SetString(expect[0].token, 0)
		assert.True(t, ok, "invalid big integer number: '%s'", input)
		_, err := strconv.ParseInt(expect[0].token, 0, 64)
		assert.NoError(t, err, "invalid integer number: '%s'", input)
		if param.json == Json {
			err = json.Unmarshal([]byte(expect[0].token), new(big.Int))
			assert.NoError(t, err, "invalid big integer number: '%s'", input)
			err = json.Unmarshal([]byte(expect[0].token), new(int64))
			assert.NoError(t, err, "invalid integer number: '%s'", input)
		}
	case HexaDecimal:
		if signedHexPrefixRegex.MatchString(expect[0].token) {
			_, ok := new(big.Int).SetString(expect[0].token, 0)
			assert.True(t, ok, "invalid big hexadecimal number: '%s'", input)
			if param.json == Json {
				err := json.Unmarshal([]byte(expect[0].token), new(big.Int))
				assert.NoError(t, err,
					"invalid big hexadecimal number: '%s'", input)
			}
		}
	case Decimal:
		_, ok := new(big.Float).SetString(expect[0].token)
		assert.True(t, ok, "invalid big decimal number: '%s'", input)
		_, err := strconv.ParseFloat(expect[0].token, 64)
		assert.NoError(t, err, "invalid decimal number: '%s'", input)
		if param.json == Json {
			if name != "number-float" &&
				name != "number-float-zero" &&
				name != "number-float-dot-start" &&
				name != "number-float-dot-stop" &&
				name != "number-float-space" &&
				name != "number-float-comma" &&
				name != "number-float-neg" {
				err = json.Unmarshal([]byte(expect[0].token), new(big.Float))
				assert.NoError(t, err, "invalid big decimal number: '%s'", input)
			}
			if name != "number-float-dot-start" &&
				name != "number-float-dot-stop" {
				err = json.Unmarshal([]byte(expect[0].token), new(float64))
				assert.NoError(t, err, "invalid decimal number: '%s'", input)
			}
		}
	case Infinity, NaN:
		if strings.HasPrefix(expect[0].token, "+") ||
			strings.HasPrefix(expect[0].token, "-") {
			_, err := strconv.ParseFloat(expect[0].token[1:], 64)
			assert.NoError(t, err, "invalid decimal number: '%s'", input)
			if param.json == Json {
				err = json.Unmarshal([]byte(expect[0].token[1:]), new(float64))
				assert.NoError(t, err, "invalid decimal number: '%s'", input)
			}
		} else {
			_, err := strconv.ParseFloat(expect[0].token, 64)
			assert.NoError(t, err, "invalid decimal number: '%s'", input)
			if param.json == Json {
				err = json.Unmarshal([]byte(expect[0].token), new(float64))
				assert.NoError(t, err, "invalid decimal number: '%s'", input)
			}
		}
	case Complex:
		token := expect[0].token
		if strings.HasSuffix(token, "+i") || strings.HasSuffix(token, "-i") {
			token = token[:len(token)-1] + "1i"
		}
		_, err := strconv.ParseComplex(token, 128)
		assert.NoError(t, err, "invalid complex number: '%s'", input)
		if param.json == Json {
			err = json.Unmarshal([]byte(token), new(complex128))
			assert.NoError(t, err, "invalid complex number: '%s'", input)
		}
	default:
		expect[0].typ = EOF
		expect[0].token = ""
	}

	// Could be at begin, but we want to test the json abilities.
	if len(fail) > 0 && slices.Contains(fail, param.json) {
		expect[0].typ = EOF
		expect[0].token = ""
	}

	return input, expect
}

func testScanNumber(
	t *testing.T, name string,
	call func(*Scanner, int, []byte) (byte, int, int),
	fail ...JsonType,
) {
	test.Map(t, scanTestCases).
		Prefix("method=" + name + "/test=").Filter(types(Number)).
		// Filter(test.Pattern[scanParams]("^number-dot-comma$")).
		Run(func(t test.Test, param scanParams) {
			input, expect := setupValidNumber(t, param, fail...)

			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(input), 1,
			), make([]byte, 0, 64))
			scanner.reader.skip()

			// When
			typ, offset, _ := call(scanner, 0, scanner.reader.window())

			// Then
			assert.Equal(t, string(expect[0].typ), string(typ), "'%s'", input)
			assert.Equal(t, expect[0].token,
				string(scanner.reader.window()[:offset]), "'%s'", input)
		})
}

// TestScanNumber tests the scanning of numbers using various number scanning
// functions, ensuring that they correctly identify valid numbers and reject
// invalid ones based on the provided test cases and JSON mode.
func TestScanNumber(t *testing.T) {
	testScanNumber(t, "number", (*Scanner).number)
	testScanNumber(t, "numberx", (*Scanner).numberx)
	testScanNumber(t, "numbery", (*Scanner).numbery)
	testScanNumber(t, "numberz", (*Scanner).numberz)
	testScanNumber(t, "numbers", (*Scanner).numbers, Json5Ext)
}

// benchmarkScanNumber benchmarks the scanning of numbers using the provided
// function. It iterates through the test cases, measuring the performance of
// the number scanning function for valid number inputs, while also considering
// any JSON mode restrictions specified in the test cases.
func benchmarkScanNumber(
	b *testing.B, name string,
	call func(*Scanner, int, []byte) (byte, int, int),
	fail ...JsonType,
) {
	var buffer [1 << 12]byte

	test.Map(test.Benchmark(b), scanTestCases).
		Prefix("method=" + name + "/test=").Filter(types(Number)).
		Filter(test.Pattern[scanParams]("test=number-")).
		Benchmark(func(b *testing.B, param scanParams) func(*testing.B) {
			// Setup
			input, _ := setupValidNumber(test.Benchmark(b), param, fail...)
			reader := strings.NewReader(input)
			scanner := Scanner{}

			b.SetBytes(reader.Size())

			// Loop
			return func(*testing.B) {
				test.Must(reader.Seek(0, 0))
				scanner = Scanner{
					reader: *NewReader(buffer[:0], reader),
				}
				scanner.reader.extend()
				window := scanner.reader.window()
				typ, offset, skip := call(&scanner, 0, window)

				runtime.KeepAlive(typ)
				runtime.KeepAlive(offset)
				runtime.KeepAlive(skip)
			}
		})
}

// BenchmarkScanNumber benchmarks the scanning of numbers using various number
// scanning functions, measuring their performance for valid number inputs while
// considering any JSON mode restrictions specified in the test cases.
func BenchmarkScanNumber(b *testing.B) {
	benchmarkScanNumber(b, "number", (*Scanner).number)
	benchmarkScanNumber(b, "numberx", (*Scanner).numberx)
	benchmarkScanNumber(b, "numbery", (*Scanner).numbery)
	benchmarkScanNumber(b, "numberz", (*Scanner).numberz)
	benchmarkScanNumber(b, "numbers", (*Scanner).numbers, Json5Ext)
}

//
// Scanning strings.
//

func setupValidString(
	t test.Test, param scanParams,
) (string, []token) {
	input, expect := param.input, slices.Clone(param.expect)
	// name := t.Name()[strings.LastIndex(t.Name(), "=")+1:]

	// if param.json == Json &&
	// 	name != "string-quote-double-raw" &&
	// 	name != "string-quote-double-escape" {
	if param.json == Json {
		if expect[0].typ == String && strings.HasPrefix(input, "\"") {
			// Input is a quoted JSON string — validate it directly.
			err := json.Unmarshal([]byte(input), new(string))
			assert.NoError(t, err, "invalid string: '%s'", input)
		} else {
			err := json.Unmarshal([]byte("\""+expect[0].token+"\""), new(string))
			assert.NoError(t, err, "invalid string: '%s'", input)
		}
	}

	switch expect[0].typ {
	case ArrayStart, ArrayEnd, ObjectStart, ObjectEnd, Colon, Comma:
		expect[0].typ = EOF
		expect[0].token = ""
	}

	return input, expect
}

func filterScanString(
	quoted func(*Scanner, byte) (byte, []byte, int),
	relaxed func(*Scanner) (byte, []byte, int),
) test.FilterFunc[scanParams] {
	if quoted != nil && relaxed == nil {
		return test.Pattern[scanParams]("test=string-quote-")
	} else if quoted == nil && relaxed != nil {
		return test.And(test.Pattern[scanParams]("test=string-"),
			test.Not(test.Pattern[scanParams]("test=string-quote-")))
	}
	return test.Not(test.Pattern[scanParams]("test=token-empty$"))
}

func wrapQuoted(
	quoted func(*Scanner, byte) (byte, []byte, int),
) func(*Scanner, byte) (byte, []byte, int) {
	return func(s *Scanner, b byte) (byte, []byte, int) {
		return quoted(s, b)
	}
}

func wrapQuoted_(
	quoted func(*Scanner, byte) (byte, []byte, int, int, int),
) func(*Scanner, byte) (byte, []byte, int) {
	return func(s *Scanner, b byte) (byte, []byte, int) {
		typ, token, offset, _, _ := quoted(s, b)
		return typ, token, offset
	}
}

func wrapRelaxed(
	relaxed func(*Scanner) (byte, []byte, int),
) func(*Scanner) (byte, []byte, int) {
	return func(s *Scanner) (byte, []byte, int) {
		return relaxed(s)
	}
}

func wrapRelaxed_(
	relaxed func(*Scanner) (byte, []byte, int, int, int),
) func(*Scanner) (byte, []byte, int) {
	return func(s *Scanner) (byte, []byte, int) {
		typ, token, offset, _, _ := relaxed(s)
		return typ, token, offset
	}
}

var (
	quoted   = wrapQuoted((*Scanner).quoted)
	quotedO  = wrapQuoted_((*Scanner).quoted_)
	relaxed  = wrapRelaxed((*Scanner).relaxed)
	relaxedO = wrapRelaxed_((*Scanner).relaxed_)
)

func testScanString(
	t *testing.T, name string,
	quoted func(*Scanner, byte) (byte, []byte, int),
	relaxed func(*Scanner) (byte, []byte, int),
) {
	test.Map(t, scanTestCases).
		Prefix("method=" + name + "/test=").Filter(types(Quoted)).
		Filter(filterScanString(quoted, relaxed)).
		Run(func(t test.Test, param scanParams) {
			input, expect := setupValidString(t, param)

			// Given
			scanner := NewScanner(
				strings.NewReader(input), make([]byte, 0, 64))
			scanner.reader.skip()
			window := scanner.reader.window()
			typ, buffer, offset := byte(0), []byte(nil), 0

			// When
			if window[0] == Quote || window[0] == String {
				if quoted != nil {
					typ, buffer, offset = quoted(scanner, window[0])
				}
			} else if relaxed != nil {
				typ, buffer, offset = relaxed(scanner)
			}

			// Then
			assert.Equal(t, string(expect[0].typ), string(typ),
				"string: %s", input)
			assert.Equal(t, expect[0].token, string(buffer),
				"string: %s", input)
			assert.Equal(t, len(expect[0].token), len(buffer),
				"string: %s", input)
			// TODO: find a way to test the offset.
			assert.Equal(t, offset, offset, "string: %s", input)
		})
}

func TestScanString(t *testing.T) {
	testScanString(t, "quoted", quoted, nil)
	testScanString(t, "quotedx", quotedx, nil)
	testScanString(t, "quotedo", quotedO, nil)
	testScanString(t, "quotedox", quotedOx, nil)
	testScanString(t, "relaxed", nil, relaxed)
	testScanString(t, "relaxedx", nil, relaxedx)
	testScanString(t, "relaxedo", nil, relaxedO)
	testScanString(t, "relaxedox", nil, relaxedOx)
}

func benchmarkScanString(
	b *testing.B, name string,
	quoted func(*Scanner, byte) (byte, []byte, int),
	relaxed func(*Scanner) (byte, []byte, int),
) {
	var buffer [1 << 12]byte

	test.Map(test.Benchmark(b), scanTestCases).
		Prefix("method=" + name + "/test=").Filter(types(Quoted)).
		Filter(filterScanString(quoted, relaxed)).
		// Filter(test.Pattern[scanParams]("test=string-text")).
		Benchmark(func(b *testing.B, param scanParams) func(b *testing.B) {
			// Setup
			input, _ := setupValidString(test.Benchmark(b), param)
			reader := strings.NewReader(input)
			scanner := Scanner{}

			b.SetBytes(reader.Size())

			// Loop
			return func(*testing.B) {
				test.Must(reader.Seek(0, 0))
				scanner = Scanner{
					reader: *NewReader(buffer[:0], reader),
				}
				scanner.reader.extend()
				window := scanner.reader.window()

				var offset int
				if window[0] == Quote || window[0] == String {
					if quoted != nil {
						_, _, offset = quoted(&scanner, window[0])
					}
				} else if relaxed != nil {
					_, _, offset = relaxed(&scanner)
				}

				runtime.KeepAlive(offset)
			}
		})
}

func BenchmarkScanString(b *testing.B) {
	benchmarkScanString(b, "quoted", quoted, nil)
	benchmarkScanString(b, "quotedx", quotedx, nil)
	benchmarkScanString(b, "quotedo", quotedO, nil)
	benchmarkScanString(b, "quotedox", quotedOx, nil)
	benchmarkScanString(b, "relaxed", nil, relaxed)
	benchmarkScanString(b, "relaxedx", nil, relaxedx)
	benchmarkScanString(b, "relaxedo", nil, relaxedO)
	benchmarkScanString(b, "relaxedox", nil, relaxedOx)
}

// scannerStringFailParams holds the test parameters for scanner fail paths.
type scannerStringFailParams struct {
	input   string
	quoted  func(*Scanner, byte) (byte, []byte, int)
	relaxed func(*Scanner) (byte, []byte, int)
}

var scannerStringFailTestCases = map[string]scannerStringFailParams{
	// Invalid unicode in a double-quoted string.
	"quoted-unicode-fail": {
		input: "\"\\u!!!!\"", quoted: (*Scanner).quoted,
	},
	// Invalid unicode in a quotedo string.
	"quotedo-unicode-fail": {
		input: "\"\\u!!!!\"", quoted: quotedO,
	},
	// EOF in the escape sub-loop of a relaxed string.
	"relaxed-eof-escape": {
		input: "hello\\", relaxed: (*Scanner).relaxed,
	},
	// EOF after first byte of UTF-8 whitespace in relaxed.
	"relaxed-eof-uspace": {
		input: "hello\xC2", relaxed: (*Scanner).relaxed,
	},
	// Invalid unicode in a relaxed string.
	"relaxed-unicode-fail": {
		input: "hello\\u!!!!,", relaxed: (*Scanner).relaxed,
	},
	// EOF in the escape sub-loop of a relaxedo string.
	"relaxedo-eof-escape": {
		input: "hello\\", relaxed: relaxedO,
	},
	// EOF after first byte of UTF-8 whitespace in relaxedo.
	"relaxedo-eof-uspace": {
		input: "hello\xC2", relaxed: relaxedO,
	},
	// Invalid unicode in a relaxedo string.
	"relaxedo-unicode-fail": {
		input: "hello\\u!!!!,", relaxed: relaxedO,
	},
}

// TestScanStringFail tests the error return paths in all string scanner
// functions that result in a zero offset and nil token (invalid input).
func TestScanStringFail(t *testing.T) {
	test.Map(t, scannerStringFailTestCases).
		Run(func(t test.Test, param scannerStringFailParams) {
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

//
// Scanning unicode escape sequences.
//

// unicodeParams defines the parameters for testing the scanning of unicode
// escape sequences in JSON strings.
type unicodeParams struct {
	input   string
	offset  int
	chunked bool
	erune   rune
	eoffset int
	ok      bool
}

var unicodeTestCases = map[string]unicodeParams{
	// EOF before 4 hex digits can be read.
	"too-short-eof": {input: "AB", ok: false},
	// Invalid hex digits (all 4 already in window).
	"invalid-hex": {input: "XXXX", ok: false},
	// Invalid hex digits arriving via window extension (chunked reader).
	"invalid-hex-extend": {input: "XXXX", chunked: true, ok: false},

	// BMP characters (no surrogate handling).
	"bmp-nul":   {input: "0000", erune: 0x0000, eoffset: 4, ok: true},
	"bmp-ascii": {input: "0041", erune: 'A', eoffset: 4, ok: true},
	"bmp-heart": {input: "2764", erune: 0x2764, eoffset: 4, ok: true},
	"bmp-max":   {input: "D7FF", erune: 0xD7FF, eoffset: 4, ok: true},

	// Low surrogate alone: not in D800-DBFF, treated as plain rune.
	"low-surrogate-alone": {
		input: "DC00", erune: 0xDC00, eoffset: 4, ok: true,
	},

	// High surrogate with EOF before any pair data: returned unpaired.
	"surrogate-high-eof": {
		input: "D800", erune: 0xD800, eoffset: 4, ok: true,
	},
	// High surrogate followed by non-backslash: no pair detected.
	"surrogate-high-no-escape": {
		input: "D800ABCD", erune: 0xD800, eoffset: 4, ok: true,
	},
	// High surrogate: backslash present but not 'u'.
	"surrogate-high-not-u": {
		input: "D800\\xDC00", erune: 0xD800, eoffset: 4, ok: true,
	},
	// High surrogate: \u present but followed by invalid hex.
	"surrogate-high-invalid-hex": {
		input: "D800\\uXXXX", erune: 0xD800, eoffset: 4, ok: true,
	},
	// High surrogate: \u followed by another high surrogate (not DC00-DFFF).
	"surrogate-high-not-low": {
		input: "D800\\uD800", erune: 0xD800, eoffset: 4, ok: true,
	},

	// Valid surrogate pairs.
	"surrogate-pair-min": {
		input: "D800\\uDC00", erune: 0x10000, eoffset: 10, ok: true,
	},
	"surrogate-pair-max": {
		input: "DBFF\\uDFFF", erune: 0x10FFFF, eoffset: 10, ok: true,
	},
	"surrogate-pair-emoji": {
		input: "D83D\\uDE0A", erune: 0x1F60A, eoffset: 10, ok: true,
	},

	// Extend path: 4 hex digits split across window boundary.
	"extend-bmp": {
		input: "2764", chunked: true, erune: 0x2764, eoffset: 4, ok: true,
	},
	// Extend path: surrogate pair split across window boundary.
	"extend-surrogate-pair": {
		input:   "D800\\uDC00",
		chunked: true, erune: 0x10000, eoffset: 10, ok: true,
	},
}

func testScanUnicode(
	t *testing.T, name string,
	call func(*Scanner, int, []byte) (rune, int, []byte, bool),
) {
	test.Map(t, unicodeTestCases).
		Prefix("method=" + name + "/test=").
		Run(func(t test.Test, param unicodeParams) {
			// Given
			scanner := NewScanner(NewTestReader(
				strings.NewReader(param.input), 1),
				make([]byte, 0, 64))
			scanner.reader.extend()
			window := scanner.reader.window()

			// When
			rune, offset, _, ok := call(scanner, param.offset, window)

			// Then
			assert.Equal(t, param.ok, ok)
			if param.ok {
				assert.Equal(t, param.erune, rune)
				assert.Equal(t, param.eoffset, offset)
			}
		})
}

func TestScanUnicode(t *testing.T) {
	testScanUnicode(t, "unicode", (*Scanner).unicode)
	testScanUnicode(t, "unicodex", (*Scanner).unicodex)
}

func benchmarkScanUnicode(
	b *testing.B, name string,
	call func(*Scanner, int, []byte) (rune, int, []byte, bool),
) {
	test.Map(test.Benchmark(b), unicodeTestCases).
		Prefix("method=" + name + "/test=").
		// Skip failed, chunked, and high-surrogate cases where the inner
		// extend loop would fire (input too short to satisfy offset+6).
		Filter(func(_ string, p unicodeParams) bool {
			return p.ok && !p.chunked &&
				(p.erune < 0xD800 || p.erune > 0xDBFF ||
					len(p.input) >= 10)
		}).
		Benchmark(func(b *testing.B, param unicodeParams) func(*testing.B) {
			// Setup
			scanner := NewScanner(
				strings.NewReader(param.input),
				make([]byte, 0, 64),
			)
			scanner.reader.extend()
			window := scanner.reader.window()

			b.SetBytes(int64(len(param.input)))

			// Loop
			return func(*testing.B) {
				got, offset, _, ok := call(scanner, param.offset, window)

				runtime.KeepAlive(got)
				runtime.KeepAlive(offset)
				runtime.KeepAlive(ok)
			}
		})
}

func BenchmarkScanUnicode(b *testing.B) {
	benchmarkScanUnicode(b, "unicode", (*Scanner).unicode)
	// benchmarkScanUnicode(b, "unicodex", (*Scanner).unicodex)
}
