package jsonq

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

// Parser parses JSON.
//
// Parser may be re-used for subsequent parsing.
//
// Parser cannot be used from concurrent goroutines.
// Use per-goroutine parsers or ParserPool instead.
type Parser struct {
	// b contains working copy of the string to be parsed.
	b []byte

	// c is a cache for json values.
	c cache
}

// Parse parses s containing JSON.
//
// The returned value is valid until the next call to Parse*.
//
// Use Scanner if a stream of JSON values must be parsed.
func (p *Parser) Parse(s string) (*Value, error) {
	s = skipWS(s)
	p.b = append(p.b[:0], s...)
	p.c.reset()

	v, tail, err := parseValue(b2s(p.b), &p.c)
	if err != nil {
		return nil, fmt.Errorf("cannot parse JSON: %s; unparsed tail: %q", err, tail)
	}
	tail = skipWS(tail)
	if len(tail) > 0 {
		return nil, fmt.Errorf("unexpected tail: %q", tail)
	}
	return v, nil
}

// ParseBytes parses b containing JSON.
//
// The returned Value is valid until the next call to Parse*.
//
// Use Scanner if a stream of JSON values must be parsed.
func (p *Parser) ParseBytes(b []byte) (*Value, error) {
	return p.Parse(b2s(b))
}

func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func s2b(s string) []byte {
	strh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	var sh reflect.SliceHeader
	sh.Data = strh.Data
	sh.Len = strh.Len
	sh.Cap = strh.Len
	return *(*[]byte)(unsafe.Pointer(&sh))
}

func skipWS(s string) string {
	if len(s) == 0 || s[0] > '\x20' {
		// Fast path.
		return s
	}

	// Slow path.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		// Whitespace chars are obtained from http://www.ietf.org/rfc/rfc4627.txt .
		case '\x20', '\x0D', '\x0A', '\x09':
			continue
		default:
			return s[i:]
		}
	}
	return ""
}

type kv struct {
	k string
	v *Value
}

func parseValue(s string, c *cache) (*Value, string, error) {
	if len(s) == 0 {
		return nil, s, fmt.Errorf("cannot parse empty string")
	}

	// fmt.Printf("- %s\n", s)

	var v *Value
	var err error

	switch s[0] {
	case '{':
		v, s, err = parseObject(s, c)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse object: %s", err)
		}
		return v, s, nil
	case '[':
		v, s, err = parseArray(s, c)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse array: %s", err)
		}
		return v, s, nil
	case '"':
		var ss string
		ss, s, err = parseRawString(s)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse string: %s", err)
		}
		v = c.getValue()
		v.t = typeRawString
		v.s = ss
		return v, s, nil
	case 't':
		if !strings.HasPrefix(s, "true") {
			return nil, s, fmt.Errorf("unexpected value found: %q", s)
		}
		s = s[len("true"):]
		return valueTrue, s, nil
	case 'f':
		if !strings.HasPrefix(s, "false") {
			return nil, s, fmt.Errorf("unexpected value found: %q", s)
		}
		s = s[len("false"):]
		return valueFalse, s, nil
	case 'n':
		if !strings.HasPrefix(s, "null") {
			return nil, s, fmt.Errorf("unexpected value found: %q", s)
		}
		s = s[len("null"):]
		return valueNull, s, nil
	default:
		var ns string
		ns, s, err = parseRawNumber(s)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse number: %s", err)
		}
		v = c.getValue()
		v.t = typeRawNumber
		v.s = ns
		return v, s, nil
	}
}

func parseArray(s string, c *cache) (*Value, string, error) {
	// Skip the first char - '['
	s = s[1:]

	s = skipWS(s)
	if len(s) == 0 {
		return nil, s, fmt.Errorf("missing ']'")
	}

	if s[0] == ']' {
		return emptyArray, s[1:], nil
	}

	a := c.getValue()
	a.t = TypeArray
	for {
		var v *Value
		var err error

		s = skipWS(s)
		v, s, err = parseValue(s, c)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse array value: %s", err)
		}
		a.a = append(a.a, v)

		s = skipWS(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unexpected end of array")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == ']' {
			s = s[1:]
			return a, s, nil
		}
		return nil, s, fmt.Errorf("missing ',' after array value")
	}
}

func parseObject(s string, c *cache) (*Value, string, error) {
	// Skip the first char - '{'
	s = s[1:]

	s = skipWS(s)
	if len(s) == 0 {
		return nil, s, fmt.Errorf("missing '}'")
	}

	if s[0] == '}' {
		return emptyObject, s[1:], nil
	}

	o := c.getValue()
	o.t = TypeObject
	for {
		var err error
		kv := o.o.getKV()

		// Parse key.
		s = skipWS(s)
		kv.k, s, err = parseRawString(s)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse object key: %s", err)
		}
		s = skipWS(s)
		if len(s) == 0 || s[0] != ':' {
			return nil, s, fmt.Errorf("missing ':' after object key")
		}
		s = s[1:]

		// Parse value
		s = skipWS(s)
		kv.v, s, err = parseValue(s, c)
		if err != nil {
			return nil, s, fmt.Errorf("cannot parse object value: %s", err)
		}
		s = skipWS(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unexpected end of object")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == '}' {
			return o, s[1:], nil
		}
		return nil, s, fmt.Errorf("missing ',' after object value")
	}
}

func unescapeStringBestEffort(s string) string {
	n := strings.IndexByte(s, '\\')
	if n < 0 {
		// Fast path - nothing to unescape.
		return s
	}

	// Slow path - unescape string.
	b := s2b(s) // It is safe to do, since s points to a byte slice in Parser.b.
	b = b[:n]
	s = s[n+1:]
	for len(s) > 0 {
		ch := s[0]
		s = s[1:]
		switch ch {
		case '"':
			b = append(b, '"')
		case '\\':
			b = append(b, '\\')
		case '/':
			b = append(b, '/')
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'u':
			if len(s) < 4 {
				// Too short escape sequence. Just store it unchanged.
				b = append(b, '\\', ch)
				break
			}
			xs := s[:4]
			x, err := strconv.ParseUint(xs, 16, 16)
			if err != nil {
				// Invalid escape sequence. Just store it unchanged.
				b = append(b, '\\', ch)
				break
			}
			b = append(b, string(rune(x))...)
			s = s[4:]
		default:
			// Unknown escape sequence. Just store it unchanged.
			b = append(b, '\\', ch)
		}
		n = strings.IndexByte(s, '\\')
		if n < 0 {
			b = append(b, s...)
			break
		}
		b = append(b, s[:n]...)
		s = s[n+1:]
	}
	return b2s(b)
}

func parseRawString(s string) (string, string, error) {
	if len(s) == 0 || s[0] != '"' {
		return "", s, fmt.Errorf(`missing opening '"'`)
	}
	s = s[1:]

	n := strings.IndexByte(s, '"')
	if n < 0 {
		return "", "", fmt.Errorf(`missing closing '"'`)
	}
	if n == 0 || s[n-1] != '\\' {
		// Fast path. No escaped ".
		return s[:n], s[n+1:], nil
	}

	// Slow path - possible escaped " found.
	ss := s
	for {
		i := n - 1
		for i > 0 && s[i-1] == '\\' {
			i--
		}
		if uint(n-i)%2 == 0 {
			return ss[:len(ss)-len(s)+n], s[n+1:], nil
		}
		s = s[n+1:]

		n = strings.IndexByte(s, '"')
		if n < 0 {
			return "", "", fmt.Errorf(`missing closing '"'`)
		}
		if n == 0 || s[n-1] != '\\' {
			return ss[:len(ss)-len(s)+n], s[n+1:], nil
		}
	}
}

func parseRawNumber(s string) (string, string, error) {
	// The caller must ensure len(s) > 0

	// Find the end of the number.
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= '0' && ch <= '9') || ch == '-' || ch == '.' || ch == 'e' || ch == 'E' || ch == '+' {
			continue
		}
		if i == 0 {
			return "", s, fmt.Errorf("unexpected char: %q", s[:1])
		}
		ns := s[:i]
		s = s[i:]
		return ns, s, nil
	}
	return s, "", nil
}
