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
	"github.com/tkrop/go-testing/mock"
	"github.com/tkrop/go-testing/test"
)

// FIXME: Improve AI generated test cases.

// newDecoderParams holds parameters for TestDecNew.
type newDecoderParams struct {
	size   int
	expect int
}

// newDecoderTestCases defines test cases for testing the NewDecoder and
// NewDecoderBuffer functions, including default buffer size, custom buffer
// size, and minimum buffer size.
var newDecoderTestCases = map[string]newDecoderParams{
	"default": {expect: 4096},
	"custom":  {size: 256, expect: 256},
	"max":     {size: 8192, expect: 8192},
	"min":     {size: 1, expect: 1},
}

// TestNewDecoder tests the NewDecoder and NewDecoderBuffer functions to ensure
// that the decoder is initialized with the correct buffer size and state. It
// verifies that the default buffer size is used when no custom buffer is
// provided, and that a custom buffer size is respected.
func TestNewDecoder(t *testing.T) {
	test.Map(t, newDecoderTestCases).
		Run(func(t test.Test, p newDecoderParams) {
			// When
			var decoder *Decoder
			if p.size == 0 {
				decoder = NewDecoderReader(strings.NewReader(""))
			} else {
				decoder = NewDecoderSource(
					strings.NewReader(""), make([]byte, p.size))
			}

			// Then
			require.NotNil(t, decoder)
			assert.NotNil(t, decoder.state)
			assert.Equal(t, p.expect, cap(
				test.Cast[*scanner](decoder.scanner).reader.buffer))
		})
}

// decodeError is a helper function that constructs a DecodeError with the
// given message, position, token type, token string, expected type, and
// underlying error.
func decodeError(
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

// clearPos clears the position information from a DecodeError, returning a
// new error with the position set to zero.
func clearPos(err error) error {
	if err == nil {
		return nil
	}

	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		return err
	}

	copy := *decodeErr
	copy.Pos = Position{}

	return &copy
}

type structInternalObject struct {
	Name string `json:"name"`
}

type decodeOmittedObject struct {
	Enabled bool `json:"enabled"`
	Count   int  `json:"count"`
}

type structRepeatedObject struct {
	First  structInternalObject `json:"first"`
	Second structInternalObject `json:"second"`
}

type structMapValues struct {
	Values map[string]int `json:"values"`
}

// decDecodeParams holds parameters for testing the decode method of the decoder.
type decDecodeParams struct {
	mode   Mode
	input  string
	tokens []Token
	pos    []Position
	output any
	expect any
	error  error
	check  func(t test.Test, output any)
}

// DecodeMode checks whether the given mode is enabled by the decoder
// test.
func DecodeMode(mode ...Mode) test.FilterFunc[decDecodeParams] {
	return func(_ string, param decDecodeParams) bool {
		for _, m := range mode {
			if param.mode&m == m {
				return true
			}
		}

		return false
	}
}

func CallScannerGetMode(mode Mode) mock.SetupFunc {
	return func(mocks *mock.Mocks) any {
		scanner := mock.Get(mocks, NewMockScanner)

		return scanner.EXPECT().GetMode().Return(mode).Times(1)
	}
}

func CallScannerSetMode(mode Mode) mock.SetupFunc {
	return func(mocks *mock.Mocks) any {
		scanner := mock.Get(mocks, NewMockScanner)

		return scanner.EXPECT().SetMode(mode).Return(scanner).Times(1)
	}
}

func CallScannerNext(token Token) mock.SetupFunc {
	return func(mocks *mock.Mocks) any {
		scanner := mock.Get(mocks, NewMockScanner)

		return scanner.EXPECT().Next(token.ctx).
			Return(token.typ, []byte(token.token)).Times(1)
	}
}

func CallScannerPosition(pos Position) mock.SetupFunc {
	return func(mocks *mock.Mocks) any {
		scanner := mock.Get(mocks, NewMockScanner)

		return scanner.EXPECT().Position().Return(pos).Times(1)
	}
}

// decDecodeTestCases defines test cases for testing the Decode method of the
// Decoder.
var decDecodeTestCases = map[string]decDecodeParams{
	"zero-bool": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(bool),
		expect: test.Ptr(false),
	},
	"zero-int": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(int),
		expect: test.Ptr(0),
	},
	"zero-ptr": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: test.Ptr(new(int)),
		expect: test.Ptr((*int)(nil)),
	},
	"zero-float64": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(float64),
		expect: test.Ptr(float64(0)),
	},
	"zero-float32": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(float32),
		expect: test.Ptr(float32(0)),
	},
	"zero-string": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(string),
		expect: test.Ptr(""),
	},
	"zero-any": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: test.Ptr[any](nil),
		expect: test.Ptr[any](nil),
	},
	"zero-map": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new(map[int]string),
		expect: new(map[int]string),
	},
	"zero-array": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
		output: new([]int),
		expect: new([]int),
	},

	// Bool targets.
	"bool-true": {
		mode: Json, input: `true`,
		tokens: []Token{{ctx: RootValue, typ: True, token: "true"}},
		output: new(bool),
		expect: test.Ptr(true),
	},
	"bool-false": {
		mode: Json, input: `false`,
		tokens: []Token{{ctx: RootValue, typ: False, token: "false"}},
		output: new(bool),
		expect: test.Ptr(false),
	},
	"bool-any-true": {
		mode: Json, input: `true`,
		tokens: []Token{{ctx: RootValue, typ: True, token: "true"}},
		output: test.Ptr[any](false),
		expect: test.Ptr[any](true),
	},
	"bool-any-false": {
		mode: Json, input: `false`,
		tokens: []Token{{ctx: RootValue, typ: False, token: "false"}},
		output: test.Ptr[any](true),
		expect: test.Ptr[any](false),
	},

	// Null into pointer/map/slice.
	"null-ptr": {
		mode: Json, input: `null`,
		tokens: []Token{{ctx: RootValue, typ: Null, token: "null"}},
		output: test.Ptr(new(int)),
		expect: test.Ptr((*int)(nil)),
	},
	"null-map": {
		mode: Json, input: `null`,
		tokens: []Token{{ctx: RootValue, typ: Null, token: "null"}},
		output: test.Ptr(map[int]string{}),
		expect: test.Ptr((map[int]string)(nil)),
	},
	"null-slice": {
		mode:   Json,
		input:  `null`,
		tokens: []Token{{ctx: RootValue, typ: Null, token: "null"}},
		output: test.Ptr([]string{"a"}),
		expect: test.Ptr(([]string)(nil)),
	},

	// Numeric targets.
	"float64-any": {
		mode: Json, input: `3`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "3"}},
		output: new(any),
		expect: test.Ptr[any](float64(3)),
	},
	"float64": {
		mode: Json, input: `1`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		output: new(float64),
		expect: test.Ptr(float64(1)),
	},
	"float32": {
		mode: Json, input: `1`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		output: new(float32),
		expect: test.Ptr(float32(1)),
	},
	"int": {
		mode: Json, input: `1`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		output: new(int),
		expect: test.Ptr(1),
	},
	"int64-neg": {
		mode: Json, input: `-1`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "-1"}},
		output: new(int64),
		expect: test.Ptr(int64(-1)),
	},
	"uint": {
		mode: Json, input: `1`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		output: new(uint),
		expect: test.Ptr(uint(1)),
	},

	// Hex numeric target.
	"hex-any": {
		mode: Strict, input: `0xFF`,
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0xFF"}},
		output: new(any),
		expect: test.Ptr[any](float64(255)),
	},
	"hex-int": {
		mode: Strict, input: `0x10`,
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0x10"}},
		output: new(int),
		expect: test.Ptr(16),
	},
	"hex-uint": {
		mode: Strict, input: `0x10`,
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0x10"}},
		output: new(uint),
		expect: test.Ptr(uint(16)),
	},
	"hex-float64": {
		mode: Strict, input: `0x10`,
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0x10"}},
		output: new(float64),
		expect: test.Ptr(float64(16)),
	},

	// Complex target.
	"complex-any": {
		mode: Extended, input: `1+2i`,
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "1+2i"}},
		output: new(any),
		expect: test.Ptr[any](complex(1, 2)),
	},
	"complex128": {
		mode: Extended, input: `3+4i`,
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "3+4i"}},
		output: new(complex128),
		expect: test.Ptr(complex128(3 + 4i)),
	},
	"complex64": {
		mode: Extended, input: `1+2i`,
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "1+2i"}},
		output: new(complex64),
		expect: test.Ptr(complex64(1 + 2i)),
	},

	// Infinity target.
	"infinity-pos": {
		mode: Strict, input: `Infinity`,
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "Infinity"}},
		output: new(float64),
		expect: test.Ptr(math.Inf(1)),
	},
	"infinity-neg": {
		mode: Strict, input: `-Infinity`,
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "-Infinity"}},
		output: new(float64),
		expect: test.Ptr(math.Inf(-1)),
	},
	"infinity-any": {
		mode: Strict, input: `Infinity`,
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "Infinity"}},
		output: new(any),
		expect: test.Ptr[any](math.Inf(1)),
	},

	// String targets.
	"string": {
		mode: Json, input: `"hello"`,
		tokens: []Token{{ctx: RootValue, typ: String, token: "hello"}},
		output: new(string),
		expect: test.Ptr("hello"),
	},
	"string-any": {
		mode: Json, input: `"world"`,
		tokens: []Token{{ctx: RootValue, typ: String, token: "world"}},
		output: new(any),
		expect: test.Ptr[any]("world"),
	},
	"plain-string": {
		mode: Relaxed, input: `hello`,
		tokens: []Token{{ctx: RootValue, typ: Plain, token: "hello"}},
		output: new(string),
		expect: test.Ptr("hello"),
	},
	"plain-any": {
		mode: Relaxed, input: `hello`,
		tokens: []Token{{ctx: RootValue, typ: Plain, token: "hello"}},
		output: new(any),
		expect: test.Ptr[any]("hello"),
	},
	"quote-string": {
		mode: Strict, input: `'hello'`,
		tokens: []Token{{ctx: RootValue, typ: Quote, token: "hello"}},
		output: new(string),
		expect: test.Ptr("hello"),
	},
	"quote-any": {
		mode: Strict, input: `'hello'`,
		tokens: []Token{{ctx: RootValue, typ: Quote, token: "hello"}},
		output: new(any),
		expect: test.Ptr[any]("hello"),
	},

	// Object/map targets.
	"object-empty-any": {
		mode: Json, input: `{}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{}),
	},
	"object-empty-struct": {
		mode: Json, input: `{}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structInternalObject{},
		expect: &structInternalObject{},
	},
	"object-unknown-field-skip": {
		mode: Json, input: `{"missing":true}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: True, token: "true"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structInternalObject{},
		expect: &structInternalObject{},
	},
	"object-repeated-nested-struct": {
		mode: Json,
		input: `{"first":{"name":"one"},` +
			`"second":{"name":"two"}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "first"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "one"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "second"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "two"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structRepeatedObject{},
		expect: &structRepeatedObject{
			First:  structInternalObject{Name: "one"},
			Second: structInternalObject{Name: "two"},
		},
	},
	"object-nested-any": {
		mode: Json, input: `{"a": 1, "b": {"c": 2}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "c"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "2"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"a": big.NewInt(1),
			"b": map[string]any{"c": big.NewInt(2)},
		}),
	},
	"object-map-string": {
		mode:  Json,
		input: `{"hello": "world"}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "hello"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "world"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &map[string]string{},
		expect: &map[string]string{"hello": "world"},
	},
	"object-map-any": {
		mode:  Json,
		input: `{"a": 1, "b": false, "c":[1, 2.0, "three"]}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: False, token: "false"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "c"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Decimal, token: "2.0"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: String, token: "three"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &map[string]any{},
		expect: &map[string]any{
			"a": float64(1), "b": false, "c": []any{
				big.NewInt(1), test.Okay((&big.Float{}).SetString("2.0")), "three",
			},
		},
	},

	// Array/slice targets.
	"array-string-strict-json": {
		mode:  Json,
		input: `["a","b"]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: String, token: "a"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: String, token: "b"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: &[]string{},
		expect: &[]string{"a", "b"},
	},
	"array-string-truncated": {
		mode: Json, input: `[`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: EOF},
		},
		output: &[]string{},
		error:  io.ErrUnexpectedEOF,
	},
	"array-string-element-type": {
		mode: Json, input: `[true]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: True, token: "true"},
		},
		pos:    []Position{{Byte: 5}},
		output: &[]string{},
		error: decodeError("bool type", Position{Byte: 5}, True,
			"true", reflect.TypeOf(""), nil),
	},
	"array-string-relaxed-identifiers": {
		mode:  Relaxed,
		input: `[a,b]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Plain, token: "a"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Plain, token: "b"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: &[]string{},
		expect: &[]string{"a", "b"},
	},
	"array-any-relaxed-identifiers": {
		mode:  Relaxed,
		input: `[a,b]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Plain, token: "a"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Plain, token: "b"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: &[]any{},
		expect: &[]any{"a", "b"},
	},
	"array-nested-any": {
		mode: Json, input: `[{"a": [{}]}]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new(any),
		expect: test.Ptr[any]([]any{
			map[string]any{"a": []any{map[string]any{}}},
		}),
	},

	// Error cases.
	"error-invalid-interface": {
		mode: Json, input: `1`, output: nil,
		tokens: []Token{},
		pos:    []Position{{}},
		error:  decodeError("invalid", Position{}, 0, "", nil, nil),
	},
	"error-non-pointer": {
		mode: Json, input: `1`, output: 42,
		tokens: []Token{},
		pos:    []Position{{}},
		error: decodeError("no-pointer", Position{}, 0, "",
			reflect.TypeOf(42), nil),
	},
	"error-nil-pointer": {
		mode: Json, input: `1`, output: (*int)(nil),
		tokens: []Token{},
		pos:    []Position{{}},
		error: decodeError("nil", Position{}, 0, "",
			reflect.TypeOf((*int)(nil)), nil),
	},
	"error-object-unhandled-kind": {
		mode: Json, input: `{}`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: ObjectStart, token: "{"}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("object type", Position{Byte: 1},
			ObjectStart, "{", reflect.TypeOf(int(0)), nil),
	},
	"error-array-unhandled-kind": {
		mode: Json, input: `[]`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: ArrayStart, token: "["}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("array type", Position{Byte: 1},
			ArrayStart, "[", reflect.TypeOf(int(0)), nil),
	},
	"error-bool-unhandled-kind": {
		mode: Json, input: `true`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: True, token: "true"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("bool type", Position{Byte: 4},
			True, "true", reflect.TypeOf(int(0)), nil),
	},
	"error-null-unhandled-kind": {
		mode: Json, input: `null`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: Null, token: "null"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("null type", Position{Byte: 4},
			Null, "null", reflect.TypeOf(int(0)), nil),
	},
	"error-string-unhandled-kind": {
		mode: Json, input: `"x"`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: String, token: "x"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("string type", Position{Byte: 3},
			String, "x", reflect.TypeOf(int(0)), nil),
	},
	"error-int-overflow": {
		mode: Json, input: `300`, output: new(int8),
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "300"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("number int", Position{Byte: 3},
			Integer, "300", reflect.TypeOf(int8(0)), nil),
	},
	"error-uint-overflow": {
		mode: Json, input: `300`, output: new(uint8),
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "300"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("number uint", Position{Byte: 3},
			Integer, "300", reflect.TypeOf(uint8(0)), nil),
	},
	"error-non-string-map-key": {
		mode: Json, input: `{"a":1}`, output: &map[int]int{},
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
		},
		pos: []Position{{Byte: 1}},
		error: decodeError("map key", Position{Byte: 1},
			ObjectStart, "", reflect.TypeOf(map[int]int{}), nil),
	},
	"error-hex-unhandled-kind": {
		mode: Strict, input: `0xFF`, output: new(bool),
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0xFF"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("hexa-decimal number type", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf(false), nil),
	},
	"error-complex-unhandled-kind": {
		mode: Extended, input: `1+2i`, output: new(float64),
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "1+2i"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("complex number type", Position{Byte: 4},
			Complex, "1+2i", reflect.TypeOf(float64(0)), nil,
		),
	},
	"error-infinity-unhandled-kind": {
		mode: Strict, input: `Infinity`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "Infinity"}},
		pos:    []Position{{Byte: 8}},
		error: decodeError("infinity type", Position{Byte: 8},
			Infinity, "Infinity", reflect.TypeOf(int(0)), nil),
	},
	"error-nan-unhandled-kind": {
		mode: Strict, input: `NaN`, output: new(int),
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("nan type", Position{Byte: 3}, NaN, "NaN",
			reflect.TypeOf(int(0)), nil),
	},

	// NaN targets (require special IsNaN check).
	"nan-float64": {
		mode: Strict, input: `NaN`,
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
		output: new(float64),
		check: func(t test.Test, out any) {
			assert.True(t, math.IsNaN(*test.Cast[*float64](out)))
		},
	},
	"nan-float32": {
		mode: Strict, input: `NaN`,
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
		output: new(float32),
		check: func(t test.Test, out any) {
			assert.True(t, math.IsNaN(float64(*test.Cast[*float32](out))))
		},
	},
	"nan-any": {
		mode: Strict, input: `NaN`,
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
		output: new(any),
		check: func(t test.Test, out any) {
			f, ok := (*out.(*any)).(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},

	// Decode into any with comprehensive value types (decodeValueAny coverage).
	"object-all-values-any": {
		mode:  Json,
		input: `{"a":true,"b":false,"c":null,"d":"str","e":[1]}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: True, token: "true"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: False, token: "false"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "c"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Null, token: "null"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "d"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "str"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "e"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"a": true, "b": false, "c": nil, "d": "str",
			"e": []any{big.NewInt(1)},
		}),
	},
	"object-decimal-any": {
		mode:  Json,
		input: `{"n":1.5}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "n"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Decimal, token: "1.5"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"n": test.Okay((&big.Float{}).SetString("1.5")),
		}),
	},
	"object-extended-any": {
		mode:  Extended,
		input: `{"h":0xFF,"c":1+2i,"i":Infinity,"ni":-Infinity}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "h"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: HexaDecimal, token: "0xFF"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "c"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Complex, token: "1+2i"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "i"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Infinity, token: "Infinity"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: String, token: "ni"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Infinity, token: "-Infinity"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"h": big.NewInt(255), "c": complex(1, 2),
			"i": math.Inf(1), "ni": math.Inf(-1),
		}),
	},
	"object-key-zero": {
		mode:  Relaxed,
		input: `{ :1}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Zero, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{"": 1}),
	},
	"object-key-complex": {
		mode:  Relaxed | Extended,
		input: `{i:1}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Complex, token: "i"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{"i": 1}),
	},
	"object-key-identifier": {
		mode:  Relaxed,
		input: `{name:0}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Plain, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{"name": 0}),
	},
	"object-plain-key-value": {
		mode:  Relaxed,
		input: `{name:value}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Plain, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Plain, token: "value"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]string),
		expect: &map[string]string{"name": "value"},
	},
	"object-key-scalar-tokens": {
		mode: Relaxed | Extended,
		input: `{1:0,1.5:0,1+2i:0,0x10:0,Infinity:0,` +
			`NaN:0,true:0,false:0,null:0}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Integer, token: "1"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: Decimal, token: "1.5"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: Complex, token: "1+2i"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: HexaDecimal, token: "0x10"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: Infinity, token: "Infinity"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: NaN, token: "NaN"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: True, token: "true"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: False, token: "false"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: Comma, token: ","},
			{ctx: ObjectKey, typ: Null, token: "null"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{
			"1": 0, "1.5": 0, "1+2i": 0, "0x10": 0,
			"Infinity": 0, "NaN": 0, "true": 0,
			"false": 0, "null": 0,
		}),
	},
	"error-object-key-integer-strict": {
		mode:  Json,
		input: `{1:0}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Integer, token: "1"},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			Integer, "1", nil, nil),
	},
	"error-object-key-complex-extended-only": {
		mode:  Extended,
		input: `{1+2i:0}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Complex, token: "1+2i"},
		},
		pos:    []Position{{Byte: 5}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 5},
			Complex, "1+2i", nil, nil),
	},
	"error-object-key-quote-strict-json": {
		mode: Mock | Json,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Quote, token: "name"},
		},
		output: new(map[string]int),
		error: decodeError("object key", Position{},
			Quote, "name", nil, nil),
	},
	"object-key-quote-relaxed": {
		mode: Mock | Relaxed,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Quote, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{"name": 0}),
	},
	"error-object-key-zero-strict": {
		mode: Mock | Json,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Zero, token: ":"},
		},
		output: new(map[string]int),
		error: decodeError("object key", Position{},
			Zero, ":", nil, nil),
	},
	"error-object-key-zero-empty": {
		mode: Mock | Relaxed,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Zero},
		},
		output: new(map[string]int),
		error: decodeError("object key", Position{},
			Zero, "", nil, nil),
	},
	"error-object-key-zero-noncolon": {
		mode: Mock | Relaxed,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Zero, token: "x"},
		},
		output: new(map[string]int),
		error: decodeError("object key", Position{},
			Zero, "x", nil, nil),
	},
	"error-object-key-object-start": {
		mode:  Json,
		input: `{{`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ObjectStart, token: "{"},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			ObjectStart, "{", nil, nil),
	},
	"error-object-key-array-start": {
		mode:  Json,
		input: `{[`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ArrayStart, token: "["},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			ArrayStart, "[", nil, nil),
	},
	"error-object-key-array-end": {
		mode:  Json,
		input: `{]`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ArrayEnd, token: "]"},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			ArrayEnd, "]", nil, nil),
	},
	"error-object-key-comma": {
		mode:  Json,
		input: `{,`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Comma, token: ","},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			Comma, ",", nil, nil),
	},
	"error-object-key-colon": {
		mode:  Json,
		input: `{:`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Colon, token: ":"},
		},
		pos:    []Position{{Byte: 2}},
		output: new(map[string]int),
		error: decodeError("object key", Position{Byte: 2},
			Colon, ":", nil, nil),
	},
	"object-key-zero-mock-valid": {
		mode: Mock | Relaxed,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Zero, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]int),
		expect: test.Ptr(map[string]int{"": 1}),
	},
	"object-omitted-values-zero": {
		mode: Relaxed, input: `{"enabled":,"count":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: ","},
			{ctx: ObjectKey, typ: String, token: "count"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
		},
		output: &decodeOmittedObject{Enabled: true, Count: 7},
		expect: &decodeOmittedObject{},
	},
	"object-omitted-nested-array": {
		mode: Relaxed, input: `[{"enabled":}]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: &[]decodeOmittedObject{{Enabled: true, Count: 7}},
		expect: &[]decodeOmittedObject{{}},
	},
	"object-omitted-nested-object": {
		mode: Relaxed, input: `{"first":{"name":}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "first"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structRepeatedObject{
			First: structInternalObject{Name: "existing"},
		},
		expect: &structRepeatedObject{},
	},
	"object-omitted-any": {
		mode: Relaxed, input: `{"missing":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{"missing": nil}),
	},
	"object-omitted-unknown-field": {
		mode: Relaxed, input: `{"missing":,"name":"known"}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: ","},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "known"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structInternalObject{Name: "before"},
		expect: &structInternalObject{Name: "known"},
	},
	"object-map-omitted-value": {
		mode: Relaxed, input: `{"missing":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
		},
		output: &map[string]int{"missing": 7},
		expect: &map[string]int{"missing": 0},
	},
	"object-omitted-nested-any": {
		mode: Relaxed, input: `{"outer":{"missing":}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "outer"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		expect: test.Ptr[any](map[string]any{
			"outer": map[string]any{"missing": nil},
		}),
	},
	"object-omitted-nested-comma": {
		mode: Relaxed, input: `{"first":{"name":,"unknown":true}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "first"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: ","},
			{ctx: ObjectKey, typ: String, token: "unknown"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: True, token: "true"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structRepeatedObject{
			First: structInternalObject{Name: "before"},
		},
		expect: &structRepeatedObject{},
	},

	// Decode into any with array of all value types.
	"array-all-values-any": {
		mode:  Json,
		input: `[true, false, null, "str", 1, 1.5, {"a":1}, [2]]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: True, token: "true"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: False, token: "false"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Null, token: "null"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: String, token: "str"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Decimal, token: "1.5"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
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
	},
	"array-extended-any": {
		mode:  Extended,
		input: `[0xFF, 1+2i, Infinity, -Infinity]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: HexaDecimal, token: "0xFF"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Complex, token: "1+2i"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Infinity, token: "Infinity"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Infinity, token: "-Infinity"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new(any),
		expect: test.Ptr[any]([]any{
			big.NewInt(255), complex(1, 2), math.Inf(1), math.Inf(-1),
		}),
	},

	// Decoding NaN.
	"nan-slice-any": {
		mode: Strict, input: `[NaN]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: NaN, token: "NaN"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new(any),
		check: func(t test.Test, out any) {
			s, ok := (*out.(*any)).([]any)
			require.True(t, ok)
			require.Len(t, s, 1)
			f, ok := s[0].(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},
	"nan-map-any": {
		mode: Strict, input: `{"n":NaN}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "n"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: NaN, token: "NaN"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(any),
		check: func(t test.Test, out any) {
			m, ok := (*out.(*any)).(map[string]any)
			require.True(t, ok)
			f, ok := m["n"].(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(f))
		},
	},

	// Errors for JSON5 string tokens in strict JSON mode.
	"error-json-quote-root": {
		mode: Json, input: `'text'`, output: new(any),
		tokens: []Token{{ctx: RootValue, typ: Quote, token: "text"}},
		pos:    []Position{{Byte: 6}},
		error: decodeError("value", Position{Byte: 6},
			Quote, "text", nil, nil),
	},
	"error-json-quote-array": {
		mode: Json, input: `['text']`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Quote, token: "text"},
		},
		pos: []Position{{Byte: 7}},
		error: decodeError("array value", Position{Byte: 7},
			Quote, "text", nil, nil),
	},
	"error-json-quote-object-key": {
		mode: Json, input: `{'text':1}`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Quote, token: "text"},
		},
		pos: []Position{{Byte: 7}},
		error: decodeError("object key", Position{Byte: 7},
			Quote, "text", nil, nil),
	},
	"error-json-quote-object-value": {
		mode: Json, input: `{"key":'text'}`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "key"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Quote, token: "text"},
		},
		pos: []Position{{Byte: 13}},
		error: decodeError("object value", Position{Byte: 13},
			Quote, "text", nil, nil),
	},
	"error-json-plain-root": {
		mode: Mock | Json, output: new(any),
		tokens: []Token{{ctx: RootValue, typ: Plain, token: "text"}},
		error: decodeError("value", Position{},
			Plain, "text", nil, nil),
	},
	"error-json-plain-array": {
		mode: Mock | Json, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Plain, token: "text"},
		},
		error: decodeError("array value", Position{},
			Plain, "text", nil, nil),
	},
	"error-json-plain-object-key": {
		mode: Mock | Json, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: Plain, token: "text"},
		},
		error: decodeError("object key", Position{},
			Plain, "text", nil, nil),
	},
	"error-json-plain-object-value": {
		mode: Mock | Json, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "key"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Plain, token: "text"},
		},
		error: decodeError("object value", Position{},
			Plain, "text", nil, nil),
	},

	// Errors unhandled tokens.
	"error-decode-valueany-default": {
		mode: Json, input: `{"a"::}`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Colon, token: ":"},
		},
		pos:   []Position{{Byte: 6}},
		error: decodeError("unknown", Position{Byte: 6}, Colon, ":", nil, nil),
	},
	"error-decode-empty": {
		mode: Json, input: ``, output: new(any),
		tokens: []Token{{ctx: RootValue, typ: EOF}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-decode-colon": {
		mode: Json, input: `:`, output: new(any),
		tokens: []Token{{ctx: RootValue, typ: Colon, token: ":"}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("unknown", Position{Byte: 1},
			Colon, ":", reflect.TypeOf((*any)(nil)).Elem(), nil),
	},

	// Errors decoding maps.
	"error-decode-map-value": {
		mode: Json, input: `{"a":{}}`, output: &map[string]int{},
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
		},
		pos: []Position{{Byte: 6}},
		error: decodeError("object type", Position{Byte: 6},
			ObjectStart, "{", reflect.TypeOf(int(0)), nil),
	},
	"error-decode-obj-any-truncated-key": {
		mode: Json, input: `{`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: EOF},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-decode-obj-any-truncated-value": {
		mode: Json, input: `{"a":`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: EOF},
		},
		error: io.ErrUnexpectedEOF,
	},

	// Errors decoding slices.
	"error-decode-slice-truncated": {
		mode: Json, input: `[`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: EOF},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-decode-slice-obj-truncated": {
		mode: Json, input: `[{`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: EOF},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-decode-slice-arr-truncated": {
		mode: Json, input: `[[`, output: new(any),
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: EOF},
		},
		error: io.ErrUnexpectedEOF,
	},

	// Errors decoding numbers.
	"error-object-interface-methods": {
		mode: Json, input: `{}`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: ObjectStart, token: "{"}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("object", Position{Byte: 1},
			ObjectStart, "{", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-array-interface-methods": {
		mode: Json, input: `[]`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: ArrayStart, token: "["}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("array", Position{Byte: 1},
			ArrayStart, "[", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-bool-interface-methods": {
		mode: Json, input: `true`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: True, token: "true"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("bool", Position{Byte: 4},
			True, "true", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-string-interface-methods": {
		mode: Json, input: `"x"`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: String, token: "x"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("string", Position{Byte: 3},
			String, "x", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-int-interface-methods": {
		mode: Json, input: `1`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("number", Position{Byte: 1},
			Integer, "1", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-hex-interface-methods": {
		mode: Strict, input: `0xFF`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0xFF"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("hexa-decimal number", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-complex-interface-methods": {
		mode: Extended, input: `1+2i`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "1+2i"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("complex number", Position{Byte: 4},
			Complex, "1+2i", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-complex-any-parse": {
		mode: Extended, input: `1e9999+1e9999i`, output: new(any),
		tokens: []Token{{
			ctx: RootValue, typ: Complex, token: "1e9999+1e9999i",
		}},
		pos: []Position{{Byte: 14}},
		error: decodeError("complex number", Position{Byte: 14},
			Complex, "1e9999+1e9999i",
			reflect.TypeOf((*any)(nil)).Elem(), &strconv.NumError{
				Func: "ParseComplex", Num: "1e9999+1e9999i",
				Err: strconv.ErrRange,
			}),
	},
	"error-complex64-parse": {
		mode: Extended, input: `1e9999+1e9999i`, output: new(complex64),
		tokens: []Token{{
			ctx: RootValue, typ: Complex, token: "1e9999+1e9999i",
		}},
		pos: []Position{{Byte: 14}},
		error: decodeError("complex number", Position{Byte: 14},
			Complex, "1e9999+1e9999i", reflect.TypeOf(complex64(0)),
			&strconv.NumError{
				Func: "ParseComplex", Num: "1e9999+1e9999i",
				Err: strconv.ErrRange,
			}),
	},
	"error-complex128-parse": {
		mode: Extended, input: `1e9999+1e9999i`, output: new(complex128),
		tokens: []Token{{
			ctx: RootValue, typ: Complex, token: "1e9999+1e9999i",
		}},
		pos: []Position{{Byte: 14}},
		error: decodeError("complex number", Position{Byte: 14},
			Complex, "1e9999+1e9999i", reflect.TypeOf(complex128(0)),
			&strconv.NumError{
				Func: "ParseComplex", Num: "1e9999+1e9999i",
				Err: strconv.ErrRange,
			}),
	},
	"error-infinity-interface-methods": {
		mode: Strict, input: `Infinity`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "Infinity"}},
		pos:    []Position{{Byte: 8}},
		error: decodeError("infinity", Position{Byte: 8},
			Infinity, "Infinity", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-nan-interface-methods": {
		mode: Strict, input: `NaN`, output: new(error),
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
		pos:    []Position{{Byte: 3}},
		error: decodeError("nan", Position{Byte: 3},
			NaN, "NaN", reflect.TypeOf((*error)(nil)).Elem(), nil),
	},
	"error-int-target-bool": {
		mode: Json, input: `1`, output: new(bool),
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "1"}},
		pos:    []Position{{Byte: 1}},
		error: decodeError("number type", Position{Byte: 1},
			Integer, "1", reflect.TypeOf(false), nil),
	},
	"error-float32-overflow": {
		mode: Json, input: `4e38`, output: new(float32),
		tokens: []Token{{ctx: RootValue, typ: Decimal, token: "4e38"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("number float", Position{Byte: 4},
			Decimal, "4e38", reflect.TypeOf(float32(0)), &strconv.NumError{
				Func: "ParseFloat",
				Num:  "4e38",
				Err:  strconv.ErrRange,
			}),
	},
	"error-hex-int-overflow": {
		mode: Strict, input: `0xFF`, output: new(int8),
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0xFF"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("hexa-decimal number int overflow", Position{Byte: 4},
			HexaDecimal, "0xFF", reflect.TypeOf(int8(0)), nil),
	},
	"error-hex-uint-negative": {
		mode: Strict, input: `-0x1`, output: new(uint8),
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "-0x1"}},
		pos:    []Position{{Byte: 4}},
		error: decodeError("hexa-decimal number uint overflow", Position{Byte: 4},
			HexaDecimal, "-0x1", reflect.TypeOf(uint8(0)), nil),
	},

	// Errors decoding maps.
	"error-decode-map-key-truncated": {
		mode: Json, input: `{`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: EOF},
		},
		output: func() any {
			m := map[string]string{}

			return &m
		}(),
		error: io.ErrUnexpectedEOF,
	},
	"error-decode-any-complex-parse": {
		mode:  Extended,
		input: `{"value":1e9999+1e9999i}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "value"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Complex, token: "1e9999+1e9999i"},
		},
		pos:    []Position{{Byte: 23}},
		output: new(any),
		error: decodeError("complex number", Position{Byte: 23}, Complex,
			"1e9999+1e9999i",
			nil, &strconv.NumError{
				Func: "ParseComplex",
				Num:  "1e9999+1e9999i",
				Err:  strconv.ErrRange,
			}),
	},
	"error-decode-any-hex-parse": {
		mode:  Strict,
		input: `{"value":0x}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "value"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: HexaDecimal, token: "0x"},
		},
		pos:    []Position{{Byte: 11}},
		output: new(any),
		error: decodeError("hexa-decimal number", Position{Byte: 11},
			HexaDecimal, "0x", nil, nil),
	},
	"error-decode-any-integer-parse": {
		mode: Mock | Json,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "value"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "not-an-integer"},
		},
		pos:    []Position{{}},
		output: new(any),
		error: decodeError("number integer", Position{}, Integer,
			"not-an-integer", nil, nil),
	},
	"error-decode-slice-any-integer-parse": {
		mode: Mock | Json,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "not-an-integer"},
		},
		pos:    []Position{{}},
		output: new(any),
		error: decodeError("number integer", Position{}, Integer,
			"not-an-integer", nil, nil),
	},
	"error-decode-zero-empty-token": {
		mode:  Mock | Relaxed,
		input: `{"enabled":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: ""},
		},
		pos:    []Position{{}},
		output: new(decodeOmittedObject),
		error:  decodeError("object value", Position{}, Zero, "", nil, nil),
	},
	"error-decode-zero-invalid-delimiter": {
		mode:  Mock | Relaxed,
		input: `{"enabled":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "x"},
		},
		pos:    []Position{{}},
		output: new(decodeOmittedObject),
		error:  decodeError("object value", Position{}, Zero, "x", nil, nil),
	},
	"object-omitted-close-root": {
		mode:  Mock | Relaxed,
		input: `{"enabled":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
		},
		output: &decodeOmittedObject{Enabled: true},
		expect: &decodeOmittedObject{},
	},
	"object-omitted-close-in-object": {
		mode:  Mock | Relaxed,
		input: `{"first":{"name":}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "first"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: &structRepeatedObject{
			First: structInternalObject{Name: "existing"},
		},
		expect: &structRepeatedObject{},
	},
	"object-omitted-close-in-array": {
		mode:  Mock | Relaxed,
		input: `[{"enabled":}]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "enabled"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: &[]decodeOmittedObject{{Enabled: true}},
		expect: &[]decodeOmittedObject{{}},
	},
	"error-decode-any-decimal-parse": {
		mode:  Strict,
		input: `{"value":-.}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "value"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Decimal, token: "-."},
		},
		output: new(any),
		pos:    []Position{{Byte: 11}},
		error: decodeError("number decimal", Position{Byte: 11},
			Decimal, "-.", nil, nil),
	},
	"error-decode-slice-any-complex-parse": {
		mode:  Extended,
		input: `[1e9999+1e9999i]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Complex, token: "1e9999+1e9999i"},
		},
		pos:    []Position{{Byte: 15}},
		output: new(any),
		error: decodeError("complex number", Position{Byte: 15}, Complex,
			"1e9999+1e9999i",
			nil, &strconv.NumError{
				Func: "ParseComplex",
				Num:  "1e9999+1e9999i",
				Err:  strconv.ErrRange,
			}),
	},
	"error-decode-slice-any-hex-parse": {
		mode:  Strict,
		input: `[0x]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: HexaDecimal, token: "0x"},
		},
		pos:    []Position{{Byte: 3}},
		output: new(any),
		error: decodeError("hexa-decimal number", Position{Byte: 3},
			HexaDecimal, "0x", nil, nil),
	},
	"error-decode-slice-any-decimal-parse": {
		mode:  Strict,
		input: `[-.]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Decimal, token: "-."},
		},
		pos:    []Position{{Byte: 3}},
		output: new(any),
		error: decodeError("number decimal", Position{Byte: 3},
			Decimal, "-.", nil, nil),
	},
	"error-decode-struct-next": {
		mode:  Json,
		input: `{`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: EOF},
		},
		output: &structInternalObject{},
		error:  io.ErrUnexpectedEOF,
	},
	"error-decode-struct-skip-unknown-value": {
		mode:  Json,
		input: `{"missing":,}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "missing"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Comma, token: ","},
		},
		output: &structInternalObject{},
		pos:    []Position{{Byte: 12}},
		error: decodeError("unknown", Position{Byte: 12}, Comma, ",",
			reflect.TypeOf((*any)(nil)).Elem(), nil),
	},
	"error-decode-struct-field-value": {
		mode:  Json,
		input: `{"name":,}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Comma, token: ","},
		},
		output: &structInternalObject{},
		pos:    []Position{{Byte: 9}},
		error: decodeError("unknown", Position{Byte: 9}, Comma, ",",
			reflect.TypeOf(""), nil),
	},

	// Pointer, map, slice, and array targets.
	"pointer-int": {
		mode: Json, input: `7`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "7"}},
		output: new(*int),
		expect: test.Ptr(test.Ptr(7)),
	},
	"pointer-struct": {
		mode: Json, input: `{"name":"value"}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "name"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: String, token: "value"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(*structInternalObject),
		expect: test.Ptr(&structInternalObject{Name: "value"}),
	},
	"pointer-slice": {
		mode: Json, input: `[1,2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new(*[]int),
		expect: test.Ptr(test.Ptr([]int{1, 2})),
	},
	"pointer-map": {
		mode: Json, input: `{"a":1}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(*map[string]int),
		expect: test.Ptr(test.Ptr(map[string]int{"a": 1})),
	},
	"nil-map-populated": {
		mode: Json, input: `{"values":{"a":1}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "values"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(structMapValues),
		expect: &structMapValues{Values: map[string]int{"a": 1}},
	},
	"nil-map-empty": {
		mode: Json, input: `{"values":{}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "values"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(structMapValues),
		expect: &structMapValues{Values: map[string]int{}},
	},
	"map-pointer-values": {
		mode: Json, input: `{"a":1}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectColon, typ: Colon, token: ":"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: ObjectEnd, token: "}"},
		},
		output: new(map[string]*int),
		expect: test.Ptr(map[string]*int{"a": test.Ptr(1)}),
	},
	"nil-slice-populated": {
		mode: Json, input: `[1,2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([]int),
		expect: test.Ptr([]int{1, 2}),
	},
	"nil-slice-empty": {
		mode: Json, input: `[]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([]int),
		expect: test.Ptr([]int{}),
	},
	"slice-pointer-elements": {
		mode: Json, input: `[1,2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([]*int),
		expect: test.Ptr([]*int{test.Ptr(1), test.Ptr(2)}),
	},
	"array-exact": {
		mode: Json, input: `[1,2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([2]int),
		expect: test.Ptr([2]int{1, 2}),
	},
	"array-state-error": {
		mode: Json, input: `[1:2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Colon, token: ":"},
		},
		pos:    []Position{{Byte: 3}},
		output: new([1]int),
		error: decodeError("array comma", Position{Byte: 3},
			Colon, ":", nil, nil),
	},
	"array-element-type-error": {
		mode: Json, input: `[true]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: True, token: "true"},
		},
		pos:    []Position{{Byte: 5}},
		output: new([1]int),
		error: decodeError("bool type", Position{Byte: 5},
			True, "true", reflect.TypeOf(int(0)), nil),
	},
	"array-short": {
		mode: Json, input: `[1]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([3]int),
		expect: test.Ptr([3]int{1}),
	},
	"array-long": {
		mode: Json, input: `[1,2,3]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "3"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new([2]int),
		expect: test.Ptr([2]int{1, 2}),
	},
	"pointer-array": {
		mode: Json, input: `[1,2]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Comma, token: ","},
			{ctx: ArrayValue, typ: Integer, token: "2"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
		output: new(*[2]int),
		expect: test.Ptr(test.Ptr([2]int{1, 2})),
	},
}

// TestDecDecodeScan tests Decode with scanner tokens.
func TestDecDecodeScan(t *testing.T) {
	test.Map(t, decDecodeTestCases).
		Filter(test.Not(DecodeMode(Mock))).
		// Filter(test.Pattern[decDecodeParams]("object-relaxed-i-key")).
		Run(func(t test.Test, param decDecodeParams) {
			// Given
			decoder := NewDecoder(NewWrapper(
				NewScanner(NewBuffer([]byte(param.input))),
				t, param.tokens, param.pos,
			)).SetMode(IgnoreCase | SkipDelims).SetMode(param.mode)
			output := copyShallow(param.output)

			// When
			var err error
			var panics any
			func() {
				defer func() {
					panics = recover()
				}()
				err = decoder.Decode(output)
			}()

			if panics == nil {
				test.Cast[*Wrapper](decoder.scanner).Validate()
			}

			// Then
			require.Nil(t, panics, "unexpected panic")
			if param.error != nil {
				assert.Equal(t, param.error, err)
			} else if param.check != nil {
				require.NoError(t, err)
				param.check(t, output)
			} else {
				require.NoError(t, err)
				assert.Equal(t, param.expect, output)
			}
		})
}

// TestDecDecodeMock tests `Decode` using context-checked scanner tokens.
func TestDecDecodeMock(t *testing.T) {
	test.Map(t, decDecodeTestCases).
		Run(func(t test.Test, param decDecodeParams) {
			// Given
			mocks := mock.NewMocks(t)
			setup := []func(*mock.Mocks) any{
				CallScannerGetMode(param.mode),
				CallScannerSetMode(param.mode),
			}
			for _, token := range param.tokens {
				setup = append(setup, CallScannerNext(token))
			}
			var decodeErr *DecodeError
			if errors.As(param.error, &decodeErr) {
				setup = append(setup, CallScannerPosition(Position{}))
			}
			mocks.Expect(mock.Chain(setup...))

			scanner := mock.Get(mocks, NewMockScanner)
			decoder := NewDecoder(scanner).SetMode(param.mode)
			output := copyShallow(param.output)

			// When
			var err error
			var panics any
			func() {
				defer func() {
					panics = recover()
				}()
				err = decoder.Decode(output)
			}()

			// Then
			require.Nil(t, panics, "unexpected panic")
			if param.error != nil {
				assert.Equal(t, clearPos(param.error), clearPos(err))
			} else if param.check != nil {
				require.NoError(t, err)
				param.check(t, output)
			} else {
				require.NoError(t, err)
				assert.Equal(t, param.expect, output)
			}
		})
}

type errorStringParams struct {
	err    *DecodeError
	expect string
}

var errorStringTestCases = map[string]errorStringParams{
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

func TestErrorString(t *testing.T) {
	test.Map(t, errorStringTestCases).
		Run(func(t test.Test, param errorStringParams) {
			assert.Equal(t, param.expect, param.err.Error())
		})
}

// decoderNextParams holds parameters for TestDecNextToken.
type decoderNextParams struct {
	mode   Mode
	input  string
	tokens []Token
	error  error
}

var decoderNextTestCases = map[string]decoderNextParams{
	// Flat primitive values.
	"zero": {
		mode:   Relaxed,
		tokens: []Token{{ctx: RootValue, typ: Zero}},
	},
	"string": {
		mode: Json, input: `"hello"`,
		tokens: []Token{{ctx: RootValue, typ: String, token: "hello"}},
	},
	"integer": {
		mode: Json, input: `42`,
		tokens: []Token{{ctx: RootValue, typ: Integer, token: "42"}},
	},
	"decimal": {
		mode: Json, input: `3.14`,
		tokens: []Token{{ctx: RootValue, typ: Decimal, token: "3.14"}},
	},
	"true": {
		mode: Json, input: `true`,
		tokens: []Token{{ctx: RootValue, typ: True, token: "true"}},
	},
	"false": {
		mode: Json, input: `false`,
		tokens: []Token{{ctx: RootValue, typ: False, token: "false"}},
	},
	"null": {
		mode: Json, input: `null`,
		tokens: []Token{{ctx: RootValue, typ: Null, token: "null"}},
	},

	// Strict numeric token types.
	"nan": {
		mode: Strict, input: `NaN`,
		tokens: []Token{{ctx: RootValue, typ: NaN, token: "NaN"}},
	},
	"infinity": {
		mode: Strict, input: `Infinity`,
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "Infinity"}},
	},
	"infinity-neg": {
		mode: Strict, input: `-Infinity`,
		tokens: []Token{{ctx: RootValue, typ: Infinity, token: "-Infinity"}},
	},
	"hex": {
		mode: Strict, input: `0xFF`,
		tokens: []Token{{ctx: RootValue, typ: HexaDecimal, token: "0xFF"}},
	},
	"complex": {
		mode: Extended, input: `3+4i`,
		tokens: []Token{{ctx: RootValue, typ: Complex, token: "3+4i"}},
	},

	// Empty collections.
	"empty-object": {
		mode: Json, input: `{}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
		},
	},
	"empty-array": {
		mode: Json, input: `[]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
	},

	// Simple object with one key.
	"object-int": {
		mode: Json, input: `{"a": 0}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: Integer, token: "0"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
		},
	},
	"object-omitted-values": {
		mode: Relaxed, input: `{"a":,"b":}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: Zero, token: ","},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectValue, typ: Zero, token: "}"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
		},
	},

	// Object with nested array value.
	"object-array": {
		mode: Json, input: `{"a": []}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
		},
	},

	// Object with two keys.
	"object-two-keys": {
		mode: Json, input: `{"a":{}, "b":{}}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectValue, typ: ObjectStart, token: "{"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
		},
	},

	// Array with one integer.
	"array-int": {
		mode: Json, input: `[10]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "10"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
	},

	// Deeply nested structure.
	"deep-nested": {
		mode: Json, input: `[[[[[[{"true":true}]]]]]]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "true"},
			{ctx: ObjectValue, typ: True, token: "true"},
			{ctx: RootValue, typ: ObjectEnd, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
	},

	// Mixed-type array.
	"array-mixed": {
		mode:  Json,
		input: `[{"a": 1,"b": 123.456, "c": null, "d": [1, -2, "three", true, false, ""]}]`,
		tokens: []Token{
			{ctx: RootValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
			{ctx: ObjectKey, typ: String, token: "b"},
			{ctx: ObjectValue, typ: Decimal, token: "123.456"},
			{ctx: ObjectKey, typ: String, token: "c"},
			{ctx: ObjectValue, typ: Null, token: "null"},
			{ctx: ObjectKey, typ: String, token: "d"},
			{ctx: ObjectValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
			{ctx: ArrayValue, typ: Integer, token: "-2"},
			{ctx: ArrayValue, typ: String, token: "three"},
			{ctx: ArrayValue, typ: True, token: "true"},
			{ctx: ArrayValue, typ: False, token: "false"},
			{ctx: ArrayValue, typ: String, token: ""},
			{ctx: ObjectKey, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ObjectEnd, token: "}"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
	},

	// Error cases: truncated input.
	"error-truncated-array": {
		mode: Json, input: `[`,
		tokens: []Token{{ctx: RootValue, typ: ArrayStart, token: "["}},
		error:  io.ErrUnexpectedEOF,
	},
	"error-truncated-object": {
		mode: Json, input: `{"":2`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: ""},
			{ctx: ObjectValue, typ: Integer, token: "2"},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-truncated-key": {
		mode: Json, input: `{"`,
		tokens: []Token{{ctx: RootValue, typ: ObjectStart, token: "{"}},
		error:  io.ErrUnexpectedEOF,
	},

	// Error cases: structural errors.
	"error-bad-key-type": {
		mode: Json, input: `{1: 1}`,
		tokens: []Token{{ctx: RootValue, typ: ObjectStart, token: "{"}},
		error: &DecodeError{
			Msg: "object key",
			Pos: Position{Byte: 2},
			Typ: Integer, Token: "1",
		},
	},
	"error-double-colon": {
		mode: Json, input: `{"test"::"input"}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "test"},
			{ctx: ObjectKey, typ: Colon, token: ":"},
		},
		error: &DecodeError{
			Msg: "object comma",
			Pos: Position{Byte: 16},
			Typ: String, Token: "input",
		},
	},
	"error-missing-comma-obj": {
		mode: Json, input: `{"a":1:}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
			{ctx: ObjectValue, typ: Integer, token: "1"},
		},
		error: &DecodeError{
			Msg: "object comma",
			Pos: Position{Byte: 7},
			Typ: Colon, Token: ":",
		},
	},
	"error-unexpected-comma-value": {
		mode: Json, input: `,`,
		error: &DecodeError{
			Msg:   "value",
			Pos:   Position{Byte: 1},
			Typ:   Comma,
			Token: ",",
		},
	},
	"error-unexpected-comma-array": {
		mode: Json, input: `[,]`,
		tokens: []Token{{ctx: ArrayValue, typ: ArrayStart, token: "["}},
		error: &DecodeError{
			Msg:   "array value",
			Pos:   Position{Byte: 2},
			Typ:   Comma,
			Token: ",",
		},
	},
	"error-missing-comma-arr": {
		mode: Json, input: `[1:]`,
		tokens: []Token{
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
		},
		error: &DecodeError{
			Msg:   "array comma",
			Pos:   Position{Byte: 3},
			Typ:   Colon,
			Token: ":",
		},
	},

	// Error cases: truncated at various state-machine steps.
	"error-empty-input": {
		mode:  Json,
		error: io.ErrUnexpectedEOF,
	},
	"error-truncated-colon": {
		mode: Json, input: `{"a"`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-missing-colon": {
		mode: Json, input: `{"a" 1}`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
		},
		error: &DecodeError{
			Msg:   "object colon",
			Pos:   Position{Byte: 6},
			Typ:   Integer,
			Token: "1",
		},
	},
	"error-truncated-value": {
		mode: Json, input: `{"a":`,
		tokens: []Token{
			{ctx: RootValue, typ: ObjectStart, token: "{"},
			{ctx: ObjectKey, typ: String, token: "a"},
		},
		error: io.ErrUnexpectedEOF,
	},
	"error-truncated-arr-value": {
		mode: Json, input: `[1`,
		tokens: []Token{
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: Integer, token: "1"},
		},
		error: io.ErrUnexpectedEOF,
	},

	// Array nested in array (exercises stateArrayValue case !inObj).
	"nested-array": {
		mode: Json, input: `[[]]`,
		tokens: []Token{
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayStart, token: "["},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
			{ctx: ArrayValue, typ: ArrayEnd, token: "]"},
		},
	},
}

func TestDecoderNext(t *testing.T) {
	test.Map(t, decoderNextTestCases).
		Run(func(t test.Test, param decoderNextParams) {
			// Given
			decoder := NewDecoderBuffer([]byte(param.input)).
				SetMode(param.mode)

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

//
// Benchmarks
//

// mapIntParams holds parameters for map[int] decoding benchmarks.
type mapIntParams struct {
	input string
}

// mapIntTestCases defines test cases for testing map[string]int decoding
// benchmarks.
var mapIntTestCases = map[string]mapIntParams{
	"flat-int-map": {
		input: `{"a":97,"b":98,"c":99,"d":100,"e":101,"f":102,"g":103}`,
	},
}

// benchReader is an interface that combines io.ReadSeeker and a Size method
// for benchmarking.
type benchReader interface {
	io.ReadSeeker
	Size() int64
}

// setupStringReader creates a benchReader from a string input.
func setupStringReader(_ test.Reporter, param mapIntParams) *strings.Reader {
	return strings.NewReader(param.input)
}

// setupFileReader creates a benchReader from a file path specified in the test
// parameters. It uses a helper function getFixtureReader to read the file
// contents into a bytes.Reader.
func setupFileReader(t test.Reporter, param fileParams) *bytes.Reader {
	return getFixtureReader(t, param.name)
}

// checkFileTokens is a helper function to verify the number of tokens read in
// the benchmarks. It compares the expected token count from the test parameters
// with the actual token count and fails the benchmark if they do not match.
func checkFileTokens(b *testing.B, param fileParams, tokens int) {
	if tokens != param.tokens-param.delims {
		b.Fatalf("expected %v tokens, got %v",
			param.tokens-param.delims, tokens)
	}
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
			var buffer [8 << 12]byte
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
			dec := NewDecoderSource(reader, buffer)

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
			dec := NewDecoderSource(reader, buffer)

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

// BenchmarkDecoderToken benchmarks the Token method, which returns Go values for
// JSON tokens. This measures the full decoding performance, including
// scanning, tokenization, and value conversion.
func BenchmarkDecoderToken(b *testing.B) {
	var tokens int

	benchmarkDecoder(b, "gojson", fileTestCase, setupFileReader,
		func(reader *bytes.Reader, buffer []byte) (int, error) {
			dec := NewDecoderSource(reader, buffer)
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
