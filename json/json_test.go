package json_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adhocore/jsonc"
	jsonx "github.com/pkg/json"
	"github.com/stretchr/testify/assert"
	jsont "github.com/titanous/json5"
	"github.com/tkrop/go-testing/test"
	jsonf "github.com/yosuke-furukawa/json5/encoding/json5"
	"gopkg.in/yaml.v3"

	refl "github.com/tkrop/go-testing/reflect"
)

type object struct {
	Bool     bool          `json:"bool"`
	Int      int           `json:"int"`
	Uint     uint          `json:"uint"`
	Float    float64       `json:"float"`
	String   string        `json:"string"`
	Value    any           `json:"value"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"duration"`
}

type parserParams struct {
	input  string
	output any
	expect any
	error  error
}

var parserTestCases = map[string]parserParams{
	// Test basic types.
	"nil": {
		input:  `null`,
		output: &object{},
		expect: &object{},
	},
	"int": {
		input:  `42`,
		output: test.Ptr(0),
		expect: test.Ptr(42),
	},
	"uint": {
		input:  `42`,
		output: test.Ptr(uint(0)),
		expect: test.Ptr(uint(42)),
	},
	"float": {
		input:  `3.14`,
		output: test.Ptr(float64(0)),
		expect: test.Ptr(3.14),
	},
	"bool": {
		input:  `true`,
		output: test.Ptr(false),
		expect: test.Ptr(true),
	},
	"string": {
		input:  `"hello"`,
		output: test.Ptr(""),
		expect: test.Ptr("hello"),
	},
	"time": {
		input:  `"2024-01-02T15:04:05Z"`,
		output: &time.Time{},
		expect: test.Ptr(time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)),
	},
	"duration": {
		input:  `5400`,
		output: test.Ptr(time.Duration(0)),
		expect: test.Ptr(time.Hour + 30*time.Minute),
	},

	// Test composite types.
	"map": {
		input:  `{key: "value"}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value"},
	},
	"map nested": {
		input:  `{nested: {key: "value"}}`,
		output: &map[string]map[string]string{},
		expect: &map[string]map[string]string{
			"nested": {"key": "value"},
		},
	},
	"slice": {
		input:  `[1,2,3,4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 3, 4, 5},
	},
	"object": {
		input: `{bool: true,int: 42,uint: 42,float: 3.14,string: "hello",
			value: {nested: "object"},time: "2025-11-14T14:15:10Z"}`,
		output: &object{},
		expect: &object{
			Bool:   true,
			Int:    42,
			Uint:   42,
			Float:  3.14,
			String: "hello",
			Value:  map[string]any{"nested": "object"},
			Time:   time.Date(2025, 11, 14, 14, 15, 10, 0, time.UTC),
		},
	},
}

var json5TestCases = map[string]parserParams{
	// Test basic types (success).

	// Test composite types (success).
	"map": {
		input:  `{key:"value"}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value"},
	},
	"map-double": {
		input:  `{key:"value",key:"next"}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "next"},
	},
	"map-nested": {
		input:  `{nested:{key:"value"}}`,
		output: &map[string]map[string]string{},
		expect: &map[string]map[string]string{
			"nested": {"key": "value"},
		},
	},
	"slice": {
		input:  `[1,2,3,4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 3, 4, 5},
	},
	"object": {
		input: `{bool:true,int:42,uint:42,float:3.14,string:"hello",
		    value:{nested:"object"},time:"2025-11-14T14:15:10Z"}`,
		output: &object{},
		expect: &object{
			Bool:   true,
			Int:    42,
			Uint:   42,
			Float:  3.14,
			String: "hello",
			Value:  map[string]any{"nested": "object"},
			Time:   time.Date(2025, 11, 14, 14, 15, 10, 0, time.UTC),
		},
	},
}

var jsontTestCases = map[string]parserParams{
	// Test basic types (failure).
	"duration": {
		input:  `"1h30m"`,
		output: test.Ptr(time.Duration(0)),
		expect: test.Ptr(time.Duration(0)),
		error: &jsont.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(time.Duration(0)), Offset: 7,
		},
	},
	"complex-failure-1": {
		input:  `64`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &jsont.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(complex128(0)), Offset: 2,
		},
	},
	"complex-failure-2": {
		input:  `64+2i`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &jsont.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(complex128(0)), Offset: 2,
		},
	},

	// Test composite types (failure).
	"map-failure": {
		input:  `{key:"value",num:42}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value"},
		error: &jsont.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(""), Offset: 19,
		},
	},
	"slice-failure": {
		input:  `[1,2,"invalid",4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 0, 4, 5},
		error: &jsont.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(0), Offset: 14,
		},
	},
}

var jsonfTestCases = map[string]parserParams{
	// Test basic types (failure).
	"duration": {
		input:  `"1h30m"`,
		output: test.Ptr(time.Duration(0)),
		expect: test.Ptr(time.Duration(0)),
		error: &jsonf.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(time.Duration(0)),
		},
	},
	"complex-failure-1": {
		input:  `64`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &jsonf.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(complex128(0)),
		},
	},
	"complex-failure-2": {
		input:  `64+2i`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &jsonf.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(complex128(0)),
		},
	},

	// Test composite types (failure).
	"map-failure": {
		input:  `{key:"value",num:42}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value"},
		error: &jsonf.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(""),
		},
	},
	"slice-failure": {
		input:  `[1,2,"invalid",4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 0, 4, 5},
		error: &jsonf.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(0),
		},
	},
}

var jsoncTestCases = map[string]parserParams{
	// Test basic types (failure).
	"duration": {
		input:  `"1h30m"`,
		output: test.Ptr(time.Duration(0)),
		expect: test.Ptr(time.Duration(0)),
		error: &json.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(time.Duration(0)), Offset: 7,
		},
	},
	"complex-failure-1": {
		input:  `64`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &json.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(complex128(0)), Offset: 2,
		},
	},
	"complex-failure-2": {
		input:  `64+2i`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: refl.NewAccessor(&json.SyntaxError{Offset: 4}).
			Set("msg", "invalid character 'i' after top-level value").Build(),
	},

	// Test composite types (failure).
	"map-failure": {
		input:  `{key:"value",num:42}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value", "num": ""},
		error: &json.UnmarshalTypeError{
			Value: "number", Type: reflect.TypeOf(""), Offset: 23,
		},
	},
	"slice-failure": {
		input:  `[1,2,"invalid",4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 0, 4, 5},
		error: &json.UnmarshalTypeError{
			Value: "string", Type: reflect.TypeOf(0), Offset: 14,
		},
	},
}

var jsonyTestCases = map[string]parserParams{
	// Test basic types (success).
	"duration": {
		input:  `"1h30m"`,
		output: test.Ptr(time.Duration(0)),
		expect: test.Ptr(time.Hour + 30*time.Minute),
	},

	// Test composite types (success).
	"map-number": {
		input:  `{key: "value", num: 42}`,
		output: &map[string]string{},
		expect: &map[string]string{"key": "value", "num": "42"},
	},

	// Test basic types (failure).
	"complex": {
		input:  `64`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &yaml.TypeError{Errors: []string{
			"line 1: cannot unmarshal !!int `64` into complex128",
		}},
	},
	"complex-failure": {
		input:  `64+2i`,
		output: test.Ptr(complex128(0)),
		expect: test.Ptr(complex128(0)),
		error: &yaml.TypeError{Errors: []string{
			"line 1: cannot unmarshal !!str `64+2i` into complex128",
		}},
	},

	// Test composite types (failure).
	"slice-failure": {
		input:  `[1,2,invalid,4,5]`,
		output: &[]int{},
		expect: &[]int{1, 2, 4, 5},
		error: &yaml.TypeError{Errors: []string{
			"line 1: cannot unmarshal !!str `invalid` into int",
		}},
	},
	"map-double-failure": {
		input:  `{key: "value",key: "next"}`,
		output: &map[string]string{},
		expect: &map[string]string{},
		error: &yaml.TypeError{Errors: []string{
			"line 1: mapping key \"key\" already defined at line 1",
		}},
	},
	"object": {
		input: `{bool: true,int: 42,uint: 42,float: 3.14,
			string: "hello",value: {nested: "object"}}`,
		output: &object{},
		expect: &object{
			Bool:   true,
			Int:    42,
			Uint:   42,
			Float:  3.14,
			String: "hello",
			Value:  map[string]any{"nested": "object"},
		},
	},
}

// TODO: fix test cases.
func XTestJSONX(t *testing.T) {
	test.Map(t, parserTestCases).
		Run(func(t test.Test, param parserParams) {
			// When
			err := jsonx.NewDecoder(strings.NewReader(param.input)).
				Decode(param.output)

			s := jsonx.NewScanner(strings.NewReader(param.input))

			s.Next()

			// Then
			assert.Equal(t, param.error, err)
			assert.Equal(t, param.expect, param.output)
		})
}

func TestJSONT(t *testing.T) {
	test.Map(t, parserTestCases, json5TestCases, jsontTestCases).
		RunSeq(func(t test.Test, param parserParams) {
			// When
			err := jsont.NewDecoder(strings.NewReader(param.input)).
				Decode(param.output)

			// Then
			assert.Equal(t, param.error, err)
			assert.Equal(t, param.expect, param.output)
		})
}

func TestJSONF(t *testing.T) {
	test.Map(t, parserTestCases, json5TestCases, jsonfTestCases).
		RunSeq(func(t test.Test, param parserParams) {
			// When
			err := jsonf.NewDecoder(strings.NewReader(param.input)).
				Decode(param.output)

			// Then
			assert.Equal(t, param.error, err)
			assert.Equal(t, param.expect, param.output)
		})
}

func TestJSONC(t *testing.T) {
	test.Map(t, parserTestCases, json5TestCases, jsoncTestCases).
		RunSeq(func(t test.Test, param parserParams) {
			// When
			err := jsonc.New().
				Unmarshal([]byte(param.input), param.output)

			// Then
			assert.Equal(t, param.error, err)
			assert.Equal(t, param.expect, param.output)
		})
}

func TestJSONY(t *testing.T) {
	test.Map(t, parserTestCases, jsonyTestCases).
		Run(func(t test.Test, param parserParams) {
			// When
			err := yaml.Unmarshal([]byte(param.input), param.output)

			// Then
			assert.Equal(t, param.error, err)
			assert.Equal(t, param.expect, param.output)
		})
}
