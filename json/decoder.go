package json

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"unsafe"
)

// DecodeError represents an error that occurred during decoding of the input
// stream.
type DecodeError struct {
	// The error message.
	Msg string
	// The position in the input stream where the error occurred.
	Pos Position
	// The token type that caused the error.
	Typ byte
	// The token string that caused the error.
	Token string
	// The object type that caused the error.
	Type reflect.Type
	// The underlying error that caused the decode error, if any.
	Err error
}

// NewErrState creates a new decoder state error with the given message and
// decoder position.
func NewErrState(
	msg string, pos Position, typ byte, token []byte,
) *DecodeError {
	return &DecodeError{
		Msg:   msg,
		Pos:   pos,
		Typ:   typ,
		Token: viewString(token),
	}
}

// NewErrDecode creates a new decode error with the given message and decoder
// position.
func NewErrDecode(
	msg string, pos Position, typ byte, token []byte, t reflect.Type, err error,
) *DecodeError {
	return &DecodeError{
		Msg:   msg,
		Pos:   pos,
		Typ:   typ,
		Token: viewString(token),
		Type:  t,
		Err:   err,
	}
}

// Error returns the error message for the decode error.
func (e *DecodeError) Error() string {
	if e.Pos.Line == 0 && e.Pos.Char == 0 {
		return fmt.Sprintf("decode %s [type=%v, byte=%d, token=%q]",
			e.Msg, e.Type, e.Pos.Byte, e.Token)
	}
	return fmt.Sprintf("decode %s "+
		"[type=%v, byte=%d, line=%d, char=%d, token=%q]",
		e.Msg, e.Type, e.Pos.Byte, e.Pos.Line, e.Pos.Char, e.Token)
}

// A Decoder decodes JSON values from an input stream.
type Decoder struct {
	// The scanner used to tokenize the input.
	scanner Scanner
	// The stack to keep track of whether we are pushing a new object or array.
	stack []bool
	// The current state function.
	state func(*Decoder) (byte, []byte, error)
}

// NewDecoder creates a new default buffered decoder for the supplied reader.
func NewDecoder(r io.Reader) *Decoder {
	return NewDecoderBuffer(r, make([]byte, 8192))
}

// NewDecoderBuffer creates a new buffered decoder for the supplier reader
// using the provided buffer as working storage.
func NewDecoderBuffer(reader io.Reader, buffer []byte) *Decoder {
	return &Decoder{
		scanner: Scanner{
			reader: *NewReader(buffer[:0], reader),
		},
		state: (*Decoder).stateValue,
	}
}

//
// Stack management functions for the decoder. The stack is used to keep track
// of whether we are currently parsing an object or an array, which is necessary
// to ensure that delimiters are properly matched and to determine the next state
// function to call.
//
// The stack is a slice of bools, where true represents an object and false
// represents an array. The top of the stack is the last element of the slice.
// When we encounter a new object or array, we push a new state onto the stack.
// When we encounter a closing delimiter, we pop a state from the stack.
//
// The stack management functions are designed to be called by the state
// functions, which manage the decoder's state machine.
//

// push is a convenience method that pushes a new state onto the stack.
func (d *Decoder) push(v bool) {
	d.stack = append(d.stack, v)
}

// pop is a convenience method that pops a state from the stack.
func (d *Decoder) pop() bool {
	d.stack = d.stack[:len(d.stack)-1]
	if len(d.stack) == 0 {
		return false
	}
	return d.stack[len(d.stack)-1]
}

// len is a convenience method that returns the current stack length.
func (d *Decoder) len() int { return len(d.stack) }

// viewString converts a []byte to a string without copying. The caller must
// ensure that the []byte is not modified while the string is in use. This is
// used to avoid unnecessary allocations when parsing numbers.
//
// #nosec G103 -- only used for internal purposes.
func viewString(bytes []byte) string {
	return unsafe.String(unsafe.SliceData(bytes), len(bytes))
}

// Next returns the token type and a []byte referencing the next logical token
// in the stream. The []byte is only valid until Next is called again.  At the
// end of the input stream, Next returns EOF, nil, io.EOF.
//
// Next guarantees that the delimiters [ ] { } it returns are properly nested
// and matched. If Next encounters an unexpected delimiter in the input, it
// will return an error. Commas, colons, comments, and white space sequences
// are elided.
func (d *Decoder) Next() (byte, []byte, error) {
	return d.state(d)
}

// Token returns the next JSON token in the input stream. At the end of the
// input stream, Token returns nil, io.EOF.
//
// Token guarantees that the delimiters [ ] { } it returns are properly nested
// and matched: if Token encounters an unexpected delimiter in the input, it
// will return an error.
//
// The input stream consists of basic JSON values—bool, string, number, and
// null—along with delimiters [ ] { } of type json.Delim to mark the start and
// end of arrays and objects. Commas and colons are elided.
//
// Note: this API is provided for compatibility with the encoding/json package
// and carries a significant allocation cost. See `Next` for a more efficient
// API.
func (d *Decoder) Token() (json.Token, error) {
	typ, token, err := d.Next()
	if err != nil {
		return nil, err
	}

	switch typ {
	case ObjectStart, ObjectEnd, ArrayStart, ArrayEnd:
		return json.Delim(token[0]), nil
	case True:
		return true, nil
	case False:
		return false, nil
	case Null:
		//nolint:nilnil // compatibility with encoding/json.
		return nil, nil
	case String:
		return string(token), nil
	case Integer, Decimal:
		num, err := strconv.ParseFloat(viewString(token), 64)
		if err != nil {
			return 0, NewErrDecode("number",
				d.scanner.Position(), typ, token, nil, err)
		}
		return num, nil

	case HexaDecimal:
		num, err := strconv.ParseInt(viewString(token), 0, 64)
		if err != nil {
			return 0, NewErrDecode("hexa-decimal number",
				d.scanner.Position(), typ, token, nil, err)
		}
		return float64(num), nil

	case Complex:
		num, err := strconv.ParseComplex(viewString(token), 128)
		if err != nil {
			return 0, NewErrDecode("complex number",
				d.scanner.Position(), typ, token, nil, err)
		}
		return num, nil

	case Infinity:
		if len(token) > 0 && token[0] == '-' {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	case NaN:
		return math.NaN(), nil
	default:
		return nil, NewErrState("unhandled type",
			d.scanner.Position(), typ, token)
	}
}

//
// State functions for the decoder. Each state function is responsible for
// consuming the next token in the input stream and updating the decoder's
// state accordingly.
//
// Each state function returns the token type, a []byte referencing the token
// value, and an error if one occurred. The state functions are designed to be
// called in a loop by NextToken, which manages the decoder state.
//

// stateObjectKey is the state function for parsing the key of an object.
func (d *Decoder) stateObjectKey() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}

	switch typ {
	case ObjectEnd:
		inObj := d.pop()
		switch {
		case d.len() == 0:
			d.state = (*Decoder).stateEnd
		case inObj:
			d.state = (*Decoder).stateObjectComma
		case !inObj:
			d.state = (*Decoder).stateArrayComma
		}
		return typ, token, nil
	case String:
		d.state = (*Decoder).stateObjectColon
		return typ, token, nil
	default:
		return 0, nil, NewErrState("object key",
			d.scanner.Position(), typ, token)
	}
}

// stateObjectColon is the state function for parsing the colon after a string
// key in an object.
func (d *Decoder) stateObjectColon() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}

	switch typ {
	case Colon:
		d.state = (*Decoder).stateObjectValue
		return d.Next()
	default:
		return 0, nil, NewErrState("object colon",
			d.scanner.Position(), typ, token)
	}
}

// stateObjectValue is the state function for parsing the value after a colon
// in an object.
func (d *Decoder) stateObjectValue() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}

	switch typ {
	case ObjectStart:
		d.state = (*Decoder).stateObjectKey
		d.push(true)
		return typ, token, nil
	case ArrayStart:
		d.state = (*Decoder).stateArrayValue
		d.push(false)
		return typ, token, nil
	default:
		d.state = (*Decoder).stateObjectComma
		return typ, token, nil
	}
}

// stateObjectComma is the state function for parsing the comma after a value
// in an object.
func (d *Decoder) stateObjectComma() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}
	switch typ {
	case ObjectEnd:
		inObj := d.pop()
		switch {
		case d.len() == 0:
			d.state = (*Decoder).stateEnd
		case inObj:
			d.state = (*Decoder).stateObjectComma
		case !inObj:
			d.state = (*Decoder).stateArrayComma
		}
		return typ, token, nil
	case Comma:
		d.state = (*Decoder).stateObjectKey
		return d.Next()
	default:
		return 0, nil, NewErrState("object comma",
			d.scanner.Position(), typ, token)
	}
}

// stateArrayValue is the state function for parsing a value in an array.
func (d *Decoder) stateArrayValue() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}
	switch typ {
	case ObjectStart:
		d.state = (*Decoder).stateObjectKey
		d.push(true)
		return typ, token, nil
	case ArrayStart:
		d.state = (*Decoder).stateArrayValue
		d.push(false)
		return typ, token, nil
	case ArrayEnd:
		inObj := d.pop()
		switch {
		case d.len() == 0:
			d.state = (*Decoder).stateEnd
		case inObj:
			d.state = (*Decoder).stateObjectComma
		case !inObj:
			d.state = (*Decoder).stateArrayComma
		}
		return typ, token, nil
	case Comma:
		return 0, nil, NewErrState("array value",
			d.scanner.Position(), typ, token)
	default:
		d.state = (*Decoder).stateArrayComma
		return typ, token, nil
	}
}

// stateArrayComma is the state function for parsing the comma after a value
// in an array.
func (d *Decoder) stateArrayComma() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}
	switch typ {
	case ArrayEnd:
		inObj := d.pop()
		switch {
		case d.len() == 0:
			d.state = (*Decoder).stateEnd
		case inObj:
			d.state = (*Decoder).stateObjectComma
		case !inObj:
			d.state = (*Decoder).stateArrayComma
		}
		return typ, token, nil
	case Comma:
		d.state = (*Decoder).stateArrayValue
		return d.Next()
	default:
		return 0, nil, NewErrState("array comma",
			d.scanner.Position(), typ, token)
	}
}

// stateValue is the state function for parsing a value in the root of the JSON
// document.
func (d *Decoder) stateValue() (byte, []byte, error) {
	typ, token := d.scanner.Next()
	if typ == EOF {
		return 0, nil, io.ErrUnexpectedEOF
	}

	switch typ {
	case ObjectStart:
		d.state = (*Decoder).stateObjectKey
		d.push(true)
		return typ, token, nil
	case ArrayStart:
		d.state = (*Decoder).stateArrayValue
		d.push(false)
		return typ, token, nil
	case Comma:
		return 0, nil, NewErrState("value",
			d.scanner.Position(), typ, token)
	default:
		d.state = (*Decoder).stateEnd
		return typ, token, nil
	}
}

// stateEnd is the state function for parsing after the end of the JSON
// document. It always returns EOF.
func (*Decoder) stateEnd() (byte, []byte, error) { return EOF, nil, io.EOF }

//
// Decoder functions for decoding JSON values into Go values. These functions
// are responsible for decoding JSON values into Go values of the appropriate
// type. They are called by Decode and are responsible for recursively decoding
// nested objects and arrays.
//

// Decode reads the next JSON-encoded value from the input and stores it
// in the given value pointer.
func (d *Decoder) Decode(value any) error {
	rv := reflect.ValueOf(value)
	switch {
	case rv.Kind() != reflect.Ptr:
		return NewErrDecode("no-pointer", d.scanner.Position(), EOF, nil, rv.Type(), nil)
	case rv.IsNil():
		return NewErrDecode("nil", d.scanner.Position(), EOF, nil, rv.Type(), nil)
	default:
		return d.decodeValue(rv.Elem())
	}
}

// decodeValue decodes the next JSON value from the input stream and stores it
// in the given reflect.Value. The value must be settable. The function is
// responsible for recursively decoding nested objects and arrays.
//
//nolint:gocognit,gocyclo,cyclop,funlen,maintidx // we prioritize speed.
//revive:disable:function-length -- we prioritize speed.
//revive:disable:cognitive-complexity -- we prioritize speed.
//revive:disable:cyclomatic -- we prioritize speed.
func (d *Decoder) decodeValue(v reflect.Value) error {
	typ, token, err := d.Next()
	if err != nil {
		return err
	}

	switch typ {
	case ObjectStart:
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("object",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			m, err := d.decodeMapAny()
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(m))
		case reflect.Map:
			return d.decodeMap(v)
		default:
			return NewErrDecode("object type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case ArrayStart:
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("array",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			s, err := d.decodeSliceAny()
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(s))
		default:
			return NewErrDecode("array type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case True, False:
		value := typ == True
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(value)
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("bool",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(value))
		default:
			return NewErrDecode("bool type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case Null:
		switch v.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice:
			v.Set(reflect.Zero(v.Type()))
			return nil
		default:
			return NewErrDecode("null type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}

	case String:
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("string",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(string(token)))
		case reflect.String:
			v.SetString(string(token))
		default:
			return NewErrDecode("string type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case Integer, Decimal:
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("number",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			num, _ := strconv.ParseFloat(viewString(token), 64)
			v.Set(reflect.ValueOf(num))

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			num, err := strconv.ParseInt(viewString(token), 10, 64)
			if err != nil || v.OverflowInt(num) {
				return NewErrDecode("number int",
					d.scanner.Position(), typ, token, v.Type(), err)
			}
			v.SetInt(num)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			num, err := strconv.ParseUint(viewString(token), 10, 64)
			if err != nil || v.OverflowUint(num) {
				return NewErrDecode("number uint",
					d.scanner.Position(), typ, token, v.Type(), err)
			}
			v.SetUint(num)

		case reflect.Float64, reflect.Float32:
			num, err := strconv.ParseFloat(viewString(token), v.Type().Bits())
			if err != nil || v.OverflowFloat(num) {
				return NewErrDecode("number float",
					d.scanner.Position(), typ, token, v.Type(), err)
			}
			v.SetFloat(num)

		default:
			return NewErrDecode("number type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case HexaDecimal:
		num, _ := strconv.ParseInt(viewString(token), 0, 64)
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("hexa-decimal number",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(float64(num)))

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if v.OverflowInt(num) {
				return NewErrDecode("hexa-decimal number int overflow",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.SetInt(num)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if num < 0 || v.OverflowUint(uint64(num)) {
				return NewErrDecode("hexa-decimal number uint overflow",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.SetUint(uint64(num))

		case reflect.Float64, reflect.Float32:
			v.SetFloat(float64(num))
		default:
			return NewErrDecode("hexa-decimal number type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case Complex:
		num, _ := strconv.ParseComplex(viewString(token), 128)
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("complex number",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(num))
		case reflect.Complex64, reflect.Complex128:
			v.SetComplex(num)
		default:
			return NewErrDecode("complex number type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case Infinity:
		sign := 1
		if len(token) > 0 && token[0] == '-' {
			sign = -1
		}
		num := math.Inf(sign)

		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("infinity",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(num))
		case reflect.Float64, reflect.Float32:
			v.SetFloat(num)
		default:
			return NewErrDecode("infinity type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	case NaN:
		num := math.NaN()
		switch v.Kind() {
		case reflect.Interface:
			if v.NumMethod() > 0 {
				return NewErrDecode("nan",
					d.scanner.Position(), typ, token, v.Type(), nil)
			}
			v.Set(reflect.ValueOf(num))
		case reflect.Float64, reflect.Float32:
			v.SetFloat(num)
		default:
			return NewErrDecode("nan type",
				d.scanner.Position(), typ, token, v.Type(), nil)
		}
		return nil

	default:
		return NewErrDecode("unknown",
			d.scanner.Position(), typ, token, v.Type(), nil)
	}
}

// decodeValueAny decodes the next JSON value from the input stream and returns
// it as an any. This is used for decoding into interface{} values, where the
// type of the value is not known in advance. The function is responsible for
// recursively decoding nested objects and arrays.
//
//nolint:cyclop,funlen // we prioritize speed.
func (d *Decoder) decodeValueAny() (any, error) {
	typ, token, err := d.Next()
	if err != nil {
		return nil, err
	}

	switch typ {
	case ObjectStart:
		return d.decodeMapAny()
	case ArrayStart:
		return d.decodeSliceAny()
	case True:
		return true, nil
	case False:
		return false, nil
	case String:
		return string(token), nil
	case Null:
		//nolint:nilnil // compatibility with encoding/json.
		return nil, nil

	case Integer:
		num := &big.Int{}
		if _, ok := num.SetString(viewString(token), 10); !ok {
			return nil, NewErrDecode("number integer",
				d.scanner.Position(), typ, token, nil, nil)
		}
		return num, nil

	case Decimal:
		num := &big.Float{}
		if _, ok := num.SetString(viewString(token)); !ok {
			return nil, NewErrDecode("number decimal",
				d.scanner.Position(), typ, token, nil, nil)
		}
		return num, nil

	case HexaDecimal:
		num := &big.Int{}
		if _, ok := num.SetString(viewString(token), 0); !ok {
			return nil, NewErrDecode("hexa-decimal number",
				d.scanner.Position(), typ, token, nil, nil)
		}
		return num, nil

	case Complex: // TODO: check optimal type for complex numbers.
		num, err := strconv.ParseComplex(viewString(token), 128)
		if err != nil {
			return nil, NewErrDecode("complex number",
				d.scanner.Position(), typ, token, nil, err)
		}
		return num, nil

	case Infinity:
		if len(token) > 0 && token[0] == '-' {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	case NaN:
		return math.NaN(), nil
	default:
		return nil, NewErrDecode("unknown",
			d.scanner.Position(), typ, token, nil, nil)
	}
}

// decodeMapAny decodes the next JSON object from the input stream and returns
// it as a map[string]any. This is used for decoding into values, where the
// type of the value is not known in advance. The function is responsible for
// recursively decoding nested objects and arrays.
func (d *Decoder) decodeMapAny() (map[string]any, error) {
	m := make(map[string]any)
	for {
		typ, token, err := d.Next()
		if err != nil {
			return nil, err
		} else if typ == ObjectEnd {
			return m, nil
		}

		key := string(token)
		val, err := d.decodeValueAny()
		if err != nil {
			return nil, err
		}
		m[key] = val
	}
}

// decodeMap decodes the next JSON object from the input stream and stores it
// in the given reflect.Value, which must be a map. The function is responsible
// for recursively decoding nested objects and arrays.
func (d *Decoder) decodeMap(v reflect.Value) error {
	t := v.Type()
	kt := t.Key()
	if kt.Kind() != reflect.String {
		return NewErrDecode("map key",
			d.scanner.Position(), ObjectStart, nil, t, nil)
	}

	for {
		typ, token, err := d.Next()
		if err != nil {
			return err
		}
		if typ == ObjectEnd {
			return nil
		}
		key := string(token)
		kv := reflect.ValueOf(key).Convert(kt)

		value := reflect.New(t.Elem()).Elem()
		if err := d.decodeValue(value); err != nil {
			return err
		}
		v.SetMapIndex(kv, value)
	}
}

// decodeSliceAny decodes the next JSON array from the input stream and returns
// it as a []any. This is used for decoding into interface{} values, where the
// type of the value is not known in advance. The function is responsible for
// recursively decoding nested objects and arrays.
//
//nolint:gocognit,cyclop,funlen // we prioritize speed.
//revive:disable:cyclomatic -- we prioritize speed.
func (d *Decoder) decodeSliceAny() ([]any, error) {
	s := make([]any, 0, 1)
	for {
		typ, token, err := d.Next()
		if err != nil {
			return nil, err
		}
		switch typ {
		case ArrayEnd:
			return s, nil
		case ObjectStart:
			m, err := d.decodeMapAny()
			if err != nil {
				return nil, err
			}
			s = append(s, m)
		case ArrayStart:
			sv, err := d.decodeSliceAny()
			if err != nil {
				return nil, err
			}
			s = append(s, sv)
		case True:
			s = append(s, true)
		case False:
			s = append(s, false)
		case String:
			s = append(s, string(token))
		case Null:
			s = append(s, nil)

		case Integer:
			num := &big.Int{}
			if _, ok := num.SetString(viewString(token), 10); !ok {
				return nil, NewErrDecode("number integer",
					d.scanner.Position(), typ, token, nil, nil)
			}
			s = append(s, num)

		case Decimal:
			num := &big.Float{}
			if _, ok := num.SetString(viewString(token)); !ok {
				return nil, NewErrDecode("number decimal",
					d.scanner.Position(), typ, token, nil, nil)
			}
			s = append(s, num)

		case HexaDecimal:
			num := &big.Int{}
			if _, ok := num.SetString(viewString(token), 0); !ok {
				return nil, NewErrDecode("hexa-decimal number",
					d.scanner.Position(), typ, token, nil, nil)
			}
			s = append(s, num)

		case Complex: // TODO: check optimal type for complex numbers.
			num, err := strconv.ParseComplex(viewString(token), 128)
			if err != nil {
				return nil, NewErrDecode("complex number",
					d.scanner.Position(), typ, token, nil, err)
			}
			s = append(s, num)

		case Infinity:
			if len(token) > 0 && token[0] == '-' {
				s = append(s, math.Inf(-1))
			} else {
				s = append(s, math.Inf(1))
			}

		case NaN:
			s = append(s, math.NaN())
		}
	}
}
