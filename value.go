package jsonQuerry

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Value represents any JSON value.
//
// Call Type in order to determine the actual type of the JSON value.
//
// Value cannot be used from concurrent goroutines.
// Use per-goroutine parsers or ParserPool instead.
type Value struct {
	o Object
	a []*Value
	s string
	n float64
	t Type
}

func (v *Value) reset() {
	v.o.reset()
	v.a = v.a[:0]
	v.s = ""
	v.n = 0
	v.t = TypeNull
}

// String returns string representation of the v.
//
// The function is for debugging purposes only. It isn't optimized for speed.
//
// Don't confuse this function with StringBytes, which must be called
// for obtaining the underlying JSON string for the v.
func (v *Value) String() string {
	switch v.Type() {
	case TypeObject:
		return v.o.String()
	case TypeArray:
		// Use bytes.Buffer instead of strings.Builder,
		// so it works on go 1.9 and below.
		var bb bytes.Buffer
		bb.WriteString("[")
		for i, vv := range v.a {
			fmt.Fprintf(&bb, "%s", vv)
			if i != len(v.a)-1 {
				bb.WriteString(",")
			}
		}
		bb.WriteString("]")
		return bb.String()
	case TypeString:
		return fmt.Sprintf("%q", v.s)
	case TypeNumber:
		if float64(int(v.n)) == v.n {
			return fmt.Sprintf("%d", int(v.n))
		}
		return fmt.Sprintf("%f", v.n)
	case TypeTrue:
		return "true"
	case TypeFalse:
		return "false"
	case TypeNull:
		return "null"
	default:
		panic(fmt.Errorf("BUG: unknown Value type: %d", v.Type()))
	}
}

// Type returns the type of the v.
func (v *Value) Type() Type {
	switch v.t {
	case typeRawString:
		v.s = unescapeStringBestEffort(v.s)
		v.t = TypeString
	case typeRawNumber:
		f, err := strconv.ParseFloat(v.s, 64)
		if err != nil {
			f = 0
		}
		v.n = f
		v.t = TypeNumber
	}
	return v.t
}

// Exists returns true if the field exists for the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
func (v *Value) Exists(keys ...string) bool {
	v = v.Get(keys...)
	return v != nil
}

// Get returns value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// nil is returned for non-existing keys path.
//
// The returned value is valid until Parse is called on the Parser returned v.
func (v *Value) Get(keys ...string) *Value {
	if v == nil {
		return nil
	}
	for _, key := range keys {
		switch v.t {
		case TypeObject:
			v = v.o.Get(key)
			if v == nil {
				return nil
			}
		case TypeArray:
			n, err := strconv.Atoi(key)
			if err != nil || n < 0 || n >= len(v.a) {
				return nil
			}
			v = v.a[n]
		default:
			return nil
		}
	}
	return v
}

// GetObject returns object value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// nil is returned for non-existing keys path or for invalid value type.
//
// The returned object is valid until Parse is called on the Parser returned v.
func (v *Value) GetObject(keys ...string) *Object {
	v = v.Get(keys...)
	if v == nil || v.Type() != TypeObject {
		return nil
	}
	return &v.o
}

// GetArray returns array value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// nil is returned for non-existing keys path or for invalid value type.
//
// The returned array is valid until Parse is called on the Parser returned v.
func (v *Value) GetArray(keys ...string) []*Value {
	v = v.Get(keys...)
	if v == nil || v.Type() != TypeArray {
		return nil
	}
	return v.a
}

// GetFloat64 returns float64 value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// 0 is returned for non-existing keys path or for invalid value type.
func (v *Value) GetFloat64(keys ...string) float64 {
	v = v.Get(keys...)
	if v == nil || v.Type() != TypeNumber {
		return 0
	}
	return v.n
}

// GetInt returns int value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// 0 is returned for non-existing keys path or for invalid value type.
func (v *Value) GetInt(keys ...string) int {
	v = v.Get(keys...)
	if v == nil || v.Type() != TypeNumber {
		return 0
	}
	return int(v.n)
}

// GetStringBytes returns string value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// nil is returned for non-existing keys path or for invalid value type.
//
// The returned string is valid until Parse is called on the Parser returned v.
func (v *Value) GetStringBytes(keys ...string) []byte {
	v = v.Get(keys...)
	if v == nil || v.Type() != TypeString {
		return nil
	}
	return s2b(v.s)
}

// GetBool returns bool value by the given keys path.
//
// Array indexes may be represented as decimal numbers in keys.
//
// false is returned for non-existing keys path or for invalid value type.
func (v *Value) GetBool(keys ...string) bool {
	v = v.Get(keys...)
	if v != nil && v.Type() == TypeTrue {
		return true
	}
	return false
}

// Object returns the underlying JSON object for the v.
//
// The returned object is valid until Parse is called on the Parser returned v.
//
// Use GetObject if you don't need error handling.
func (v *Value) Object() (*Object, error) {
	if v.Type() != TypeObject {
		return nil, fmt.Errorf("value doesn't contain object; it contains %s", v.Type())
	}
	return &v.o, nil
}

// Array returns the underlying JSON array for the v.
//
// The returned array is valid until Parse is called on the Parser returned v.
//
// Use GetArray if you don't need error handling.
func (v *Value) Array() ([]*Value, error) {
	if v.Type() != TypeArray {
		return nil, fmt.Errorf("value doesn't contain array; it contains %s", v.Type())
	}
	return v.a, nil
}

// StringBytes returns the underlying JSON string for the v.
//
// The returned string is valid until Parse is called on the Parser returned v.
//
// Use GetStringBytes if you don't need error handling.
func (v *Value) StringBytes() ([]byte, error) {
	if v.Type() != TypeString {
		return nil, fmt.Errorf("value doesn't contain string; it contains %s", v.Type())
	}
	return s2b(v.s), nil
}

// Float64 returns the underlying JSON number for the v.
//
// Use GetFloat64 if you don't need error handling.
func (v *Value) Float64() (float64, error) {
	if v.Type() != TypeNumber {
		return 0, fmt.Errorf("value doesn't contain number; it contains %s", v.Type())
	}
	return v.n, nil
}

// Int returns the underlying JSON int for the v.
//
// Use GetInt if you don't need error handling.
func (v *Value) Int() (int, error) {
	f, err := v.Float64()
	return int(f), err
}

// Bool returns the underlying JSON bool for the v.
//
// Use GetBool if you don't need error handling.
func (v *Value) Bool() (bool, error) {
	switch v.Type() {
	case TypeTrue:
		return true, nil
	case TypeFalse:
		return false, nil
	default:
		return false, fmt.Errorf("value doesn't contain bool; it contains %s", v.Type())
	}
}

// Search return an Array of interface values by the given keys path
func (v *Value) Search(keys ...string) ([]interface{}, error) {
	var rValues []interface{}
	switch v.Type() {
	case TypeArray:
		pValue, err := v.Array()
		if err != nil {
			return nil, err
		}
		for _, uValue := range pValue {
			nValue, err := uValue.Search(keys...)
			if err != nil {
				return nil, err
			}
			rValues = append(rValues, nValue...)
		}
	case TypeObject:
		pValue, err := v.Object()
		if err != nil {
			return nil, err
		}
		nValue, err := pValue.Get(keys[0]).Search(keys[1:]...)
		if err != nil {
			return nil, err
		}
		rValues = append(rValues, nValue...)
	case TypeString:
		rValues = append(rValues, string(v.String()))
	case TypeNumber:
		pValue, err := v.Float64()
		if err != nil {
			return nil, err
		}
		rValues = append(rValues, float64(pValue))
	case TypeFalse:
		rValues = append(rValues, false)
	case TypeTrue:
		rValues = append(rValues, true)
	default:
		return nil, fmt.Errorf("Type not recognized")
	}
	return rValues, nil
}

// {description, produit:{truc,machin,}}

type KeepRequest string

func NewKeepRequest(req string) (KeepRequest, error) {
	strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, req)
	// TODO: add lexer for Request
	return KeepRequest(req), nil
}

func (v *Value) Keep(request KeepRequest) (interface{}, error) {
	// fmt.Println(request)
	// fmt.Println(v.String())
	switch v.Type() {
	case TypeArray:
		// fmt.Println("is Array")
		pValue, err := v.Array()
		if err != nil {
			return nil, err
		}
		rValues := []interface{}{}
		for _, uValue := range pValue {
			nValue, err := uValue.Keep(request)
			if err != nil {
				return nil, err
			}
			rValues = append(rValues, nValue)
		}
		return rValues, nil
	case TypeObject:
		// fmt.Println("is Object")
		pValue, err := v.Object()
		if err != nil {
			return nil, err
		}
		rValues := map[string]interface{}{}
		stays, conts := GetKeys(request)
		for _, stay := range stays {
			// fmt.Printf("key : %s", stay)
			next := pValue.Get(string(stay))
			if next == nil {
				return nil, err
			}
			rValues[string(stay)], err = next.Keep("")
			if err != nil {
				return nil, err
			}
		}
		for _, cont := range conts {
			key := strings.Split(string(cont), ":")[0]
			val := splitBraces(cont)
			rValues[key], err = pValue.Get(key).Keep(val[0])
			if err != nil {
				return nil, err
			}
		}
		return rValues, nil
	case TypeString:
		return v.String(), nil
	case TypeNumber:
		return v.Float64()
	case TypeFalse:
		return false, nil
	case TypeTrue:
		return true, nil
	default:
		return nil, fmt.Errorf("Type not recognized")
	}
}

var (
	valueTrue   = &Value{t: TypeTrue}
	valueFalse  = &Value{t: TypeFalse}
	valueNull   = &Value{t: TypeNull}
	emptyObject = &Value{t: TypeObject}
	emptyArray  = &Value{t: TypeArray}
)