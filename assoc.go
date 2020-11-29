package assoc

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// An Object is a struct whose fields reference Values.
type Object interface{}

// A Value is a string, numeric, or []byte value found traversing the fields of an Object.
// All []byte values are stored as a string.
type Value interface{}

// A Key is a unique identifier for an Object.
type Key string

type ValuePath string

// Index indexes all of the Values in an Object.
type Index struct {
	mu    sync.RWMutex
	ind   map[Value]map[Key]ValuePath // index of values -> objects
	rev   map[Key][]Value             // reverse index of objects -> values
	objs  map[Key]Object              // copies of keyed objects
	str   map[Value]string            // values as strings
	keyFn func(Object) Key
}

func NewIndex(keyFn func(Object) Key) *Index {
	return &Index{
		ind:   make(map[Value]map[Key]ValuePath),
		rev:   make(map[Key][]Value),
		objs:  make(map[Key]Object),
		str:   make(map[Value]string),
		keyFn: keyFn,
	}
}

func (a *Index) Add(oldObj, newObj Object) {
	var newKey, oldKey Key
	if newObj != nil {
		newKey = a.keyFn(newObj)
	}
	if oldObj != nil {
		oldKey = a.keyFn(oldObj)
		if newKey != "" && oldKey != newKey {
			panic(fmt.Sprintf("assoc.Index: old key %q != new key %q", oldKey, newKey))
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if oldObj != nil {
		for _, val := range a.rev[oldKey] {
			delete(a.ind[val], oldKey)
			delete(a.str, val)
		}
		delete(a.rev, oldKey)
		delete(a.objs, oldKey)
	}
	if newObj != nil {
		vals := make(map[Value]ValuePath)
		a.extractValues(vals, "", reflect.ValueOf(newObj))
		if len(vals) == 0 {
			return
		}
		//fmt.Printf("extractValues: %#+v\n", vals)
		for val, path := range vals {
			if a.ind[val] == nil {
				a.ind[val] = make(map[Key]ValuePath)
			}
			a.ind[val][newKey] = path
			a.rev[newKey] = append(a.rev[newKey], val)
			if v, _ := val.(encoding.TextMarshaler); v != nil {
				text, err := v.MarshalText()
				if err == nil {
					a.str[val] = string(text)
				}
			} else if stringer, _ := val.(fmt.Stringer); stringer != nil {
				a.str[val] = stringer.String()
			} else {
				a.str[val] = fmt.Sprintf("%v", val)
			}
			//fmt.Printf("a.str[val] = %s\n", a.str[val])
		}
		a.objs[newKey] = newObj
	}
}

func (a *Index) Collect(val Value) map[Object]ValuePath {
	a.mu.RLock()
	defer a.mu.RUnlock()
	res := make(map[Object]ValuePath)
	for key, path := range a.ind[val] {
		res[a.objs[key]] = path
	}
	if len(res) == 0 {
		return nil
	}
	return res
}

func (a *Index) Search(substr string) (res []Value) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for val, valStr := range a.str {
		//fmt.Printf("valStr=%q\n", valStr)
		if strings.Contains(valStr, substr) {
			res = append(res, val)
		}
	}
	return res
}

func (a *Index) extractValues(vals map[Value]ValuePath, path ValuePath, v reflect.Value) {
	if !v.IsValid() || v.IsZero() {
		return
	}
	if _, isLeaf := v.Interface().(encoding.TextMarshaler); isLeaf {
		vals[v.Interface()] = path
		return
	}

	switch v.Kind() {
	case reflect.Bool:
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		vals[v.Interface()] = path
	case reflect.String:
		vals[v.Interface()] = path
	case reflect.Array:
	case reflect.Chan:
	case reflect.Func:
	case reflect.Interface:
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()
			a.extractValues(vals, "", k)
			a.extractValues(vals, "", v)
		}
	case reflect.Ptr:
		a.extractValues(vals, "", v.Elem())
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			a.extractValues(vals, path, v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			t := v.Type().Field(i)
			if t.PkgPath != "" {
				continue // unexported field
			}
			name := ValuePath(t.Name)
			if path != "" {
				name = path + "." + name
			}
			a.extractValues(vals, name, v.Field(i))
		}
	case reflect.UnsafePointer:
	}
}
