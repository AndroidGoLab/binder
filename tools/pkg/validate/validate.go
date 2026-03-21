package validate

import (
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

// validPrimitiveBackingTypes is the set of valid backing types for AIDL enums.
var validPrimitiveBackingTypes = map[string]bool{
	"byte": true,
	"int":  true,
	"long": true,
}

// Validate performs semantic validation on a parsed document.
// It checks that:
//   - All type references can be resolved (are built-in or found by lookupType)
//   - Parameter directions are valid (in/out/inout on interface methods)
//   - Oneway methods return void and have only 'in' parameters
//   - Enum backing types are valid primitives
func Validate(
	doc *parser.Document,
	lookupType func(qualifiedName string) bool,
) []error {
	v := &validator{
		doc:        doc,
		lookupType: lookupType,
		pkg:        packageName(doc),
	}
	v.validate()
	return v.errs
}

type validator struct {
	doc        *parser.Document
	lookupType func(string) bool
	pkg        string
	errs       []error
}

func (v *validator) addError(
	pos parser.Position,
	msg string,
) {
	v.errs = append(v.errs, &ValidationError{Pos: pos, Message: msg})
}

func (v *validator) validate() {
	v.validateDefinitions(v.doc.Definitions)
}

func (v *validator) validateDefinitions(defs []parser.Definition) {
	for _, def := range defs {
		switch d := def.(type) {
		case *parser.InterfaceDecl:
			v.validateInterface(d)
			v.validateDefinitions(d.NestedTypes)
		case *parser.ParcelableDecl:
			v.validateParcelable(d)
			v.validateDefinitions(d.NestedTypes)
		case *parser.EnumDecl:
			v.validateEnum(d)
		case *parser.UnionDecl:
			v.validateUnion(d)
			v.validateDefinitions(d.NestedTypes)
		}
	}
}

func (v *validator) validateInterface(d *parser.InterfaceDecl) {
	for _, m := range d.Methods {
		v.validateTypeSpec(m.ReturnType)

		oneway := m.Oneway || d.Oneway
		if oneway {
			if m.ReturnType.Name != "void" {
				v.addError(m.Pos, "oneway method must return void")
			}
		}

		for _, p := range m.Params {
			v.validateTypeSpec(p.Type)

			if p.Direction == parser.DirectionNone {
				v.addError(p.Pos, "interface method parameter must have a direction (in, out, or inout)")
			}

			if oneway && p.Direction != parser.DirectionIn {
				v.addError(p.Pos, "oneway method parameters must be 'in'")
			}
		}
	}

	for _, c := range d.Constants {
		v.validateTypeSpec(c.Type)
	}
}

func (v *validator) validateParcelable(d *parser.ParcelableDecl) {
	for _, f := range d.Fields {
		v.validateTypeSpec(f.Type)
	}

	for _, c := range d.Constants {
		v.validateTypeSpec(c.Type)
	}
}

func (v *validator) validateEnum(d *parser.EnumDecl) {
	if d.BackingType != nil {
		if !validPrimitiveBackingTypes[d.BackingType.Name] {
			v.addError(d.BackingType.Pos, "enum backing type must be byte, int, or long")
		}
	}
}

func (v *validator) validateUnion(d *parser.UnionDecl) {
	for _, f := range d.Fields {
		v.validateTypeSpec(f.Type)
	}

	for _, c := range d.Constants {
		v.validateTypeSpec(c.Type)
	}
}

func (v *validator) validateTypeSpec(ts *parser.TypeSpecifier) {
	if ts == nil {
		return
	}

	name := ts.Name

	// Built-in types are always valid.
	if IsBuiltin(name) {
		for _, arg := range ts.TypeArgs {
			v.validateTypeSpec(arg)
		}
		return
	}

	// Generic container types: List and Map are valid with type args.
	if name == "List" || name == "Map" {
		for _, arg := range ts.TypeArgs {
			v.validateTypeSpec(arg)
		}
		return
	}

	// Try qualified name lookup.
	if v.resolveTypeName(name, ts.Pos) {
		for _, arg := range ts.TypeArgs {
			v.validateTypeSpec(arg)
		}
		return
	}

	v.addError(ts.Pos, "unresolved type: "+name)
}

// resolveTypeName tries to find the type by its name, both as given and qualified
// with the document's package prefix. Returns true if found.
func (v *validator) resolveTypeName(
	name string,
	pos parser.Position,
) bool {
	if v.lookupType == nil {
		return false
	}

	// Try the name as-is (fully qualified).
	if ok := v.lookupType(name); ok {
		return true
	}

	// Try qualifying with current package.
	if v.pkg != "" {
		qualified := v.pkg + "." + name
		if ok := v.lookupType(qualified); ok {
			return true
		}
	}

	// Try via imports.
	for _, imp := range v.doc.Imports {
		// Import name ends with the simple name.
		lastDot := strings.LastIndexByte(imp.Name, '.')
		if lastDot >= 0 && imp.Name[lastDot+1:] == name {
			if ok := v.lookupType(imp.Name); ok {
				return true
			}
		}
	}

	return false
}

func packageName(doc *parser.Document) string {
	if doc.Package != nil {
		return doc.Package.Name
	}
	return ""
}
