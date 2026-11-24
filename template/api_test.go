package template_test

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"
	"testing"
)

// This package's API holds the API of text/template and of html/template:
// every exported name either declares, with the same type, and every field
// and method that can be selected on a value of one of their types,
// promoted ones included. The standard library is the one that runs the
// test, so a name a later Go adds fails here until this package has it.
func TestAPIHoldsTheStandardLibrarys(t *testing.T) {
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	candidate, err := imp.Import("github.com/typelate/htt/template")
	if err != nil {
		t.Fatal(err)
	}
	have := describeAPI(candidate)
	for _, path := range []string{"text/template", "html/template"} {
		reference, err := imp.Import(path)
		if err != nil {
			t.Fatal(err)
		}
		want := describeAPI(reference)
		for _, member := range slices.Sorted(maps.Keys(want)) {
			got, ok := have[member]
			switch {
			case !ok:
				t.Errorf("%s has %s (%s); this package does not", path, member, want[member])
			case got != want[member]:
				t.Errorf("%s declares %s as %s; this package declares it as %s", path, member, want[member], got)
			}
		}
	}
}

// describeAPI returns a description of each exported member of pkg, keyed
// by Name for a package-level name and Type.Member for a field or method.
func describeAPI(pkg *types.Package) map[string]string {
	out := make(map[string]string)
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Const:
			out[name] = fmt.Sprintf("const %s = %s", typeString(obj.Type()), obj.Val().ExactString())
		case *types.Var:
			out[name] = "var " + typeString(obj.Type())
		case *types.Func:
			out[name] = typeString(obj.Type())
		case *types.TypeName:
			describeAPIType(out, pkg, obj)
		}
	}
	return out
}

func describeAPIType(out map[string]string, pkg *types.Package, obj *types.TypeName) {
	name := obj.Name()
	typ := types.Unalias(obj.Type())
	if _, isStruct := typ.Underlying().(*types.Struct); isStruct {
		out[name] = "a struct type"
	} else {
		out[name] = "type " + typeString(typ.Underlying())
	}
	for _, recv := range []types.Type{typ, types.NewPointer(typ)} {
		methods := types.NewMethodSet(recv)
		for i := range methods.Len() {
			method := methods.At(i)
			if method.Obj().Exported() {
				out[name+"."+method.Obj().Name()] = "method " + typeString(method.Type())
			}
		}
	}
	for _, field := range selectableFields(typ, pkg) {
		out[name+"."+field.Name()] = "field " + typeString(field.Type())
	}
}

// typeString prints t with each named type by its name alone, so that
// parse.Tree and htt.Tree compare equal, and a function without the names
// of its parameters: text/template and html/template name ParseFS's first
// parameter differently, and a name is not part of a signature.
func typeString(t types.Type) string {
	sig, ok := t.(*types.Signature)
	if !ok {
		return types.TypeString(t, func(*types.Package) string { return "" })
	}
	params := make([]string, sig.Params().Len())
	for i := range params {
		param := sig.Params().At(i).Type()
		if sig.Variadic() && i == len(params)-1 {
			params[i] = "..." + typeString(param.(*types.Slice).Elem())
			continue
		}
		params[i] = typeString(param)
	}
	results := make([]string, sig.Results().Len())
	for i := range results {
		results[i] = typeString(sig.Results().At(i).Type())
	}
	s := "func(" + strings.Join(params, ", ") + ")"
	switch len(results) {
	case 0:
		return s
	case 1:
		return s + " " + results[0]
	}
	return s + " (" + strings.Join(results, ", ") + ")"
}

// selectableFields returns the exported fields a selector on a value of
// type typ reaches, promoted ones included.
func selectableFields(typ types.Type, pkg *types.Package) []*types.Var {
	seen := make(map[string]bool)
	var collect func(t types.Type, depth int)
	collect = func(t types.Type, depth int) {
		if depth > 4 {
			return
		}
		if p, ok := types.Unalias(t).(*types.Pointer); ok {
			t = p.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return
		}
		for i := range st.NumFields() {
			f := st.Field(i)
			if f.Exported() {
				seen[f.Name()] = true
			}
			if f.Embedded() {
				collect(f.Type(), depth+1)
			}
		}
	}
	collect(typ, 0)
	var fields []*types.Var
	for _, name := range slices.Sorted(maps.Keys(seen)) {
		obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(typ), true, pkg, name)
		if v, ok := obj.(*types.Var); ok && v.IsField() {
			fields = append(fields, v)
		}
	}
	return fields
}
