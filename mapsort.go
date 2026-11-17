// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file copies the ordering in Go's internal/fmtsort package, which
// text/template uses to visit map keys in a stable order. It cannot be
// imported because it is internal to the standard library.

package htt

import (
	"cmp"
	"reflect"
	"slices"
)

// keyValue holds one map entry.
type keyValue struct {
	key, value reflect.Value
}

// sortMap returns the entries of mapValue ordered by key, using the same
// rules as text/template: nil compares low, ints, floats and strings order
// by <, and NaN compares less than other floats.
func sortMap(mapValue reflect.Value) []keyValue {
	sorted := make([]keyValue, 0, mapValue.Len())
	iter := mapValue.MapRange()
	for iter.Next() {
		sorted = append(sorted, keyValue{iter.Key(), iter.Value()})
	}
	slices.SortStableFunc(sorted, func(a, b keyValue) int {
		return compareKeys(a.key, b.key)
	})
	return sorted
}

// compareKeys compares two values of the same type. It returns -1, 0, 1
// according to whether a > b (1), a == b (0), or a < b (-1).
// If the types differ, it returns -1.
func compareKeys(aVal, bVal reflect.Value) int {
	aType, bType := aVal.Type(), bVal.Type()
	if aType != bType {
		return -1
	}
	switch aVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmp.Compare(aVal.Int(), bVal.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return cmp.Compare(aVal.Uint(), bVal.Uint())
	case reflect.String:
		return cmp.Compare(aVal.String(), bVal.String())
	case reflect.Float32, reflect.Float64:
		return cmp.Compare(aVal.Float(), bVal.Float())
	case reflect.Complex64, reflect.Complex128:
		a, b := aVal.Complex(), bVal.Complex()
		if c := cmp.Compare(real(a), real(b)); c != 0 {
			return c
		}
		return cmp.Compare(imag(a), imag(b))
	case reflect.Bool:
		a, b := aVal.Bool(), bVal.Bool()
		switch {
		case a == b:
			return 0
		case a:
			return 1
		default:
			return -1
		}
	case reflect.Pointer, reflect.UnsafePointer:
		return cmp.Compare(aVal.Pointer(), bVal.Pointer())
	case reflect.Chan:
		if c, ok := compareNil(aVal, bVal); ok {
			return c
		}
		return cmp.Compare(aVal.Pointer(), bVal.Pointer())
	case reflect.Struct:
		for i := 0; i < aVal.NumField(); i++ {
			if c := compareKeys(aVal.Field(i), bVal.Field(i)); c != 0 {
				return c
			}
		}
		return 0
	case reflect.Array:
		for i := 0; i < aVal.Len(); i++ {
			if c := compareKeys(aVal.Index(i), bVal.Index(i)); c != 0 {
				return c
			}
		}
		return 0
	case reflect.Interface:
		if c, ok := compareNil(aVal, bVal); ok {
			return c
		}
		c := compareKeys(reflect.ValueOf(aVal.Elem().Type()), reflect.ValueOf(bVal.Elem().Type()))
		if c != 0 {
			return c
		}
		return compareKeys(aVal.Elem(), bVal.Elem())
	default:
		panic("bad type in compareKeys: " + aType.String())
	}
}

// compareNil reports whether either value is nil. If not, the boolean is
// false. If either value is nil, the boolean is true and the integer is the
// comparison value: both nil compare equal, and the one nil value compares low.
func compareNil(aVal, bVal reflect.Value) (int, bool) {
	if aVal.IsNil() {
		if bVal.IsNil() {
			return 0, true
		}
		return -1, true
	}
	if bVal.IsNil() {
		return 1, true
	}
	return 0, false
}
