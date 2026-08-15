package json

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Note: As it turned out for encode/json: v1 folding names is by default not
// dropping hyphens and underscores, while v2 does this when case-insensitive
// matching is enabled. This behavior can in v2 be reverted using the option
// MatchCaseSensitiveDelimiter(true).

// Fields maps JSON field names to struct field indexes.
type Fields struct {
	// caseIgnore indicates whether Unicode case folding is enabled.
	caseIgnore bool
	// delimSkip indicates whether hyphens and underscores should be ignored.
	delimSkip bool
	// foldNames indicates whether any field name required Unicode case
	// folding during the mapping of field names.
	foldNames bool
	// Fields stores the mapping of JSON field names to negative and positive
	// struct field one-based indexes - positive for primary names and negative
	// for alias mappings.
	Fields map[string]int
}

// NewFields builds primary field names and mode-dependent aliases for the
// given struct type and mode. The mode controls how field names are processed.
// If the mode `IgnoreCase` is enabled or a `case:ignore` tag is present,
// Unicode case folding and case-insensitive matching are applied. If the mode
// `SkipDelim` is enabled, hyphens and underscores are ignored. Primary names
// take precedence over aliases, while aliases are added using a first-wins
// strategy.
func NewFields(typ reflect.Type, mode Mode) *Fields {
	return (&Fields{
		caseIgnore: mode&IgnoreCase != 0,
		delimSkip:  mode&SkipDelims != 0,
		Fields:     make(map[string]int, typ.NumField()),
	}).initMap(typ)
}

// initMap initializes the a fields map with primary field names and aliases
// depending on mode, e.g. skipping delimiters or folding names, for the given
// struct type. It returns the Fields for chaining.
func (f *Fields) initMap(typ reflect.Type) *Fields {
	for index := range typ.NumField() {
		name, fieldCaseIgnore := MapFieldName(typ.Field(index))
		if name == "" {
			continue
		}

		fieldIndex := index + 1
		f.addPrimary(name, fieldIndex)

		foldName := f.caseIgnore || fieldCaseIgnore
		if foldName {
			f.foldNames = true
			folded := FoldNameFast(name)
			f.addAlias(folded, fieldIndex)
		}

		if f.delimSkip {
			delimited := SkipDelimiters(name)
			f.addAlias(delimited, fieldIndex)
			if foldName {
				foldedDelimited := FoldNameFast(delimited)
				f.addAlias(foldedDelimited, fieldIndex)
			}
		}
	}

	return f
}

// addPrimary adds a primary field name with the given index. It only adds the
// primary name if it does not already exist as a primary name.
func (f *Fields) addPrimary(name string, index int) {
	if mapped, exists := f.Fields[name]; !exists || mapped < 0 {
		f.Fields[name] = index
	}
}

// addAlias adds an alias for a field name with the given index. It only adds
// the alias if it does not already exist as a primary name or alias.
func (f *Fields) addAlias(name string, index int) {
	if name == "" {
		return
	} else if _, exists := f.Fields[name]; !exists {
		f.Fields[name] = -index
	}
}

// Lookup returns the zero-based struct field index associated with name.
func (f *Fields) Lookup(name string) (int, bool) {
	if index, ok := f.lookup(name); ok {
		return index, true
	}

	if f.delimSkip {
		delimited := SkipDelimiters(name)
		if index, ok := f.lookup(delimited); ok {
			return index, true
		} else if f.foldNames {
			folded := FoldNameFast(delimited)
			if index, ok := f.lookup(folded); ok {
				return index, true
			}
		}
	}

	if f.foldNames {
		folded := FoldNameFast(name)
		index, ok := f.lookup(folded)

		return index, ok
	}

	return 0, false
}

// lookup retrieves the one-based struct field index for the given name, if it
// exists.
func (f *Fields) lookup(name string) (int, bool) {
	index, ok := f.Fields[name]
	if !ok {
		return 0, false
	} else if index < 0 {
		index = -index
	}

	return index - 1, true
}

// MapFieldName extracts the JSON field name and case-ignore option from a
// struct field. It returns the field name and a boolean indicating whether
// case should be ignored for this field. If the field is unexported or has a
// JSON tag of "-", it returns an empty string and false.
func MapFieldName(field reflect.StructField) (string, bool) {
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

// SkipDelimiters removes '-' and '_' characters from the input string. It is
// used to generate alternative field names that ignore common delimiters.
func SkipDelimiters(name string) string {
	var builder strings.Builder
	skipped := false

	for i := range name {
		if name[i] == '-' || name[i] == '_' {
			if !skipped {
				builder.Grow(len(name))
				builder.WriteString(name[:i])
				skipped = true
			}

			continue
		}

		if skipped {
			builder.WriteByte(name[i])
		}
	}

	if !skipped {
		return name
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
//nolint:gocognit,funlen // prioritize speed.
//revive:disable-next-line:cognitive-complexity // prioritize speed.
//revive:disable-next-line:function-length // prioritize speed.
func FoldNameFast(s string) string {
	hasLower := false
	hasNonASCII := false

	// Step 1: scan for ASCII lowercase or non-ASCII runes.
	for i := range s {
		b := s[i]
		if b >= utf8.RuneSelf {
			hasNonASCII = true

			break
		}
		if 'a' <= b && b <= 'z' {
			hasLower = true
		}
	}

	// Return original string if already canonical ASCII.
	if !hasLower && !hasNonASCII {
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
			if 'a' <= c && c <= 'z' {
				c &^= 0x20
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
			if 'a' <= c && c <= 'z' {
				c &^= 0x20
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
