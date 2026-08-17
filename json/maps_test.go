package json

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tkrop/go-testing/test"
)

// FIXME: Improve AI generated test cases and document.

type mapFieldsParams struct {
	call   func() reflect.Type
	ignore bool
	expect map[string]int
}

type mapFieldsLocal struct {
	hidden string
	Skip   string `json:"-"`
	Name   string `json:"name"`
	//revive:disable-next-line:struct-tag
	Lower string `json:"lower,case:ignore"`
	//revive:disable-next-line:struct-tag
	Mixed string `json:",case:ignore"`
	//revive:disable-next-line:struct-tag
	Dash  string `json:"first-name,case:ignore"`
	Plain string
}

type mapFieldsGlobal struct {
	First string `json:"Dup"`
	Dup   string
	Lower string `json:"mixed"`
	Mixed string `json:"Mixed"`
	Plain string
}

type mapFieldsFoldEmpty struct {
	//revive:disable-next-line:struct-tag
	Empty string `json:"-_,case:ignore"`
	Plain string
}

var _ = mapFieldsLocal{hidden: ""}

var mapFieldsTestCases = map[string]mapFieldsParams{
	"field-tags": {
		call: func() reflect.Type {
			return reflect.TypeOf(mapFieldsLocal{})
		},
		ignore: false,
		expect: map[string]int{
			"name":       3,
			"lower":      -4,
			"Mixed":      5,
			"mixed":      -5,
			"first-name": 6,
			"firstname":  -6,
			"Plain":      7,
		},
	},
	"global-case-ignore": {
		call: func() reflect.Type {
			return reflect.TypeOf(mapFieldsGlobal{})
		},
		ignore: true,
		expect: map[string]int{
			"Dup":   1,
			"dup":   -1,
			"mixed": -3,
			"Mixed": 4,
			"Plain": 5,
			"plain": -5,
		},
	},
	"fold-empty": {
		call: func() reflect.Type {
			return reflect.TypeOf(mapFieldsFoldEmpty{})
		},
		ignore: false,
		expect: map[string]int{
			"-_":    1,
			"Plain": 2,
		},
	},
	"fold-duplicate-lower": {
		call: func() reflect.Type {
			return reflect.StructOf([]reflect.StructField{
				{
					Name: "First",
					Type: reflect.TypeOf(""),
					Tag:  `json:"lower,case:ignore"`,
				},
				{
					Name: "Second",
					Type: reflect.TypeOf(""),
					Tag:  `json:"lower,case:ignore,string"`,
				},
				{
					Name: "Plain",
					Type: reflect.TypeOf(""),
				},
			})
		},
		ignore: false,
		expect: map[string]int{
			"lower": -1,
			"Plain": 3,
		},
	},
}

// TestMapFields validates field-name extraction and folded field mapping.
func TestMapFields(t *testing.T) {
	test.Map(t, mapFieldsTestCases).
		Run(func(t test.Test, param mapFieldsParams) {
			// When
			fields := mapFields(param.call(), param.ignore)

			// Then
			assert.Equal(t, param.expect, fields)
		})
}
