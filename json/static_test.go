package json

import (
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkrop/go-testing/test"
)

type unmarshalObject struct {
	Name  string `json:"name"`
	Count int
}

type unmarshalParams struct {
	input  string
	output any
	expect any
	error  error
	check  func(t test.Test, output any)
}

func unmarshalError(
	msg string, pos Position, typ byte, token string, t reflect.Type,
) error {
	return &DecodeError{
		Msg:   msg,
		Pos:   pos,
		Typ:   typ,
		Token: token,
		Type:  t,
	}
}

var unmarshalTestCases = map[string]unmarshalParams{
	// Successful decoding.
	"bool": {
		input: `true`, output: new(bool), expect: test.Ptr(true),
	},
	"hex-int": {
		input: `0x10`, output: new(int), expect: test.Ptr(16),
	},
	"complex128": {
		input: `1+2i`, output: new(complex128),
		expect: test.Ptr(complex128(1 + 2i)),
	},
	"nan-float64": {
		input: `NaN`, output: new(float64),
		check: func(t test.Test, output any) {
			value := test.Cast[*float64](output)

			assert.True(t, math.IsNaN(*value))
		},
	},
	"null-map": {
		input: `null`, output: test.Ptr(map[string]int{"a": 1}),
		check: func(t test.Test, output any) {
			value := test.Cast[*map[string]int](output)

			assert.Nil(t, *value)
		},
	},
	"case-insensitive-struct": {
		input:  `{"NAME":"alpha","count":2}`,
		output: &unmarshalObject{},
		expect: &unmarshalObject{},
	},

	// Error handling.
	"nil-interface": {
		input: `true`, output: nil,
		error: unmarshalError("invalid", Position{}, EOF, "", nil),
	},
	"nil-pointer": {
		input: `true`, output: (*int)(nil),
		error: unmarshalError(
			"nil", Position{}, EOF, "", reflect.TypeOf((*int)(nil)),
		),
	},
	"non-pointer": {
		input: `true`, output: 0,
		error: unmarshalError(
			"no-pointer", Position{}, EOF, "", reflect.TypeOf(0),
		),
	},
	"type-mismatch": {
		input: `true`, output: new(int),
		error: unmarshalError(
			"bool type", Position{Byte: 4}, True, "true",
			reflect.TypeOf(0),
		),
	},
	"empty-input": {
		output: new(bool),
		error:  io.ErrUnexpectedEOF,
	},
	"missing-colon": {
		input: `{"name" 1}`, output: &map[string]int{},
		error: &DecodeError{
			Msg:   "object colon",
			Pos:   Position{Byte: 9},
			Typ:   Integer,
			Token: "1",
		},
	},
}

// TestUnmarshal tests the Unmarshal helper at the API boundary.
func TestUnmarshal(t *testing.T) {
	test.Map(t, unmarshalTestCases).
		Run(func(t test.Test, param unmarshalParams) {
			// When
			err := Unmarshal([]byte(param.input), param.output)

			// Then
			if param.error != nil {
				assert.Equal(t, param.error, err)

				return
			}

			require.NoError(t, err)
			if param.check != nil {
				param.check(t, param.output)

				return
			}

			assert.Equal(t, param.expect, param.output)
		})
}
