// Package settings читает и пишет settings.json Claude Code, сохраняя порядок ключей:
// установщик правит чужой файл, и в нём должны меняться только его ключи.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Object — JSON-объект с порядком ключей. Значения: *Object, []any, string,
// json.Number, bool и nil.
type Object struct {
	keys []string
	vals map[string]any
}

// NewObject возвращает пустой объект.
func NewObject() *Object {
	return &Object{vals: map[string]any{}}
}

// With задаёт ключ и возвращает тот же объект: для литералов в коде.
func (o *Object) With(key string, value any) *Object {
	o.Set(key, value)
	return o
}

// Get возвращает значение ключа.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Set задаёт значение; новый ключ встаёт в конец, прежний остаётся на своём месте.
func (o *Object) Set(key string, value any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

// Delete удаляет ключ и сообщает, был ли он.
func (o *Object) Delete(key string) bool {
	if _, ok := o.vals[key]; !ok {
		return false
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
	return true
}

// Keys возвращает ключи по порядку.
func (o *Object) Keys() []string {
	return append([]string(nil), o.keys...)
}

// Child возвращает вложенный объект; если ключа нет, создаёт его.
func (o *Object) Child(key string) (*Object, error) {
	v, ok := o.vals[key]
	if !ok {
		child := NewObject()
		o.Set(key, child)
		return child, nil
	}
	child, ok := v.(*Object)
	if !ok {
		return nil, fmt.Errorf("%q — не объект", key)
	}
	return child, nil
}

// Lookup возвращает вложенный объект, если он есть.
func (o *Object) Lookup(key string) (*Object, bool) {
	child, ok := o.vals[key].(*Object)
	return child, ok
}

// Parse разбирает JSON-объект.
func Parse(data []byte) (*Object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("после объекта есть ещё данные")
	}
	obj, ok := v.(*Object)
	if !ok {
		return nil, errors.New("в файле не объект")
	}
	return obj, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		obj := NewObject()
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := keyTok.(string)
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			obj.Set(key, v)
		}
		_, err = dec.Token()
		return obj, err
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err = dec.Token()
		return arr, err
	default:
		return nil, fmt.Errorf("неожиданный %q", delim)
	}
}

// Encode пишет объект с отступом в два пробела, как Claude Code, и переводом строки в конце.
func (o *Object) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeValue(&buf, o, ""); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func encodeValue(buf *bytes.Buffer, v any, indent string) error {
	inner := indent + "  "
	switch v := v.(type) {
	case *Object:
		if len(v.keys) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		for i, k := range v.keys {
			buf.WriteString(inner)
			if err := encodeString(buf, k); err != nil {
				return err
			}
			buf.WriteString(": ")
			if err := encodeValue(buf, v.vals[k], inner); err != nil {
				return err
			}
			if i < len(v.keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent + "}")
	case []any:
		if len(v) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, item := range v {
			buf.WriteString(inner)
			if err := encodeValue(buf, item, inner); err != nil {
				return err
			}
			if i < len(v)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent + "]")
	case string:
		return encodeString(buf, v)
	case json.Number:
		buf.WriteString(v.String())
	case bool, nil, int, float64:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	default:
		return fmt.Errorf("значение типа %T не пишется в JSON", v)
	}
	return nil
}

// encodeString пишет строку без экранирования <, > и &: в командах settings.json они обычны.
func encodeString(buf *bytes.Buffer, s string) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	buf.WriteString(strings.TrimSuffix(tmp.String(), "\n"))
	return nil
}
