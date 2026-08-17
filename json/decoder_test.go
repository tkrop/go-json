package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkrop/go-testing/test"
)

// FIXME: Improve AI generated test cases.

// benchReader is an interface that combines io.ReadSeeker and a Size method
// for benchmarking.
type benchReader interface {
	io.ReadSeeker
	Size() int64
}

// mapIntParams holds parameters for map[int] decoding benchmarks.
type mapIntParams struct {
	input string
}

// setupStringReader creates a benchReader from a string input.
func setupStringReader(_ test.Reporter, param mapIntParams) *strings.Reader {
	return strings.NewReader(param.input)
}

// setupFileReader creates a benchReader from a file path specified in the test
// parameters. It uses a helper function getFixtureReader to read the file
// contents into a bytes.Reader.
func setupFileReader(t test.Reporter, param fileParams) *bytes.Reader {
	return getFixtureReader(t, param.path)
}

// checkFileTokens is a helper function to verify the number of tokens read in
// the benchmarks. It compares the expected token count from the test parameters
// with the actual token count and fails the benchmark if they do not match.
func checkFileTokens(b *testing.B, param fileParams, tokens int) {
	if tokens != param.chars {
		b.Fatalf("expected %v tokens, got %v", param.chars, tokens)
	}
}

// decNewParams holds parameters for TestDecNew.
type decNewParams struct {
	bufSize int
	expect  int
}

// decNewTestCases defines test cases for testing the NewDecoder and
// NewDecoderBuffer functions, including default buffer size, custom buffer
// size, and minimum buffer size.
var decNewTestCases = map[string]decNewParams{
	"default": {expect: 8192},
	"custom":  {bufSize: 256, expect: 256},
	"min":     {bufSize: 1, expect: 1},
}

// TestDecNew tests the NewDecoder and NewDecoderBuffer functions to ensure
// that the decoder is initialized with the correct buffer size and state. It
// verifies that the default buffer size is used when no custom buffer is
// provided, and that a custom buffer size is respected.
func TestDecNew(t *testing.T) {
	test.Map(t, decNewTestCases).
		Run(func(t test.Test, p decNewParams) {
			// When
			var dec *Decoder
			if p.bufSize == 0 {
				dec = NewDecoder(strings.NewReader(""))
			} else {
				dec = NewDecoderBuffer(
					strings.NewReader(""), make([]byte, p.bufSize))
			}

			// Then
			require.NotNil(t, dec)
			assert.Equal(t, p.expect, cap(dec.scanner.reader.buffer))
			assert.NotNil(t, dec.state)
		})
}

// decDecodeParams holds parameters for TestDecDecode.
type decDecodeParams struct {
	mode   Mode
	input  string
	output any
	expect any
	error  error
	check  func(t test.Test, output any)
}

func decError(
	msg string, pos Position, typ byte, token string,
	t reflect.Type, err error,
) error {
	return &DecodeError{
		Msg:   msg,
		Pos:   pos,
		Typ:   typ,
		Token: token,
		Type:  t,
		Err:   err,
	}
}

type decErrorStringParams struct {
	err    *DecodeError
	expect string
}

type decStructInternalObject struct {
	Name string `json:"name"`
}

var decErrorStringTestCases = map[string]decErrorStringParams{
	"byte-only-position": {
		err: &DecodeError{
			Msg:   "number",
			Pos:   Position{Byte: 7},
			Typ:   Integer,
			Token: "12",
			Type:  reflect.TypeOf(int(0)),
		},
		expect: `decode number [type=int, byte=7, token="12"]`,
	},
	"line-and-char-position": {
		err: &DecodeError{
			Msg:   "string",
			Pos:   Position{Byte: 9, Line: 2, Char: 4},
			Typ:   String,
			Token: "x",
			Type:  reflect.TypeOf(""),
		},
		expect: `decode string [type=string, byte=9, line=2, char=4, token="x"]`,
	},
}

// decDecodeTestCases defines test cases for testing the Decode method of the
// Decoder.
var decDecodeTestCases = map[string]decDecodeParams{
	// Bool targets.
	"bool-true": {
		mode: StrictJson, input: `true`, output: new(bool),
		expect: test.Ptr(true),
	},
	"bool-false": {
		mode: StrictJson, input: `false`, output: new(bool),
		expect: test.Ptr(false),
	},
	"bool-any-true": {
		mode: StrictJson, input: `true`, output: test.Ptr[any](false),
		expect: test.Ptr[any](true),
	},
	"bool-any-false": {
		mode: StrictJson, input: `false`, output: test.Ptr[any](true),
		expect: test.Ptr[any](false),
	},

	// Null into pointer/map/slice.
	"null-ptr": {
		mode: StrictJson, input: `null`, output: test.Ptr(new(int)),
		expect: test.Ptr((*int)(nil)),
	},
	"null-map": {
		mode: StrictJson, input: `null`, output: test.Ptr(map[int]string{}),
		expect: test.Ptr((map[int]string)(nil)),
	},
	"null-slice": {
		mode:  StrictJson,
		input: `null`, output: test.Ptr([]string{"a"}),
		expect: test.Ptr(([]string)(nil)),
	},

	// Numeric targets.
	"float64-any": {
		mode: StrictJson, input: `3`, output: new(any),
		expect: test.Ptr[any](float64(3)),
	},
	"float64": {
		mode: StrictJson, input: `1`, output: new(float64),
		expect: test.Ptr(float64(1)),
	},
	"float32": {
		mode: StrictJson, input: `1`, output: new(float32),
		expect: test.Ptr(float32(1)),
	},
	"int": {
		mode: StrictJson, input: `1`, output: new(int),
		expect: test.Ptr(1),
	},
	"int64-neg": {
		mode: StrictJson, input: `-1`, output: new(int64),
		expect: test.Ptr(int64(-1)),
	},
	"uint": {
		mode: StrictJson, input: `1`, output: new(uint),
		expect: test.Ptr(uint(1)),
	},

	// Hex numeric target.
	"hex-any": {
		mode: Strict, input: `0xFF`, output: new(any),
		expect: test.Ptr[any](float64(255)),
	},
	"hex-int": {
		mode: Strict, input: `0x10`, output: new(int),
		expect: test.Ptr(16),
	},
	"hex-uint": {
		mode: Strict, input: `0x10`, output: new(uint),
		expect: test.Ptr(uint(16)),
	},
	"hex-float64": {
		mode: Strict, input: `0x10`, output: new(float64),
		expect: test.Ptr(float64(16)),
	},

	// Complex target.
	"complex-any": {
		mode: Extended, input: `1+2i`, output: new(any),
		expect: test.Ptr[any](complex(1, 2)),
	},
	"complex128": {
		mode: Extended, input: `3+4i`, output: new(complex128),
		expect: test.Ptr(complex128(3 + 4i)),
	},
	"complex64": {
		mode: Extended, input: `1+2i`, output: new(complex64),
		expect: test.Ptr(complex64(1 + 2i)),
	},

	// Infinity target.
	"infinity-pos": {
		mode: Strict, input: `Infinity`, output: new(float64),
		expect: test.Ptr(math.Inf(1)),
	},
	"infinity-neg": {
		mode: Strict, input: `-Infinity`, output: new(float64),
		expect: test.Ptr(math.Inf(-1)),
	},
	"infinity-any": {
		mode: Strict, input: `Infinity`, output: new(any),
		expect: test.Ptr[any](math.Inf(1)),
	},

	// String targets.
	"string": {
		mode: StrictJson, input: `"hello"`, output: new(string),
		expect: test.Ptr("hello"),
	},
	"string-any": {
		mode: StrictJson, input: `"world"`, output: new(any),
		expect: test.Ptr[any]("world"),
	},

	// Object/map targets.
	"object-empty-any": {
		mode: StrictJson, input: `{}`, output: new(any),
		expect: test.Ptr[any](map[string]any{}),
	},
	"object-nested-any": {
		mode: StrictJson, input: `{"a": 1, "b": {"c": 2}}`, output: new(any),
		expect: test.Ptr[any](map[string]any{
			"a": big.NewInt(1),
			"b": map[string]any{"c": big.NewInt(2)},
		}),
	},
	"object-map-string": {
		mode:   StrictJson,
		input:  `{"hello": "world"}`,
		output: &map[string]string{},
		expect: &map[string]string{"hello": "world"},
	},
	"object-map-any": {
		mode:   StrictJson,
		input:  `{"a": 1, "b": false, "c":[1, 2.0, "three"]}`,
		output: &map[string]any{},
		expect: &map[string]any{
			"a": float64(1), "b": false, "c": []any{
				big.NewInt(1), test.Okay((&big.Float{}).SetString("2.0")), "three",
			},
		},
	},

	// Array/slice targets.
	"array-string-strict-json": {
		mode:   StrictJson,
		input:  `["a","b"]`,
		output: &[]string{},
		expect: &[]string{"a", "b"},
	},
	"array-string-relaxed-identifiers": {
		mode:   Relaxed,
		input:  `[a,b]`,
		output: &[]string{},
		expect: &[]string{"a", "b"},
	},
	"array-any-relaxed-identifiers": {
		mode:   Relaxed,
		input:  `[a,b]`,
		output: &[]any{},
		expect: &[]any{"a", "b"},
	},
	"array-nested-any": {
		mode: StrictJson, input: `[{"a": [{}]}]`, output: new(any),
		expect: test.Ptr[any]([]any{
			map[string]any{"a": []any{map[string]any{}}},
		}),
	},

	// Error cases.
	"error-invalid-interface": {
		mode: StrictJson, input: `1`, output: nil,
		error: decError("invalid", Position{}, 0, "", nil, nil),
	},
	"error-non-pointer": {
		mode: StrictJson, input: `1`, output: 42,
		error: decError("no-pointer", Position{}, 0, "",
			reflect.TypeOf(42), nil),
	},
	"error-nil-pointer": {
		mode: StrictJson, input: `1`, output: (*int)(nil),
		error: decError("nil", Position{}, 0, "",
			reflect.TypeOf((*int)(nil)), nil),
	},
	"error-object-unhandled-kind": {
		mode: StrictJson, input: `{}`, output: new(int),
		error: decError("object type", Position{Byte: 1},
			ObjectStart, "{", reflect.TypeOf(int(0)), nil),
	},
	"error-array-unhandled-kind": {
		mode: StrictJson, input: `[]`, output: new(int),
		error: decError("array type", Position{Byte: 1},
			ArrayStart, "[", reflect.TypeOf(int(0)), nil),
	},
	"error-bool-unhandled-kind": {
		mode: StrictJson, input: `true`, output: new(int),
		error: decError("bool type", Position{Byte: 4},
			True, "true", reflect.TypeOf(int(0)), nil),
	},
	"error-null-unhandled-kind": {
		mode: StrictJson, input: `null`, output: new(int),
		error: decError("null type", Position{Byte: 4},
			Null, "null", reflect.TypeOf(int(0)), nil),
	},
	"error-string-unhandled-kind": {
		mode: StrictJson, input: `"x"`, output: new(int),
		error: decError("string type", Position{Byte: 3},
			String, "x", reflect.TypeOf(int(0)), nil),
	},
	"error-int-overflow": {
		mode: StrictJson, input: `300`, output: new(int8),
		error: decError("number int", Position{Byte: 3},
			Integer, "300", reflect.TypeOf(int8(0)), nil),
	},
	"error-uint-overflow": {
		mode: StrictJson, input: `300`, output: new(uint8),
		error: decError("number uint", Position{Byte: 3},
			Integer, "300", reflect.TypeOf(uint8(0)), nil),
	},
	"error-non-string-map-key": {
		mode: StrictJson, input: `{"a":1}`, output: &map[int]int{},
		error: decError("map key", Position{Byte: 1},
			ObjectStart, "", reflect.TypeOf(map[int]int{}), nil),
	},
	"error-hex-unhandled-kind": {
		mode: Strict, input: `0xFF`, output: new(bool),
		error: decError("hexa-decimal number type", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf(false), nil),
	},
	"error-complex-unhandled-kind": {
		mode: Extended, input: `1+2i`, output: new(float64),
		error: decError("complex number type", Position{Byte: 4},
			Complex, "1+2i", reflect.TypeOf(float64(0)), nil,
		),
	},
	"error-infinity-unhandled-kind": {
		mode: Strict, input: `Infinity`, output: new(int),
		error: decError("infinity type", Position{Byte: 8},
			Infinity, "Infinity", reflect.TypeOf(int(0)), nil),
	},
	"error-nan-unhandled-kind": {
		mode: Strict, input: `NaN`, output: new(int),
		error: decError("nan type", Position{Byte: 3}, NaN, "NaN",
			reflect.TypeOf(int(0)), nil),
	},

	// NaN targets (require special IsNaN check).
	"nan-float64": {
		mode: Strict, input: `NaN`, output: new(float64),
		check: func(t test.Test, out any) {
			assert.True(t, math.IsNaN(*test.Cast[*float64](out)))
		},
	},
	"nan-float32": {
		mode: Strict, input: `NaN`, output: new(float32),
		check: func(t test.Test, out any) {
			assert.True(t, math.IsNaN(float64(*test.Cast[*float32](out))))
		},
	},
	"nan-any": {
		mode: Strict, input: `NaN`, output: new(any),
		check: func(t test.Test, out any) {
			f, ok := (*out.(*any)).(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},

	// Decode into any with comprehensive value types (decodeValueAny coverage).
	"object-all-values-any": {
		mode:   StrictJson,
		input:  `{"a":true,"b":false,"c":null,"d":"str","e":[1]}`,
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"a": true, "b": false, "c": nil, "d": "str",
			"e": []any{big.NewInt(1)},
		}),
	},
	"object-decimal-any": {
		mode:   StrictJson,
		input:  `{"n":1.5}`,
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"n": test.Okay((&big.Float{}).SetString("1.5")),
		}),
	},
	"object-extended-any": {
		mode:   Extended,
		input:  `{"h":0xFF,"c":1+2i,"i":Infinity,"ni":-Infinity}`,
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"h": big.NewInt(255), "c": complex(1, 2),
			"i": math.Inf(1), "ni": math.Inf(-1),
		}),
	},

	// Decode into any with array of all value types (decodeSliceAny coverage).
	"array-all-values-any": {
		mode:   StrictJson,
		input:  `[true, false, null, "str", 1, 1.5, {"a":1}, [2]]`,
		output: new(any),
		expect: func() any {
			num := &big.Float{}
			num.SetString("1.5")

			return test.Ptr[any]([]any{
				true, false, nil, "str", big.NewInt(1), num,
				map[string]any{"a": big.NewInt(1)},
				[]any{big.NewInt(2)},
			})
		}(),
	}, // Decode into any with extended numeric slice (decodeSliceAny HexaDecimal/Complex/Infinity).
	"array-extended-any": {
		mode:   Extended,
		input:  `[0xFF, 1+2i, Infinity, -Infinity]`,
		output: new(any),
		expect: test.Ptr[any]([]any{
			big.NewInt(255), complex(1, 2), math.Inf(1), math.Inf(-1),
		}),
	},
	// Decode NaN into slice (requires IsNaN check).
	"nan-slice-any": {
		mode: Strict, input: `[NaN]`, output: new(any),
		check: func(t test.Test, out any) {
			s, ok := (*out.(*any)).([]any)
			require.True(t, ok)
			require.Len(t, s, 1)
			f, ok := s[0].(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},
	// Decode NaN into map value (requires IsNaN check, covers decodeValueAny NaN case).
	"nan-map-any": {
		mode: Strict, input: `{"n":NaN}`, output: new(any),
		check: func(t test.Test, out any) {
			m, ok := (*out.(*any)).(map[string]any)
			require.True(t, ok)
			f, ok := m["n"].(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},
	// Error: unhandled token type in decodeValueAny default case.
	"error-decode-valueany-default": {
		mode: StrictJson, input: `{"a"::}`, output: new(any),
		error: decError("unknown", Position{Byte: 6}, Colon, ":", nil, nil),
	},
	// Error: initial NextToken failure in decodeValue.
	"error-decode-empty": {
		mode: StrictJson, input: ``, output: new(any),
		error: io.ErrUnexpectedEOF,
	},
	// Error: unhandled token type in decodeValue default case.
	"error-decode-colon": {
		mode: StrictJson, input: `:`, output: new(any),
		error: decError("unknown", Position{Byte: 1},
			Colon, ":", reflect.TypeOf((*any)(nil)).Elem(), nil),
	},
	// Error: decodeMap propagates decodeValue error.
	"error-decode-map-value": {
		mode: StrictJson, input: `{"a":{}}`, output: &map[string]int{},
		error: decError("object type", Position{Byte: 6},
			ObjectStart, "{", reflect.TypeOf(int(0)), nil),
	},
	// Error: decodeMapAny key scan truncated.
	"error-decode-obj-any-truncated-key": {
		mode: StrictJson, input: `{`, output: new(any),
		error: io.ErrUnexpectedEOF,
	},
	// Error: decodeMapAny value decode truncated (also covers decodeValueAny err).
	"error-decode-obj-any-truncated-value": {
		mode: StrictJson, input: `{"a":`, output: new(any),
		error: io.ErrUnexpectedEOF,
	},
	// Error: decodeSliceAny token scan truncated.
	"error-decode-slice-truncated": {
		mode: StrictJson, input: `[`, output: new(any),
		error: io.ErrUnexpectedEOF,
	},
	// Error: decodeSliceAny ObjectStart inner failure.
	"error-decode-slice-obj-truncated": {
		mode: StrictJson, input: `[{`, output: new(any),
		error: io.ErrUnexpectedEOF,
	},
	// Error: decodeSliceAny ArrayStart inner failure.
	"error-decode-slice-arr-truncated": {
		mode: StrictJson, input: `[[`, output: new(any),
		error: io.ErrUnexpectedEOF,
	},

	// Error: interface with methods rejects all value types.
	"error-object-interface-methods": {
		mode: StrictJson, input: `{}`, output: new(error),
		error: decError("object", Position{Byte: 1},
			ObjectStart, "{", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-array-interface-methods": {
		mode: StrictJson, input: `[]`, output: new(error),
		error: decError("array", Position{Byte: 1},
			ArrayStart, "[", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-bool-interface-methods": {
		mode: StrictJson, input: `true`, output: new(error),
		error: decError("bool", Position{Byte: 4},
			True, "true", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-string-interface-methods": {
		mode: StrictJson, input: `"x"`, output: new(error),
		error: decError("string", Position{Byte: 3},
			String, "x", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-int-interface-methods": {
		mode: StrictJson, input: `1`, output: new(error),
		error: decError("number", Position{Byte: 1},
			Integer, "1", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-hex-interface-methods": {
		mode: Strict, input: `0xFF`, output: new(error),
		error: decError("hexa-decimal number", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-complex-interface-methods": {
		mode: Extended, input: `1+2i`, output: new(error),
		error: decError("complex number", Position{Byte: 4},
			Complex, "1+2i", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-infinity-interface-methods": {
		mode: Strict, input: `Infinity`, output: new(error),
		error: decError("infinity", Position{Byte: 8},
			Infinity, "Infinity", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-nan-interface-methods": {
		mode: Strict, input: `NaN`, output: new(error),
		error: decError("nan", Position{Byte: 3},
			NaN, "NaN", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},

	// Error: unhandled target kind for numeric token.
	"error-int-target-bool": {
		mode: StrictJson, input: `1`, output: new(bool),
		error: decError("number type", Position{Byte: 1},
			Integer, "1", reflect.TypeOf(false), nil),
	},
	// Error: float32 parse overflow.
	"error-float32-overflow": {
		mode: StrictJson, input: `4e38`, output: new(float32),
		error: decError("number float", Position{Byte: 4},
			Decimal, "4e38", reflect.TypeOf(float32(0)), &strconv.NumError{
				Func: "ParseFloat",
				Num:  "4e38",
				Err:  strconv.ErrRange,
			}),
	},
	// Error: hex int8 overflow.
	"error-hex-int-overflow": {
		mode: Strict, input: `0xFF`, output: new(int8),
		error: decError("hexa-decimal number int overflow", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf(int8(0)), nil),
	},
	// Error: negative hex value decoded into uint.
	"error-hex-uint-negative": {
		mode: Strict, input: `-0x1`, output: new(uint8),
		error: decError("hexa-decimal number uint overflow", Position{Byte: 4},
			HexaDecimal, "-0x1", reflect.TypeOf(uint8(0)), nil),
	},
	// Error: decodeMap NextToken failure (truncated key).
	"error-decode-map-key-truncated": {
		mode: StrictJson, input: `{`,
		output: func() any { m := map[string]string{}; return &m }(),
		error:  io.ErrUnexpectedEOF,
	},
	"error-decode-any-complex-parse": {
		mode:   Extended,
		input:  `{"value":1e9999+1e9999i}`,
		output: new(any),
		error: decError("complex number", Position{Byte: 23}, Complex,
			"1e9999+1e9999i",
			nil, &strconv.NumError{
				Func: "ParseComplex",
				Num:  "1e9999+1e9999i",
				Err:  strconv.ErrRange,
			}),
	},
	"error-decode-slice-any-complex-parse": {
		mode:   Extended,
		input:  `[1e9999+1e9999i]`,
		output: new(any),
		error: decError("complex number", Position{Byte: 15}, Complex,
			"1e9999+1e9999i",
			nil, &strconv.NumError{
				Func: "ParseComplex",
				Num:  "1e9999+1e9999i",
				Err:  strconv.ErrRange,
			}),
	},
	"error-decode-struct-next": {
		mode:   StrictJson,
		input:  `{`,
		output: &decStructInternalObject{},
		error:  io.ErrUnexpectedEOF,
	},
	"error-decode-struct-skip-unknown-value": {
		mode:   StrictJson,
		input:  `{"missing":,}`,
		output: &decStructInternalObject{},
		error: decError("unknown", Position{Byte: 12}, Comma, ",",
			reflect.TypeOf((*any)(nil)).Elem(), nil),
	},
	"error-decode-struct-field-value": {
		mode:   StrictJson,
		input:  `{"name":,}`,
		output: &decStructInternalObject{},
		error: decError("unknown", Position{Byte: 9}, Comma, ",",
			reflect.TypeOf(""), nil),
	},
}

// TestDecDecode tests the Decode method with various input JSON and target
// types, including error cases and special values like NaN and Infinity. It
// verifies that the decoded output matches the expected value or error for
// each test case.
func TestDecDecode(t *testing.T) {
	test.Map(t, decDecodeTestCases).
		Run(func(t test.Test, param decDecodeParams) {
			// Given
			decoder := NewDecoder(strings.NewReader(param.input)).
				Mode(param.mode)

			// When
			err := decoder.Decode(param.output)

			// Then
			if param.error != nil {
				assert.Equal(t, param.error, err)
			} else if param.check != nil {
				require.NoError(t, err)
				param.check(t, param.output)
			} else {
				require.NoError(t, err)
				assert.Equal(t, param.expect, param.output)
			}
		})
}

type decDecodeStructObject struct {
	Name string `json:"name"`
	Type string
}

type decDecodeStructTaggedObject struct {
	//revive:disable-next-line:struct-tag
	Name string `json:"name,case:ignore"`
	Type string
}

type decDecodeStructCaseStrict struct {
	X bool `json:"firstName"`
}

type decDecodeStructCaseTagIgnore struct {
	//revive:disable-next-line:struct-tag
	X bool `json:"firstName,case:ignore"`
}

// decDecodeStructParams holds parameters for TestDecDecodeStructMode.
type decDecodeStructParams struct {
	mode   Mode
	input  string
	output any
	expect any
}

// decDecodeStructTestCases defines test cases for testing struct field
// decoding.
var decDecodeStructTestCases = map[string]decDecodeStructParams{
	"case-sensitive": {
		mode:   StrictJson,
		input:  `{"NAME":"alpha","Type":"primary"}`,
		output: &decDecodeStructObject{},
		expect: &decDecodeStructObject{Type: "primary"},
	},
	"case-ignore": {
		mode:   StrictJson | CaseIgnore,
		input:  `{"NAME":"alpha","Type":"primary"}`,
		output: &decDecodeStructObject{},
		expect: &decDecodeStructObject{Name: "alpha", Type: "primary"},
	},
	"field-tag-case-ignore": {
		mode:   StrictJson,
		input:  `{"NAME":"alpha","Type":"primary"}`,
		output: &decDecodeStructTaggedObject{},
		expect: &decDecodeStructTaggedObject{
			Name: "alpha", Type: "primary",
		},
	},
	"case-strict-firstname": {
		mode:   StrictJson,
		input:  `{"firstname":true}`,
		output: &decDecodeStructCaseStrict{},
		expect: &decDecodeStructCaseStrict{},
	},
	"case-strict-firstname-exact": {
		mode:   StrictJson,
		input:  `{"firstName":true}`,
		output: &decDecodeStructCaseStrict{},
		expect: &decDecodeStructCaseStrict{X: true},
	},
	"case-ignore-firstname": {
		mode:   StrictJson | CaseIgnore,
		input:  `{"firstname":true}`,
		output: &decDecodeStructCaseStrict{},
		expect: &decDecodeStructCaseStrict{X: true},
	},
	"case-ignore-first-name": {
		mode:   StrictJson | CaseIgnore,
		input:  `{"first-name":true}`,
		output: &decDecodeStructCaseStrict{},
		expect: &decDecodeStructCaseStrict{X: true},
	},
	"case-ignore-first-name-upper": {
		mode:   StrictJson | CaseIgnore,
		input:  `{"FIRST_NAME":true}`,
		output: &decDecodeStructCaseStrict{},
		expect: &decDecodeStructCaseStrict{X: true},
	},
	"tag-case-ignore-first-name": {
		mode:   StrictJson,
		input:  `{"first_name":true}`,
		output: &decDecodeStructCaseTagIgnore{},
		expect: &decDecodeStructCaseTagIgnore{X: true},
	},
}

// TestDecDecodeStructMode tests decoding JSON into struct values with
// different mode settings.
func TestDecDecodeStructMode(t *testing.T) {
	test.Map(t, decDecodeStructTestCases).
		Run(func(t test.Test, param decDecodeStructParams) {
			// Given
			decoder := NewDecoder(strings.NewReader(param.input)).
				Mode(param.mode)

			// When
			err := decoder.Decode(param.output)

			// Then
			require.NoError(t, err)
			assert.Equal(t, param.expect, param.output)
		})
}

func TestDecodeErrorError(t *testing.T) {
	test.Map(t, decErrorStringTestCases).
		Run(func(t test.Test, param decErrorStringParams) {
			assert.Equal(t, param.expect, param.err.Error())
		})
}

//
// Benchmarks
//

// mapIntTestCases defines test cases for testing map[string]int decoding
// benchmarks.
var mapIntTestCases = map[string]mapIntParams{
	"flat-int-map": {
		input: `{"a":97,"b":98,"c":99,"d":100,"e":101,"f":102,"g":103}`,
	},
}

// benchmarkDecoder is a helper function to benchmark decoding performance for
// different decoder implementations. It takes a testing.B, a method name for
// labeling, a set of test cases, and setup, call, and check functions to
// customize the benchmark logic.
func benchmarkDecoder[T any, U any, R benchReader](
	b *testing.B, method string,
	cases map[string]T,
	setup func(test.Reporter, T) R,
	call func(R, []byte) (U, error),
	check func(*testing.B, T, U),
) {
	test.Map(test.Benchmark(b), cases).
		Prefix("method=" + method + "/test=").
		Benchmark(func(
			b *testing.B, param T,
		) func(*testing.B) {
			var buffer [8 << 10]byte
			reader := setup(b, param)

			b.ReportAllocs()
			b.SetBytes(reader.Size())

			return func(b *testing.B) {
				test.Must(reader.Seek(0, 0))

				result, err := call(reader, buffer[:])
				assert.NoError(b, err)

				if check != nil {
					check(b, param, result)
				}
			}
		})
}

// BenchmarkDecDecodeInterfaceAny benchmarks decoding JSON into an any target
// using different decoder implementations. This is a common use case for
// dynamic JSON decoding and measures the performance of decoding into an any
// type.
func BenchmarkDecDecodeInterfaceAny(b *testing.B) {
	var value any

	benchmarkDecoder(b, "gojson", fileTestCase, setupFileReader,
		func(
			reader *bytes.Reader, buffer []byte,
		) (*struct{}, error) {
			dec := NewDecoderBuffer(reader, buffer)

			return nil, dec.Decode(&value)
		}, nil,
	)

	benchmarkDecoder(b, "encjson", fileTestCase, setupFileReader,
		func(
			reader *bytes.Reader, _ []byte,
		) (*struct{}, error) {
			dec := json.NewDecoder(reader)

			return nil, dec.Decode(&value)
		}, nil,
	)
}

// BenchmarkDecDecodeMapInt benchmarks decoding JSON into a map[string]int
// target using different decoder implementations. This is a common use case
// and measures the performance of decoding into a map with integer values.
func BenchmarkDecDecodeMapInt(b *testing.B) {
	value := make(map[string]int)

	benchmarkDecoder(b, "gojson", mapIntTestCases, setupStringReader,
		func(
			reader *strings.Reader, buffer []byte,
		) (*struct{}, error) {
			dec := NewDecoderBuffer(reader, buffer)

			return nil, dec.Decode(&value)
		}, nil,
	)

	benchmarkDecoder(b, "encjson", mapIntTestCases, setupStringReader,
		func(
			reader *strings.Reader, _ []byte,
		) (*struct{}, error) {
			dec := json.NewDecoder(reader)

			return nil, dec.Decode(&value)
		}, nil,
	)
}

// BenchmarkDecToken benchmarks the Token method, which returns Go values for
// JSON tokens. This measures the full decoding performance, including
// scanning, tokenization, and value conversion.
func BenchmarkDecToken(b *testing.B) {
	var tokens int

	benchmarkDecoder(b, "gojson", fileTestCase, setupFileReader,
		func(reader *bytes.Reader, buffer []byte) (int, error) {
			dec := NewDecoderBuffer(reader, buffer)
			tokens = 0

			for {
				_, err := dec.Token()
				if errors.Is(err, io.EOF) {
					return tokens, nil
				}
				if err != nil {
					return 0, err
				}
				tokens++
			}
		}, checkFileTokens,
	)

	benchmarkDecoder(b, "encjson", fileTestCase, setupFileReader,
		func(reader *bytes.Reader, _ []byte) (int, error) {
			dec := json.NewDecoder(reader)
			tokens = 0

			for {
				_, err := dec.Token()
				if errors.Is(err, io.EOF) {
					return tokens, nil
				}
				if err != nil {
					return 0, err
				}
				tokens++
			}
		}, checkFileTokens,
	)
}

// BenchmarkDecTokenNext benchmarks the NextToken method, which returns token
// types and raw byte tokens without converting to Go values. This isolates the
// scanning and tokenization performance from the value conversion logic in
// Token.
func BenchmarkDecTokenNext(b *testing.B) {
	var tokens int

	benchmarkDecoder(b, "gojson", fileTestCase, setupFileReader,
		func(reader *bytes.Reader, buffer []byte) (int, error) {
			dec := NewDecoderBuffer(reader, buffer)
			tokens = 0

			for {
				_, _, err := dec.Next()
				if errors.Is(err, io.EOF) {
					return tokens, nil
				}
				if err != nil {
					return 0, err
				}
				tokens++
			}
		}, checkFileTokens,
	)

	benchmarkDecoder(b, "encjson", fileTestCase, setupFileReader,
		func(reader *bytes.Reader, _ []byte) (int, error) {
			dec := json.NewDecoder(reader)
			tokens = 0

			for {
				_, err := dec.Token()
				if errors.Is(err, io.EOF) {
					return tokens, nil
				}
				if err != nil {
					return 0, err
				}
				tokens++
			}
		}, checkFileTokens,
	)
}

// decNextParams holds parameters for TestDecNextToken.
type decNextParams struct {
	mode   Mode
	input  string
	tokens []token
	error  error
}

var decNextTestCases = map[string]decNextParams{
	// Flat primitive values.
	"string": {
		mode: StrictJson, input: `"hello"`,
		tokens: []token{{String, "hello"}},
	},
	"integer": {
		mode: StrictJson, input: `42`,
		tokens: []token{{Integer, "42"}},
	},
	"decimal": {
		mode: StrictJson, input: `3.14`,
		tokens: []token{{Decimal, "3.14"}},
	},
	"true": {
		mode: StrictJson, input: `true`,
		tokens: []token{{True, "true"}},
	},
	"false": {
		mode: StrictJson, input: `false`,
		tokens: []token{{False, "false"}},
	},
	"null": {
		mode: StrictJson, input: `null`,
		tokens: []token{{Null, "null"}},
	},

	// Strict numeric token types.
	"nan": {
		mode: Strict, input: `NaN`,
		tokens: []token{{NaN, "NaN"}},
	},
	"infinity": {
		mode: Strict, input: `Infinity`,
		tokens: []token{{Infinity, "Infinity"}},
	},
	"infinity-neg": {
		mode: Strict, input: `-Infinity`,
		tokens: []token{{Infinity, "-Infinity"}},
	},
	"hex": {
		mode: Strict, input: `0xFF`,
		tokens: []token{{HexaDecimal, "0xFF"}},
	},
	"complex": {
		mode: Extended, input: `3+4i`,
		tokens: []token{{Complex, "3+4i"}},
	},

	// Empty collections.
	"empty-object": {
		mode: StrictJson, input: `{}`,
		tokens: []token{
			{ObjectStart, "{"},
			{ObjectEnd, "}"},
		},
	},
	"empty-array": {
		mode: StrictJson, input: `[]`,
		tokens: []token{
			{ArrayStart, "["},
			{ArrayEnd, "]"},
		},
	},

	// Simple object with one key.
	"object-int": {
		mode: StrictJson, input: `{"a": 0}`,
		tokens: []token{
			{ObjectStart, "{"}, {String, "a"}, {Integer, "0"}, {ObjectEnd, "}"},
		},
	},

	// Object with nested array value.
	"object-array": {
		mode: StrictJson, input: `{"a": []}`,
		tokens: []token{
			{ObjectStart, "{"},
			{String, "a"},
			{ArrayStart, "["},
			{ArrayEnd, "]"},
			{ObjectEnd, "}"},
		},
	},

	// Object with two keys.
	"object-two-keys": {
		mode: StrictJson, input: `{"a":{}, "b":{}}`,
		tokens: []token{
			{ObjectStart, "{"},
			{String, "a"},
			{ObjectStart, "{"},
			{ObjectEnd, "}"},
			{String, "b"},
			{ObjectStart, "{"},
			{ObjectEnd, "}"},
			{ObjectEnd, "}"},
		},
	},

	// Array with one integer.
	"array-int": {
		mode: StrictJson, input: `[10]`,
		tokens: []token{
			{ArrayStart, "["}, {Integer, "10"}, {ArrayEnd, "]"},
		},
	},

	// Deeply nested structure.
	"deep-nested": {
		mode: StrictJson, input: `[[[[[[{"true":true}]]]]]]`,
		tokens: []token{
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ObjectStart, "{"},
			{String, "true"},
			{True, "true"},
			{ObjectEnd, "}"},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
		},
	},

	// Mixed-type array.
	"array-mixed": {
		mode:  StrictJson,
		input: `[{"a": 1,"b": 123.456, "c": null, "d": [1, -2, "three", true, false, ""]}]`,
		tokens: []token{
			{ArrayStart, "["},
			{ObjectStart, "{"},
			{String, "a"},
			{Integer, "1"},
			{String, "b"},
			{Decimal, "123.456"},
			{String, "c"},
			{Null, "null"},
			{String, "d"},
			{ArrayStart, "["},
			{Integer, "1"},
			{Integer, "-2"},
			{String, "three"},
			{True, "true"},
			{False, "false"},
			{String, ""},
			{ArrayEnd, "]"},
			{ObjectEnd, "}"},
			{ArrayEnd, "]"},
		},
	},

	// Error cases: truncated input.
	"error-truncated-array": {
		mode: StrictJson, input: `[`,
		tokens: []token{{ArrayStart, "["}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-truncated-object": {
		mode: StrictJson, input: `{"":2`,
		tokens: []token{{ObjectStart, "{"}, {String, ""}, {Integer, "2"}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-truncated-key": {
		mode: StrictJson, input: `{"`,
		tokens: []token{{ObjectStart, "{"}},
		error:  io.ErrUnexpectedEOF,
	},

	// Error cases: structural errors.
	"error-bad-key-type": {
		mode: StrictJson, input: `{1: 1}`,
		tokens: []token{{ObjectStart, "{"}},
		error: &DecodeError{
			Msg:   "object key",
			Pos:   Position{Byte: 2},
			Typ:   Integer,
			Token: "1",
		},
	},
	"error-double-colon": {
		mode: StrictJson, input: `{"test"::"input"}`,
		tokens: []token{{ObjectStart, "{"}, {String, "test"}, {Colon, ":"}},
		error: &DecodeError{
			Msg:   "object comma",
			Pos:   Position{Byte: 16},
			Typ:   String,
			Token: "input",
		},
	},
	"error-missing-comma-obj": {
		mode: StrictJson, input: `{"a":1:}`,
		tokens: []token{
			{ObjectStart, "{"}, {String, "a"}, {Integer, "1"},
		},
		error: &DecodeError{
			Msg:   "object comma",
			Pos:   Position{Byte: 7},
			Typ:   Colon,
			Token: ":",
		},
	},
	"error-unexpected-comma-value": {
		mode: StrictJson, input: `,`,
		error: &DecodeError{
			Msg:   "value",
			Pos:   Position{Byte: 1},
			Typ:   Comma,
			Token: ",",
		},
	},
	"error-unexpected-comma-array": {
		mode: StrictJson, input: `[,]`,
		tokens: []token{{ArrayStart, "["}},
		error: &DecodeError{
			Msg:   "array value",
			Pos:   Position{Byte: 2},
			Typ:   Comma,
			Token: ",",
		},
	},
	"error-missing-comma-arr": {
		mode: StrictJson, input: `[1:]`,
		tokens: []token{{ArrayStart, "["}, {Integer, "1"}},
		error: &DecodeError{
			Msg:   "array comma",
			Pos:   Position{Byte: 3},
			Typ:   Colon,
			Token: ":",
		},
	},

	// Error cases: truncated at various state-machine steps.
	"error-empty-input": {
		mode:  StrictJson,
		error: io.ErrUnexpectedEOF,
	},
	"error-truncated-colon": {
		mode: StrictJson, input: `{"a"`,
		tokens: []token{{ObjectStart, "{"}, {String, "a"}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-missing-colon": {
		mode: StrictJson, input: `{"a" 1}`,
		tokens: []token{{ObjectStart, "{"}, {String, "a"}},
		error: &DecodeError{
			Msg:   "object colon",
			Pos:   Position{Byte: 6},
			Typ:   Integer,
			Token: "1",
		},
	},
	"error-truncated-value": {
		mode: StrictJson, input: `{"a":`,
		tokens: []token{{ObjectStart, "{"}, {String, "a"}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-truncated-arr-value": {
		mode: StrictJson, input: `[1`,
		tokens: []token{{ArrayStart, "["}, {Integer, "1"}},
		error:  io.ErrUnexpectedEOF,
	},

	// Array nested in array (exercises stateArrayValue case !inObj).
	"nested-array": {
		mode: StrictJson, input: `[[]]`,
		tokens: []token{
			{ArrayStart, "["},
			{ArrayStart, "["},
			{ArrayEnd, "]"},
			{ArrayEnd, "]"},
		},
	},
}

func TestDecNext(t *testing.T) {
	test.Map(t, decNextTestCases).
		Run(func(t test.Test, param decNextParams) {
			// Given
			decoder := NewDecoder(strings.NewReader(param.input)).
				Mode(param.mode)

			// When / Then (per token)
			for _, want := range param.tokens {
				typ, got, err := decoder.Next()
				require.NoError(t, err)
				assert.Equal(t, want.typ, typ)
				assert.Equal(t, want.token, string(got))
			}

			// Then (terminal state)
			_, last, err := decoder.Next()
			if param.error != nil {
				assert.Equal(t, param.error, err)
			} else {
				assert.Equal(t, io.EOF, err)
				assert.Nil(t, last)
			}
		})
}

// decTokenParams holds parameters for TestDecToken.
type decTokenParams struct {
	mode   Mode
	input  string
	tokens []json.Token
	expect json.Token
	error  error
}

var decTokenTestCases = map[string]decTokenParams{
	// Delimiter tokens.
	"delim-object": {
		mode: Strict, input: `{}`,
		tokens: []json.Token{json.Delim('{'), json.Delim('}')},
	},
	"delim-array": {
		mode: StrictJson, input: `[]`,
		tokens: []json.Token{json.Delim('['), json.Delim(']')},
	},

	// Primitive tokens.
	"bool-true": {
		mode: StrictJson, input: `true`,
		tokens: []json.Token{true},
	},
	"bool-false": {
		mode: StrictJson, input: `false`,
		tokens: []json.Token{false},
	},
	"null": {
		mode: StrictJson, input: `null`,
		tokens: []json.Token{nil},
	},
	"string": {
		mode: StrictJson, input: `"hi"`,
		tokens: []json.Token{"hi"},
	},
	"integer": {
		mode: StrictJson, input: `7`,
		tokens: []json.Token{float64(7)},
	},
	"decimal": {
		mode: StrictJson, input: `1.5`,
		tokens: []json.Token{float64(1.5)},
	},

	// Strict numeric tokens.
	"hex": {
		mode: Strict, input: `0x10`,
		tokens: []json.Token{float64(16)},
	},
	"complex": {
		mode: Extended, input: `1+2i`,
		tokens: []json.Token{complex(1, 2)},
	},
	"infinity-pos": {
		mode: Strict, input: `Infinity`,
		tokens: []json.Token{math.Inf(1)},
	},
	"infinity-neg": {
		mode: Strict, input: `-Infinity`,
		tokens: []json.Token{math.Inf(-1)},
	},
	"nan": {
		// NaN != NaN so we can only check io.EOF; actual value tested via IsNaN.
		mode: Strict, input: `NaN`,
	},

	// Error cases.
	"error-default": {
		mode: StrictJson, input: `:`,
		error: &DecodeError{
			Msg:   "unhandled type",
			Pos:   Position{Byte: 1},
			Typ:   Colon,
			Token: ":",
		},
	},
	"error-number-parse": {
		mode:   StrictJson,
		input:  `1e9999`,
		expect: 0,
		error: decError("number", Position{Byte: 6}, Decimal, "1e9999", nil,
			&strconv.NumError{
				Func: "ParseFloat",
				Num:  "1e9999",
				Err:  strconv.ErrRange,
			}),
	},
	"error-hex-parse": {
		mode:   Strict,
		input:  `0x10000000000000000`,
		expect: 0,
		error: decError("hexa-decimal number", Position{Byte: 19},
			HexaDecimal, "0x10000000000000000", nil, &strconv.NumError{
				Func: "ParseInt",
				Num:  "0x10000000000000000",
				Err:  strconv.ErrRange,
			}),
	},
	"error-complex-parse": {
		mode:   Extended,
		input:  `1e9999+1e9999i`,
		expect: 0,
		error: decError("complex number", Position{Byte: 14}, Complex,
			"1e9999+1e9999i", nil,
			&strconv.NumError{
				Func: "ParseComplex",
				Num:  "1e9999+1e9999i",
				Err:  strconv.ErrRange,
			}),
	},
}

func TestDecToken(t *testing.T) {
	test.Map(t, decTokenTestCases).
		Run(func(t test.Test, param decTokenParams) {
			// Given
			decoder := NewDecoder(strings.NewReader(param.input)).
				Mode(param.mode)

			// When / Then (per token)
			for _, want := range param.tokens {
				got, err := decoder.Token()
				require.NoError(t, err)
				assert.Equal(t, want, got)
			}

			// Special NaN case — check IsNaN separately.
			if param.input == "NaN" {
				got, err := decoder.Token()
				require.NoError(t, err)
				f, ok := got.(float64)
				require.True(t, ok)
				assert.True(t, math.IsNaN(f))
			}

			// Then (terminal state or error)
			token, err := decoder.Token()
			if param.error != nil {
				assert.Equal(t, param.expect, token)
				assert.Equal(t, param.error, err)
			} else {
				assert.Equal(t, io.EOF, err)
				assert.Nil(t, token)
			}
		})
}
