package codegen

import (
	"testing"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
)

func TestMethodNativeContextReachability(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("example.FutureNative", &parser.ParcelableDecl{ParcName: "FutureNative", NativeParcelable: true})
	registry.Register("example.Inner", &parser.ParcelableDecl{ParcName: "Inner", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "FutureNative"}}}})
	registry.Register("example.Choice", &parser.UnionDecl{UnionName: "Choice", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "Inner"}}}})
	registry.Register("example.CycleA", &parser.ParcelableDecl{ParcName: "CycleA", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "CycleB"}}}})
	registry.Register("example.CycleB", &parser.ParcelableDecl{ParcName: "CycleB", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "CycleA"}}, {Type: &parser.TypeSpecifier{Name: "FutureNative"}}}})
	registry.Register("example.Java", &parser.ParcelableDecl{ParcName: "Java", JavaWireFormat: []parser.JavaWireField{{WriteMethod: "repeated", Elements: []parser.JavaWireField{{WriteMethod: "delegate", GoType: "example.FutureNative"}}}}})
	registry.Register("example.JavaOverride", &parser.ParcelableDecl{ParcName: "JavaOverride", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "FutureNative"}}}, JavaWireFormat: []parser.JavaWireField{{WriteMethod: "int32"}}})
	registry.Register("example.JavaTyped", &parser.ParcelableDecl{ParcName: "JavaTyped", JavaWireFormat: []parser.JavaWireField{{WriteMethod: "typed_object", GoType: "example.FutureNative"}}})
	registry.Register("example.EmptyJava", &parser.ParcelableDecl{ParcName: "EmptyJava", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "FutureNative"}}}, JavaWireFormat: []parser.JavaWireField{}})
	registry.Register("example.JavaDelegate", &parser.ParcelableDecl{ParcName: "JavaDelegate", JavaWireFormat: []parser.JavaWireField{{WriteMethod: "delegate", GoType: "example.FutureNative"}}})
	registry.Register("example.ISeparate", &parser.InterfaceDecl{IntfName: "ISeparate", Methods: []*parser.MethodDecl{{ReturnType: &parser.TypeSpecifier{Name: "FutureNative"}}}})
	registry.Register("example.PlainCycleA", &parser.ParcelableDecl{ParcName: "PlainCycleA", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "PlainCycleB"}}}})
	registry.Register("example.PlainCycleB", &parser.ParcelableDecl{ParcName: "PlainCycleB", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "PlainCycleA"}}}})
	registry.Register("other.Inner", &parser.ParcelableDecl{ParcName: "Inner"})
	registry.Register("example.Outer.Nested", &parser.ParcelableDecl{ParcName: "Nested", Fields: []*parser.FieldDecl{{Type: &parser.TypeSpecifier{Name: "Sibling"}}}})
	registry.Register("example.Outer.Sibling", &parser.ParcelableDecl{ParcName: "Sibling", NativeParcelable: true})
	registry.Register("example.Sibling", &parser.ParcelableDecl{ParcName: "Sibling"})
	opts := GenOptions{Registry: registry, CurrentPkg: "example", CurrentDef: "example.IExample"}
	cases := []struct {
		name string
		typ  *parser.TypeSpecifier
		want bool
	}{
		{"scalar", &parser.TypeSpecifier{Name: "int"}, false},
		{"direct", &parser.TypeSpecifier{Name: "FutureNative"}, true},
		{"nested", &parser.TypeSpecifier{Name: "Inner"}, true},
		{"union", &parser.TypeSpecifier{Name: "Choice"}, true},
		{"cycle", &parser.TypeSpecifier{Name: "CycleA"}, true},
		{"cycle without native", &parser.TypeSpecifier{Name: "PlainCycleA"}, false},
		{"qualified shadow", &parser.TypeSpecifier{Name: "other.Inner"}, false},
		{"nested lexical shadow", &parser.TypeSpecifier{Name: "example.Outer.Nested"}, true},
		{"array", &parser.TypeSpecifier{Name: "Inner", IsArray: true}, true},
		{"list", &parser.TypeSpecifier{Name: "List", TypeArgs: []*parser.TypeSpecifier{{Name: "Inner"}}}, true},
		{"map", &parser.TypeSpecifier{Name: "Map", TypeArgs: []*parser.TypeSpecifier{{Name: "String"}, {Name: "Inner"}}}, true},
		{"java delegate", &parser.TypeSpecifier{Name: "Java"}, true},
		{"java typed object", &parser.TypeSpecifier{Name: "JavaTyped"}, true},
		{"empty java wire", &parser.TypeSpecifier{Name: "EmptyJava"}, true},
		{"direct java delegate", &parser.TypeSpecifier{Name: "JavaDelegate"}, true},
		{"java wire override", &parser.TypeSpecifier{Name: "JavaOverride"}, false},
		{"interface", &parser.TypeSpecifier{Name: "ISeparate"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &parser.MethodDecl{ReturnType: tc.typ}
			if got := methodNeedsNativeContext(m, opts); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	param := &parser.MethodDecl{Params: []*parser.ParamDecl{{Type: &parser.TypeSpecifier{Name: "FutureNative"}, Direction: parser.DirectionOut}}}
	if !methodNeedsNativeContext(param, opts) {
		t.Fatal("out parameters must propagate native context")
	}
}
