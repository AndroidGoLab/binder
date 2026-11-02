package codegen

import "github.com/AndroidGoLab/binder/tools/pkg/parser"

// methodNeedsNativeContext reports whether a method serializes a native parcelable.
func methodNeedsNativeContext(m *parser.MethodDecl, opts GenOptions) bool {
	if opts.Registry == nil {
		return false
	}
	seen := make(map[string]bool)
	if typeNeedsNativeContext(m.ReturnType, opts, seen) {
		return true
	}
	for _, p := range m.Params {
		if typeNeedsNativeContext(p.Type, opts, seen) {
			return true
		}
	}
	return false
}

func typeNeedsNativeContext(ts *parser.TypeSpecifier, opts GenOptions, seen map[string]bool) bool {
	if ts == nil {
		return false
	}
	for _, arg := range ts.TypeArgs {
		if typeNeedsNativeContext(arg, opts, seen) {
			return true
		}
	}
	if _, primitive := aidlPrimitiveToGo[ts.Name]; primitive || ts.Name == "List" || ts.Name == "Map" {
		return false
	}
	r := NewTypeRefResolver(opts.Registry, opts.CurrentPkg, NewGoFile("reachability"))
	r.CurrentDef = opts.CurrentDef
	name := r.resolveUserQualifiedName(ts.Name)
	if name == "" || seen[name] {
		return false
	}
	def, ok := opts.Registry.Lookup(name)
	if !ok {
		return false
	}
	seen[name] = true
	opts.CurrentPkg = packageFromDef(name, def.GetName())
	opts.CurrentDef = name
	switch d := def.(type) {
	case *parser.ParcelableDecl:
		if d.NativeParcelable {
			return true
		}
		if len(d.JavaWireFormat) > 0 {
			return javaWireNeedsNativeContext(d.JavaWireFormat, opts, seen)
		}
		for _, f := range d.Fields {
			if typeNeedsNativeContext(f.Type, opts, seen) {
				return true
			}
		}
	case *parser.UnionDecl:
		for _, f := range d.Fields {
			if typeNeedsNativeContext(f.Type, opts, seen) {
				return true
			}
		}
	}
	return false
}

func javaWireNeedsNativeContext(fields []parser.JavaWireField, opts GenOptions, seen map[string]bool) bool {
	for _, f := range fields {
		if (f.WriteMethod == "delegate" || f.WriteMethod == "typed_object") && f.GoType != "" && typeNeedsNativeContext(&parser.TypeSpecifier{Name: f.GoType}, opts, seen) {
			return true
		}
		if javaWireNeedsNativeContext(f.Elements, opts, seen) {
			return true
		}
	}
	return false
}
