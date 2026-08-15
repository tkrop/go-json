package json

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FIXME: As it turned out, in v1 folding names is by default not dropping
// hyphens and underscores, while in v2 it does as soon as case-insensitive
// matching is enabled. This behavior can in v2 be reverted using the option
// MatchCaseSensitiveDelimiter(true).

// mapFields creates a map of field names to field indices for the given struct
// type `typ`. It considers the `caseIgnore` parameter to determine whether to
// ignore case when mapping field names. The function handles struct tags and
// exported fields, and it also supports folding names by removing hyphens and
// underscores and converting to lowercase.
func mapFields(typ reflect.Type, caseIgnore bool) map[string]int {
	fields := map[string]int{}

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		name, fieldCaseIgnore := mapFieldName(field)
		if name == "" {
			continue
		}

		if _, exists := fields[name]; !exists {
			fields[name] = idx + 1
		}

		if !caseIgnore && !fieldCaseIgnore {
			continue
		}

		folded := foldName(name)
		if folded == "" {
			continue
		}

		if folded == name {
			field := fields[name]
			if field < 0 {
				continue
			}

			if field == idx+1 {
				fields[name] = -field
			}

			continue
		}

		if _, exists := fields[folded]; !exists {
			fields[folded] = -(idx + 1)
		}
	}

	return fields
}

// mapFieldName extracts the JSON field name and case-ignore option from a
// struct field. It returns the field name and a boolean indicating whether
// case should be ignored for this field. If the field is unexported or has a
// JSON tag of "-", it returns an empty string and false.
func mapFieldName(field reflect.StructField) (string, bool) {
	if !field.IsExported() {
		return "", false
	}

	name := field.Name
	caseIgnore := false
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}

	if tag != "" {
		option := false
		for token := range strings.SplitSeq(tag, ",") {
			if !option {
				option = true
				if token == "" {
					continue
				}
				name = token
				continue
			}

			if token == "case:ignore" {
				caseIgnore = true
			}
		}
	}

	return name, caseIgnore
}

// foldName removes hyphens and underscores from the input name and converts
// it to lowercase. This is used for case-insensitive field name mapping.
func foldName(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))

	for _, char := range name {
		if char == '-' || char == '_' {
			continue
		}

		builder.WriteRune(unicode.ToLower(char))
	}

	return builder.String()
}

// FoldNameExact performs a case-insensitive fold of the input string `s` using
// the SimpleFold function. It finds the canonical minimum rune in the simple
// fold cycle for each rune in the string, effectively normalizing the string
// for case-insensitive comparisons.
func FoldNameExact(s string) string {
	return strings.Map(func(r rune) rune {
		// Cycle the SimpleFold ring to find the canonical minimum rune.
		min := r
		for curr := unicode.SimpleFold(r); curr != r; curr = unicode.
			SimpleFold(curr) {
			if curr < min {
				min = curr
			}
		}
		return min
	}, s)
}

// FoldNameFast performs a fast case-insensitive fold of the input string `s`.
// It first checks for ASCII uppercase letters and non-ASCII runes. If the
// string contains only ASCII characters, it uses a stack buffer for efficient
// lowercase conversion. If the string contains non-ASCII characters, it falls
// back to a full Unicode fold using the SimpleFold function.
//
//nolint:gocognit,funlen // prioritize performance.
//revive:disable-next-line:cognitive-complexity // prioritize performance.
//revive:disable-next-line:function-length // prioritize performance.
func FoldNameFast(s string) string {
	hasUpper := false
	hasNonASCII := false

	// Step 1: scan for ASCII uppercase or non-ASCII runes.
	for i := range s {
		b := s[i]
		if b >= utf8.RuneSelf {
			hasNonASCII = true
			break
		}
		if 'A' <= b && b <= 'Z' {
			hasUpper = true
		}
	}

	// Return original string if already lowercased ASCII
	if !hasUpper && !hasNonASCII {
		return s
	}

	// Step 2: ASCII Fast-Path with Stack Buffer Allocation
	if !hasNonASCII {
		var stack [64]byte
		var b []byte
		if len(s) <= len(stack) {
			b = stack[:0]
		} else {
			b = make([]byte, 0, len(s))
		}

		for i := range s {
			c := s[i]
			if 'A' <= c && c <= 'Z' {
				c |= 0x20 // Bitwise lowercase transformation.
			}
			b = append(b, c)
		}
		return string(b)
	}

	// Step 3: full unicode fallback using simple fold ring.
	var stack [64]byte
	var b []byte
	if len(s) <= len(stack) {
		b = stack[:0]
	} else {
		b = make([]byte, 0, len(s))
	}

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size

		if r < utf8.RuneSelf {
			c := s[i-size]
			if 'A' <= c && c <= 'Z' {
				c |= 0x20
			}
			b = append(b, c)
		} else {
			// Find canonical minimum rune in simple fold cycle.
			min := r
			for curr := unicode.SimpleFold(r); curr != r; curr = unicode.SimpleFold(curr) {
				if curr < min {
					min = curr
				}
			}
			b = utf8.AppendRune(b, min)
		}
	}

	return string(b)
}
