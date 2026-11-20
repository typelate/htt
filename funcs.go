package htt

import (
	"fmt"
	"reflect"
	texttemplate "text/template"
)

// builtins returns the functions every template may call.
func builtins() FuncMap {
	return FuncMap{
		"and":      and,
		"call":     emptyCall,
		"html":     texttemplate.HTMLEscaper,
		"index":    index,
		"slice":    slice,
		"js":       texttemplate.JSEscaper,
		"len":      length,
		"not":      not,
		"or":       or,
		"print":    fmt.Sprint,
		"printf":   fmt.Sprintf,
		"println":  fmt.Sprintln,
		"urlquery": texttemplate.URLQueryEscaper,

		// Comparisons
		"eq": eq,
		"ge": ge,
		"gt": gt,
		"le": le,
		"lt": lt,
		"ne": ne,
	}
}

// IsTrue reports whether val is true as {{if}} sees it, meaning not the
// zero of its type, and whether val has a meaningful truth value.
func IsTrue(val any) (truth, ok bool) {
	return isTrue(reflect.ValueOf(val))
}

func isTrue(val reflect.Value) (truth, ok bool) {
	if !val.IsValid() {
		return false, true
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		truth = val.Len() > 0
	case reflect.Bool:
		truth = val.Bool()
	case reflect.Complex64, reflect.Complex128:
		truth = val.Complex() != 0
	case reflect.Chan, reflect.Func, reflect.Pointer, reflect.UnsafePointer, reflect.Interface:
		truth = !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		truth = val.Int() != 0
	case reflect.Float32, reflect.Float64:
		truth = val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		truth = val.Uint() != 0
	case reflect.Struct:
		truth = true // Struct values are always true.
	default:
		return
	}
	return truth, true
}

// and and or give the builtins their names and signatures. Execution does
// not call them: evalAndOr evaluates the builtins lazily.
func and(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if !truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if !truth(arg0) {
			break
		}
	}
	return arg0
}

func or(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if truth(arg0) {
			break
		}
	}
	return arg0
}

func not(arg reflect.Value) bool {
	return !truth(arg)
}

func truth(arg reflect.Value) bool {
	t, _ := isTrue(indirectInterface(arg))
	return t
}

func length(item reflect.Value) (int, error) {
	item, isNil := indirect(item)
	if isNil {
		return 0, fmt.Errorf("len of nil pointer")
	}
	switch item.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return item.Len(), nil
	default:
		return 0, fmt.Errorf("len of type %s", item.Type())
	}
}

// emptyCall is never called: the executor runs the builtin call itself, with the
// callee's text as its name, as text/template does.
func emptyCall(fn reflect.Value, args ...reflect.Value) reflect.Value {
	panic("unreachable")
}

// call returns the result of calling fn, which name identifies in errors.
func call(name string, fn reflect.Value, args ...reflect.Value) (reflect.Value, error) {
	fn = indirectInterface(fn)
	if !fn.IsValid() {
		return reflect.Value{}, fmt.Errorf("call of nil")
	}
	typ := fn.Type()
	if typ.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("non-function %s of type %s", name, typ)
	}
	if err := goodFunc(name, typ); err != nil {
		return reflect.Value{}, err
	}
	numIn := typ.NumIn()
	var dddType reflect.Type
	if typ.IsVariadic() {
		if len(args) < numIn-1 {
			return reflect.Value{}, fmt.Errorf("wrong number of args for %s: got %d want at least %d", name, len(args), numIn-1)
		}
		dddType = typ.In(numIn - 1).Elem()
	} else {
		if len(args) != numIn {
			return reflect.Value{}, fmt.Errorf("wrong number of args for %s: got %d want %d", name, len(args), numIn)
		}
	}
	argv := make([]reflect.Value, len(args))
	for i, arg := range args {
		paramType := argType(typ, i, numIn, dddType)
		arg = indirectInterface(arg)
		if !arg.IsValid() && !canBeNil(paramType) {
			return reflect.Value{}, fmt.Errorf("value is nil; should be of type %s", paramType)
		}
		argv[i] = prepareArg(arg, paramType)
	}
	return safeCall(fn, argv)
}

func argType(typ reflect.Type, i, numIn int, dddType reflect.Type) reflect.Type {
	if !typ.IsVariadic() || i < numIn-1 {
		return typ.In(i)
	}
	return dddType
}

func safeCall(fun reflect.Value, args []reflect.Value) (val reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	ret := fun.Call(args)
	if len(ret) == 2 && !ret[1].IsNil() {
		return ret[0], ret[1].Interface().(error)
	}
	return ret[0], nil
}

// index returns item indexed by each of indexes in turn.
func index(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("index of untyped nil")
	}
	for _, idx := range indexes {
		idx = indirectInterface(idx)
		var isNil bool
		if item, isNil = indirect(item); isNil {
			return reflect.Value{}, fmt.Errorf("index of nil pointer")
		}
		switch item.Kind() {
		case reflect.Array, reflect.Slice, reflect.String:
			x, err := indexArg(idx, item.Len())
			if err != nil {
				return reflect.Value{}, err
			}
			item = item.Index(x)
		case reflect.Map:
			idx, err := prepareArgErr(idx, item.Type().Key())
			if err != nil {
				return reflect.Value{}, err
			}
			elemType := item.Type().Elem()
			item = item.MapIndex(idx)
			if !item.IsValid() {
				item = reflect.Zero(elemType)
			}
		default:
			return reflect.Value{}, fmt.Errorf("can't index item of type %s", item.Type())
		}
	}
	return item, nil
}

// indexArg converts idx to an int no greater than length, as text/template's
// indexArg does. An index equal to length passes, and the indexing that
// follows panics; safeCall reports that panic as the call's error.
func indexArg(idx reflect.Value, length int) (int, error) {
	var x int64
	switch idx.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x = idx.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		x = int64(idx.Uint())
	case reflect.Invalid:
		return 0, fmt.Errorf("cannot index slice/array with nil")
	default:
		return 0, fmt.Errorf("cannot index slice/array with type %s", idx.Type())
	}
	if x < 0 || int(x) > length {
		return 0, fmt.Errorf("index out of range: %d", x)
	}
	return int(x), nil
}

// slice returns the result of slicing its first argument.
func slice(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("slice of untyped nil")
	}
	var isNil bool
	if item, isNil = indirect(item); isNil {
		return reflect.Value{}, fmt.Errorf("slice of nil pointer")
	}
	if len(indexes) > 3 {
		return reflect.Value{}, fmt.Errorf("too many slice indexes: %d", len(indexes))
	}
	var cap int
	switch item.Kind() {
	case reflect.String:
		if len(indexes) == 3 {
			return reflect.Value{}, fmt.Errorf("cannot 3-index slice a string")
		}
		cap = item.Len()
	case reflect.Array, reflect.Slice:
		cap = item.Cap()
	default:
		return reflect.Value{}, fmt.Errorf("can't slice item of type %s", item.Type())
	}
	// Set defaults
	idx := [3]int{0, item.Len(), cap}
	for i, idxVal := range indexes {
		x, err := indexArg(idxVal, cap)
		if err != nil {
			return reflect.Value{}, err
		}
		idx[i] = x
	}
	// Given item[i:j], i must not be above j, as text/template checks it.
	if idx[0] > idx[1] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[0], idx[1])
	}
	if len(indexes) < 3 {
		return item.Slice(idx[0], idx[1]), nil
	}
	// Given item[i:j:k], j must not be above k.
	if idx[1] > idx[2] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[1], idx[2])
	}
	return item.Slice3(idx[0], idx[1], idx[2]), nil
}

// Comparison functions

func eq(arg1 reflect.Value, arg2 ...reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	if len(arg2) == 0 {
		return false, fmt.Errorf("missing argument for comparison")
	}
	k1, _ := basicKind(arg1)
	for _, arg := range arg2 {
		arg = indirectInterface(arg)
		k2, _ := basicKind(arg)
		truth := false
		if k1 != k2 {
			// Special case: integer values compare regardless of the sign of their type.
			switch {
			case k1 == intKind && k2 == uintKind:
				truth = arg1.Int() >= 0 && uint64(arg1.Int()) == arg.Uint()
			case k1 == uintKind && k2 == intKind:
				truth = arg.Int() >= 0 && arg1.Uint() == uint64(arg.Int())
			default:
				if arg1.IsValid() && arg.IsValid() {
					return false, fmt.Errorf("incompatible types for comparison: %v and %v", arg1.Type(), arg.Type())
				}
			}
		} else {
			switch k1 {
			case boolKind:
				truth = arg1.Bool() == arg.Bool()
			case complexKind:
				truth = arg1.Complex() == arg.Complex()
			case floatKind:
				truth = arg1.Float() == arg.Float()
			case intKind:
				truth = arg1.Int() == arg.Int()
			case stringKind:
				truth = arg1.String() == arg.String()
			case uintKind:
				truth = arg1.Uint() == arg.Uint()
			default:
				if !canCompare(arg1, arg) {
					return false, fmt.Errorf("non-comparable types %s: %v, %s: %v", arg1, arg1.Type(), arg.Type(), arg)
				}
				if isNil(arg1) || isNil(arg) {
					truth = isNil(arg) == isNil(arg1)
				} else {
					if !arg.Type().Comparable() {
						return false, fmt.Errorf("non-comparable type %s: %v", arg, arg.Type())
					}
					truth = arg1.Interface() == arg.Interface()
				}
			}
		}
		if truth {
			return true, nil
		}
	}
	return false, nil
}

func ne(arg1, arg2 reflect.Value) (bool, error) {
	equal, err := eq(arg1, arg2)
	return !equal, err
}

func lt(arg1, arg2 reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	k1, err := basicKind(arg1)
	if err != nil {
		return false, err
	}
	arg2 = indirectInterface(arg2)
	k2, err := basicKind(arg2)
	if err != nil {
		return false, err
	}
	truth := false
	if k1 != k2 {
		// Special case: integer values compare regardless of the sign of their type.
		switch {
		case k1 == intKind && k2 == uintKind:
			truth = arg1.Int() < 0 || uint64(arg1.Int()) < arg2.Uint()
		case k1 == uintKind && k2 == intKind:
			truth = arg2.Int() >= 0 && arg1.Uint() < uint64(arg2.Int())
		default:
			return false, fmt.Errorf("incompatible types for comparison: %v and %v", arg1.Type(), arg2.Type())
		}
	} else {
		switch k1 {
		case boolKind, complexKind:
			return false, fmt.Errorf("invalid type for comparison")
		case floatKind:
			truth = arg1.Float() < arg2.Float()
		case intKind:
			truth = arg1.Int() < arg2.Int()
		case stringKind:
			truth = arg1.String() < arg2.String()
		case uintKind:
			truth = arg1.Uint() < arg2.Uint()
		default:
			panic("invalid kind")
		}
	}
	return truth, nil
}

func le(arg1, arg2 reflect.Value) (bool, error) {
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}
	equal, err := eq(arg1, arg2)
	return equal, err
}

func gt(arg1, arg2 reflect.Value) (bool, error) {
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessOrEqual, nil
}

func ge(arg1, arg2 reflect.Value) (bool, error) {
	lessThan, err := lt(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessThan, nil
}

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	stringKind
	uintKind
)

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	}
	return invalidKind, fmt.Errorf("invalid type for comparison")
}

// isNil reports whether v is the zero reflect.Value, or nil of its type.
func isNil(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// canCompare reports whether v1 and v2 are both the same kind, or one is nil.
// It is called only for nillable types, or just before an error.
func canCompare(v1, v2 reflect.Value) bool {
	k1 := v1.Kind()
	k2 := v2.Kind()
	if k1 == k2 {
		return true
	}
	return k1 == reflect.Invalid || k2 == reflect.Invalid
}

// prepareArg converts value to typ where it can.
func prepareArg(value reflect.Value, typ reflect.Type) reflect.Value {
	if !value.IsValid() {
		return reflect.Zero(typ)
	}
	converted, err := prepareArgErr(value, typ)
	if err != nil {
		return value
	}
	return converted
}

func prepareArgErr(value reflect.Value, typ reflect.Type) (reflect.Value, error) {
	if !value.IsValid() {
		if canBeNil(typ) {
			return reflect.Zero(typ), nil
		}
		return reflect.Zero(typ), fmt.Errorf("value is nil; should be of type %s", typ)
	}
	if value.Type().AssignableTo(typ) {
		return value, nil
	}
	if intLike(value.Kind()) && intLike(typ.Kind()) && value.Type().ConvertibleTo(typ) {
		return value.Convert(typ), nil
	}
	return reflect.Value{}, fmt.Errorf("value has type %s; should be %s", value.Type(), typ)
}

func intLike(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func canBeNil(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	}
	return false
}

// goodFunc reports why a function with the return signature typ cannot be
// called, naming it as name does, or nil if it can. It allows one result, or
// two where the second is an error, as text/template does.
func goodFunc(name string, typ reflect.Type) error {
	switch numOut := typ.NumOut(); {
	case numOut == 1:
		return nil
	case numOut == 2 && typ.Out(1) == errorType:
		return nil
	case numOut == 2:
		return fmt.Errorf("invalid function signature for %s: second return value should be error; is %s", name, typ.Out(1))
	default:
		return fmt.Errorf("function %s has %d return values; should be 1 or 2", name, numOut)
	}
}

var errorType = reflect.TypeFor[error]()

// indirect dereferences v through its pointers and interfaces and reports
// whether it met a nil one.
func indirect(v reflect.Value) (rv reflect.Value, isNil bool) {
	for ; v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface; v = v.Elem() {
		if v.IsNil() {
			return v, true
		}
	}
	return v, false
}

// indirectInterface returns the concrete value in an interface value.
func indirectInterface(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Interface {
		return v
	}
	if v.IsNil() {
		return reflect.Value{}
	}
	return v.Elem()
}
