// Package vibe implements a Go parser for the VIBE configuration format.
// VIBE (Values In Bracket Expression) is a human-friendly configuration format
// with minimal syntax and fast parsing.
package vibe

import (
	"fmt"
	"strconv"
)

// ValueType represents the type of a VIBE value
type ValueType int

const (
	TypeNull ValueType = iota
	TypeString
	TypeInt
	TypeFloat
	TypeBool
	TypeArray
	TypeObject
)

func (t ValueType) String() string {
	switch t {
	case TypeNull:
		return "null"
	case TypeString:
		return "string"
	case TypeInt:
		return "integer"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "boolean"
	case TypeArray:
		return "array"
	case TypeObject:
		return "object"
	default:
		return "unknown"
	}
}

// Value represents a VIBE value of any type
type Value struct {
	Type   ValueType
	String string
	Int    int64
	Float  float64
	Bool   bool
	Array  []*Value
	Object *Object
	Line   int
	Column int
}

// Object represents a VIBE object (key-value pairs)
type Object struct {
	Keys   []string          // Ordered list of keys
	Values map[string]*Value // Key to value mapping
}

// NewObject creates a new empty Object
func NewObject() *Object {
	return &Object{
		Keys:   make([]string, 0),
		Values: make(map[string]*Value),
	}
}

// Set sets a key-value pair in the object
func (o *Object) Set(key string, value *Value) {
	if _, exists := o.Values[key]; !exists {
		o.Keys = append(o.Keys, key)
	}
	o.Values[key] = value
}

// Get retrieves a value by key
func (o *Object) Get(key string) *Value {
	return o.Values[key]
}

// Has checks if a key exists
func (o *Object) Has(key string) bool {
	_, exists := o.Values[key]
	return exists
}

// Len returns the number of keys
func (o *Object) Len() int {
	return len(o.Keys)
}

// NewStringValue creates a new string value
func NewStringValue(s string) *Value {
	return &Value{Type: TypeString, String: s}
}

// NewIntValue creates a new integer value
func NewIntValue(i int64) *Value {
	return &Value{Type: TypeInt, Int: i}
}

// NewFloatValue creates a new float value
func NewFloatValue(f float64) *Value {
	return &Value{Type: TypeFloat, Float: f}
}

// NewBoolValue creates a new boolean value
func NewBoolValue(b bool) *Value {
	return &Value{Type: TypeBool, Bool: b}
}

// NewArrayValue creates a new array value
func NewArrayValue(arr []*Value) *Value {
	return &Value{Type: TypeArray, Array: arr}
}

// NewObjectValue creates a new object value
func NewObjectValue(obj *Object) *Value {
	return &Value{Type: TypeObject, Object: obj}
}

// GetPath retrieves a nested value using dot notation path
// Example: "server.ssl.enabled" or "servers[0]"
func (v *Value) GetPath(path string) *Value {
	if v == nil || path == "" {
		return v
	}

	current := v
	segments := parsePath(path)

	for _, seg := range segments {
		if current == nil {
			return nil
		}

		if seg.isIndex {
			// Array access
			if current.Type != TypeArray {
				return nil
			}
			if seg.index < 0 || seg.index >= len(current.Array) {
				return nil
			}
			current = current.Array[seg.index]
		} else {
			// Object access
			if current.Type != TypeObject || current.Object == nil {
				return nil
			}
			current = current.Object.Get(seg.key)
		}
	}

	return current
}

type pathSegment struct {
	key     string
	isIndex bool
	index   int
}

func parsePath(path string) []pathSegment {
	var segments []pathSegment
	var current string
	i := 0

	for i < len(path) {
		ch := path[i]

		if ch == '.' {
			if current != "" {
				segments = append(segments, pathSegment{key: current})
				current = ""
			}
			i++
		} else if ch == '[' {
			if current != "" {
				segments = append(segments, pathSegment{key: current})
				current = ""
			}
			// Find the closing bracket
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if j < len(path) {
				indexStr := path[i+1 : j]
				if idx, err := strconv.Atoi(indexStr); err == nil {
					segments = append(segments, pathSegment{isIndex: true, index: idx})
				}
				i = j + 1
			} else {
				i++
			}
		} else {
			current += string(ch)
			i++
		}
	}

	if current != "" {
		segments = append(segments, pathSegment{key: current})
	}

	return segments
}

// GetString retrieves a string value at the given path
func (v *Value) GetString(path string) string {
	val := v.GetPath(path)
	if val == nil {
		return ""
	}
	switch val.Type {
	case TypeString:
		return val.String
	case TypeInt:
		return strconv.FormatInt(val.Int, 10)
	case TypeFloat:
		return strconv.FormatFloat(val.Float, 'f', -1, 64)
	case TypeBool:
		return strconv.FormatBool(val.Bool)
	default:
		return ""
	}
}

// GetStringDefault retrieves a string value or returns the default
func (v *Value) GetStringDefault(path, defaultVal string) string {
	val := v.GetPath(path)
	if val == nil || val.Type != TypeString {
		return defaultVal
	}
	return val.String
}

// GetInt retrieves an integer value at the given path
func (v *Value) GetInt(path string) int64 {
	val := v.GetPath(path)
	if val == nil {
		return 0
	}
	switch val.Type {
	case TypeInt:
		return val.Int
	case TypeFloat:
		return int64(val.Float)
	case TypeString:
		if i, err := strconv.ParseInt(val.String, 10, 64); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}

// GetIntDefault retrieves an integer value or returns the default
func (v *Value) GetIntDefault(path string, defaultVal int64) int64 {
	val := v.GetPath(path)
	if val == nil {
		return defaultVal
	}
	switch val.Type {
	case TypeInt:
		return val.Int
	case TypeFloat:
		return int64(val.Float)
	default:
		return defaultVal
	}
}

// GetFloat retrieves a float value at the given path
func (v *Value) GetFloat(path string) float64 {
	val := v.GetPath(path)
	if val == nil {
		return 0
	}
	switch val.Type {
	case TypeFloat:
		return val.Float
	case TypeInt:
		return float64(val.Int)
	case TypeString:
		if f, err := strconv.ParseFloat(val.String, 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

// GetFloatDefault retrieves a float value or returns the default
func (v *Value) GetFloatDefault(path string, defaultVal float64) float64 {
	val := v.GetPath(path)
	if val == nil {
		return defaultVal
	}
	switch val.Type {
	case TypeFloat:
		return val.Float
	case TypeInt:
		return float64(val.Int)
	default:
		return defaultVal
	}
}

// GetBool retrieves a boolean value at the given path
func (v *Value) GetBool(path string) bool {
	val := v.GetPath(path)
	if val == nil {
		return false
	}
	switch val.Type {
	case TypeBool:
		return val.Bool
	case TypeString:
		return val.String == "true"
	case TypeInt:
		return val.Int != 0
	default:
		return false
	}
}

// GetBoolDefault retrieves a boolean value or returns the default
func (v *Value) GetBoolDefault(path string, defaultVal bool) bool {
	val := v.GetPath(path)
	if val == nil || val.Type != TypeBool {
		return defaultVal
	}
	return val.Bool
}

// GetArray retrieves an array value at the given path
func (v *Value) GetArray(path string) []*Value {
	val := v.GetPath(path)
	if val == nil || val.Type != TypeArray {
		return nil
	}
	return val.Array
}

// GetStringArray retrieves an array of strings at the given path
func (v *Value) GetStringArray(path string) []string {
	arr := v.GetArray(path)
	if arr == nil {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if item.Type == TypeString {
			result = append(result, item.String)
		}
	}
	return result
}

// GetObject retrieves an object value at the given path
func (v *Value) GetObject(path string) *Object {
	val := v.GetPath(path)
	if val == nil || val.Type != TypeObject {
		return nil
	}
	return val.Object
}

// IsNull checks if the value is null or nil
func (v *Value) IsNull() bool {
	return v == nil || v.Type == TypeNull
}

// String returns a string representation of the value
func (v *Value) GoString() string {
	if v == nil {
		return "null"
	}
	switch v.Type {
	case TypeNull:
		return "null"
	case TypeString:
		return fmt.Sprintf("%q", v.String)
	case TypeInt:
		return strconv.FormatInt(v.Int, 10)
	case TypeFloat:
		return strconv.FormatFloat(v.Float, 'f', -1, 64)
	case TypeBool:
		return strconv.FormatBool(v.Bool)
	case TypeArray:
		return fmt.Sprintf("[array:%d]", len(v.Array))
	case TypeObject:
		if v.Object != nil {
			return fmt.Sprintf("{object:%d}", v.Object.Len())
		}
		return "{object:nil}"
	default:
		return "unknown"
	}
}
