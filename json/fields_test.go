package json_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tkrop/go-json/json"
	"github.com/tkrop/go-testing/test"
)

// mapFieldsLocal defines a struct with various field tags for testing.
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

// mapFieldsGlobal defines a struct with global field tags for testing.
type mapFieldsGlobal struct {
	First string `json:"Dup"`
	Dup   string
	Lower string `json:"mixed"`
	Mixed string `json:"Mixed"`
	Plain string
}

// mapFieldsFoldEmpty defines a struct with an empty foldable field tag for testing.
type mapFieldsFoldEmpty struct {
	//revive:disable-next-line:struct-tag
	Empty string `json:"-_,case:ignore"`
	Plain string
}

// mapFieldsDelimiters defines a struct with delimited field tags for testing.
type mapFieldsDelimiters struct {
	Name string `json:"first-name"`
}

// mapFieldsPrimaryCollision defines a struct with primary field tag collisions for testing.
type mapFieldsPrimaryCollision struct {
	Delimited string `json:"first-name"`
	Primary   string `json:"firstname"`
}

// mapFieldsAliasCollision defines a struct with alias field tag collisions for testing.
type mapFieldsAliasCollision struct {
	Dash       string `json:"first-name"`
	Underscore string `json:"first_name"`
}

// _ is used to ensure that mapFieldsLocal can be instantiated without errors.
// This prevents the compiler from complaining about an unused type.
var _ = mapFieldsLocal{hidden: ""}

// fieldsLookupParams defines the parameters for a field lookup test case.
type fieldsLookupParams struct {
	typ         reflect.Type
	mode        json.Mode
	name        string
	expectIndex int
	expectFound bool
}

// fieldsLookupTestCases defines test cases for looking up field indexes in a FieldsMap.
var fieldsLookupTestCases = map[string]fieldsLookupParams{
	"field-name-primary": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "name",
		expectIndex: 2,
		expectFound: true,
	},
	"field-name-case-sensitive": {
		typ:  reflect.TypeOf(mapFieldsLocal{}),
		name: "NAME",
	},
	"field-name-case-ignore-primary": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "lower",
		expectIndex: 3,
		expectFound: true,
	},
	"field-name-case-ignore-folded": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "lOwEr",
		expectIndex: 3,
		expectFound: true,
	},
	"field-name-empty-tag": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "Mixed",
		expectIndex: 4,
		expectFound: true,
	},
	"field-name-empty-tag-case-ignore": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "mIxEd",
		expectIndex: 4,
		expectFound: true,
	},
	"field-name-delimited-case-ignore": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "fIrSt-NaMe",
		expectIndex: 5,
		expectFound: true,
	},
	"field-name-plain": {
		typ:         reflect.TypeOf(mapFieldsLocal{}),
		name:        "Plain",
		expectIndex: 6,
		expectFound: true,
	},
	"field-name-plain-case-sensitive": {
		typ:  reflect.TypeOf(mapFieldsLocal{}),
		name: "plain",
	},
	"field-name-unexported-excluded": {
		typ:  reflect.TypeOf(mapFieldsLocal{}),
		name: "hidden",
	},
	"field-name-dash-tag-excluded": {
		typ:  reflect.TypeOf(mapFieldsLocal{}),
		name: "Skip",
	},
	"global-case-ignore-primary": {
		typ:         reflect.TypeOf(mapFieldsGlobal{}),
		mode:        json.IgnoreCase,
		name:        "Dup",
		expectIndex: 0,
		expectFound: true,
	},
	"global-case-ignore-folded": {
		typ:         reflect.TypeOf(mapFieldsGlobal{}),
		mode:        json.IgnoreCase,
		name:        "dUp",
		expectIndex: 0,
		expectFound: true,
	},
	"global-case-ignore-primary-collision": {
		typ:         reflect.TypeOf(mapFieldsGlobal{}),
		mode:        json.IgnoreCase,
		name:        "Mixed",
		expectIndex: 3,
		expectFound: true,
	},
	"global-case-ignore-alias-collision": {
		typ:         reflect.TypeOf(mapFieldsGlobal{}),
		mode:        json.IgnoreCase,
		name:        "mIxEd",
		expectIndex: 2,
		expectFound: true,
	},
	"global-case-ignore-plain": {
		typ:         reflect.TypeOf(mapFieldsGlobal{}),
		mode:        json.IgnoreCase,
		name:        "PLAIN",
		expectIndex: 4,
		expectFound: true,
	},
	"global-case-ignore-miss": {
		typ:  reflect.TypeOf(mapFieldsGlobal{}),
		mode: json.IgnoreCase,
		name: "missing",
	},
	"delimiter-primary": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.SkipDelims,
		name:        "first-name",
		expectIndex: 0,
		expectFound: true,
	},
	"delimiter-alias": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.SkipDelims,
		name:        "firstname",
		expectIndex: 0,
		expectFound: true,
	},
	"delimiter-normalized-alias": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.SkipDelims,
		name:        "first_name",
		expectIndex: 0,
		expectFound: true,
	},
	"delimiter-case-sensitive-miss": {
		typ:  reflect.TypeOf(mapFieldsDelimiters{}),
		mode: json.SkipDelims,
		name: "FIRST-NAME",
	},
	"delimiter-fold-empty-primary": {
		typ:         reflect.TypeOf(mapFieldsFoldEmpty{}),
		mode:        json.SkipDelims,
		name:        "-_",
		expectIndex: 0,
		expectFound: true,
	},
	"delimiter-empty-alias-omitted": {
		typ:  reflect.TypeOf(mapFieldsFoldEmpty{}),
		mode: json.SkipDelims,
	},
	"case-and-delimiter-primary": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.IgnoreCase | json.SkipDelims,
		name:        "first-name",
		expectIndex: 0,
		expectFound: true,
	},
	"case-and-delimiter-normalized": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.IgnoreCase | json.SkipDelims,
		name:        "first_name",
		expectIndex: 0,
		expectFound: true,
	},
	"case-and-delimiter-folded": {
		typ:         reflect.TypeOf(mapFieldsDelimiters{}),
		mode:        json.IgnoreCase | json.SkipDelims,
		name:        "FiRsT_NaMe",
		expectIndex: 0,
		expectFound: true,
	},
	"case-and-delimiter-miss": {
		typ:  reflect.TypeOf(mapFieldsDelimiters{}),
		mode: json.IgnoreCase | json.SkipDelims,
		name: "missing-name",
	},
	"duplicate-primary-first-wins": {
		typ: reflect.StructOf([]reflect.StructField{
			{Name: "First", Type: reflect.TypeOf(""), Tag: `json:"lower,case:ignore"`},
			{Name: "Second", Type: reflect.TypeOf(""), Tag: `json:"lower,case:ignore,string"`},
			{Name: "Plain", Type: reflect.TypeOf("")},
		}),
		name:        "lower",
		expectIndex: 0,
		expectFound: true,
	},
	"duplicate-case-fold-first-wins": {
		typ: reflect.StructOf([]reflect.StructField{
			{Name: "First", Type: reflect.TypeOf(""), Tag: `json:"lower,case:ignore"`},
			{Name: "Second", Type: reflect.TypeOf(""), Tag: `json:"lower,case:ignore,string"`},
			{Name: "Plain", Type: reflect.TypeOf("")},
		}),
		name:        "LoWeR",
		expectIndex: 0,
		expectFound: true,
	},
	"primary-takes-precedence-over-alias": {
		typ:         reflect.TypeOf(mapFieldsPrimaryCollision{}),
		mode:        json.SkipDelims,
		name:        "firstname",
		expectIndex: 1,
		expectFound: true,
	},
	"primary-name-with-delimiter": {
		typ:         reflect.TypeOf(mapFieldsPrimaryCollision{}),
		mode:        json.SkipDelims,
		name:        "first-name",
		expectIndex: 0,
		expectFound: true,
	},
	"first-alias-wins": {
		typ:         reflect.TypeOf(mapFieldsAliasCollision{}),
		mode:        json.SkipDelims,
		name:        "firstname",
		expectIndex: 0,
		expectFound: true,
	},
	"primary-name-wins-over-normalized-alias": {
		typ:         reflect.TypeOf(mapFieldsAliasCollision{}),
		mode:        json.SkipDelims,
		name:        "first_name",
		expectIndex: 1,
		expectFound: true,
	},
	"first-alias-wins-after-normalization": {
		typ:         reflect.TypeOf(mapFieldsAliasCollision{}),
		mode:        json.SkipDelims,
		name:        "first__name",
		expectIndex: 0,
		expectFound: true,
	},
	"not-found": {
		typ:  reflect.TypeOf(mapFieldsDelimiters{}),
		mode: json.SkipDelims,
		name: "missing",
	},
}

// TestFieldsLookup checks zero-based field indexes and misses.
func TestFieldsLookup(t *testing.T) {
	test.Map(t, fieldsLookupTestCases).
		Run(func(t test.Test, param fieldsLookupParams) {
			// Given
			fields := json.NewFields(param.typ, param.mode)

			// When
			index, found := fields.Lookup(param.name)

			// Then
			assert.Equal(t, param.expectIndex, index)
			assert.Equal(t, param.expectFound, found)
		})
}

// skipDelimsParams test parameters for input and expected output.
type skipDelimsParams struct {
	input  string
	expect string
}

// skipDelimsTestCases test cases for input strings with various
// delimiters.
var skipDelimsTestCases = map[string]skipDelimsParams{
	"empty":      {},
	"without":    {input: "firstName", expect: "firstName"},
	"hyphen":     {input: "first-name", expect: "firstname"},
	"underscore": {input: "first_name", expect: "firstname"},
	"both":       {input: "first-_name", expect: "firstname"},
	"multiple":   {input: "first-__--name", expect: "firstname"},
}

// TestSkipDelims checks delimiter removal and unchanged-name behavior.
func TestSkipDelims(t *testing.T) {
	test.Map(t, skipDelimsTestCases).
		Run(func(t test.Test, param skipDelimsParams) {
			// When/Then
			assert.Equal(t, param.expect, json.SkipDelimiters(param.input))
		})
}

// foldNameParams test parameters for input and expected output.
type foldNameParams struct {
	input  string
	expect string
}

// foldNameTestCases test cases for various input strings, including ASCII,
// Unicode, and delimiters.
var foldNameTestCases = map[string]foldNameParams{
	"empty":                    {expect: ""},
	"ascii-lower":              {input: "name", expect: "NAME"},
	"ascii-upper":              {input: "NAME", expect: "NAME"},
	"ascii-symbols-and-digits": {input: "JSON_2.0-!", expect: "JSON_2.0-!"},
	"ascii-lower-boundary": {
		input:  strings.Repeat("a", 64),
		expect: strings.Repeat("A", 64),
	},
	"ascii-lower-over-boundary": {
		input:  strings.Repeat("a", 65),
		expect: strings.Repeat("A", 65),
	},
	"unicode-lower":           {input: "ä", expect: "Ä"},
	"mixed-ascii-and-unicode": {input: "sÄ", expect: "SÄ"},
	"ascii-after-unicode":     {input: "Äs", expect: "ÄS"},
	"unicode-without-case":    {input: "😀", expect: "😀"},
	"unicode-over-boundary": {
		input:  strings.Repeat("ä", 33),
		expect: strings.Repeat("Ä", 33),
	},
	"greek-sigma":       {input: "Σ", expect: "Σ"},
	"greek-final-sigma": {input: "ς", expect: "Σ"},
	"greek-small-sigma": {input: "σ", expect: "Σ"},
	"long-s":            {input: "ſ", expect: "S"},
	"kelvin-sign":       {input: "K", expect: "K"},
	"delimiters": {
		input: "a-b_c", expect: "A-B_C",
	},
}

// TestFoldNameExact checks that the exact fold matches the exact Unicode fold.
func TestFoldNameExact(t *testing.T) {
	test.Map(t, foldNameTestCases).
		Run(func(t test.Test, param foldNameParams) {
			// When/Then
			assert.Equal(t, param.expect, json.FoldNameExact(param.input))
		})
}

// TestFoldNameFast checks that the fast fold matches the exact Unicode fold.
func TestFoldNameFast(t *testing.T) {
	test.Map(t, foldNameTestCases).
		Run(func(t test.Test, param foldNameParams) {
			// When/Then
			assert.Equal(t, param.expect, json.FoldNameFast(param.input))
		})
}
