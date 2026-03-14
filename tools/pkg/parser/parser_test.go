package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSimpleInterface(t *testing.T) {
	doc, err := ParseFile("testdata/simple_interface.aidl")
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, "com.example", doc.Package.Name)
	require.Len(t, doc.Imports, 1)
	assert.Equal(t, "android.os.IBinder", doc.Imports[0].Name)

	require.Len(t, doc.Definitions, 1)
	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	assert.Equal(t, "ISimpleService", iface.IntfName)
	assert.False(t, iface.Oneway)

	// Constants.
	require.Len(t, iface.Constants, 1)
	assert.Equal(t, "VERSION", iface.Constants[0].ConstName)
	assert.Equal(t, "int", iface.Constants[0].Type.Name)
	intLit, ok := iface.Constants[0].Value.(*IntegerLiteral)
	require.True(t, ok)
	assert.Equal(t, "1", intLit.Value)

	// Methods.
	require.Len(t, iface.Methods, 5)

	// void doSomething(in String name, int value);
	m := iface.Methods[0]
	assert.Equal(t, "doSomething", m.MethodName)
	assert.Equal(t, "void", m.ReturnType.Name)
	assert.False(t, m.Oneway)
	require.Len(t, m.Params, 2)
	assert.Equal(t, DirectionIn, m.Params[0].Direction)
	assert.Equal(t, "String", m.Params[0].Type.Name)
	assert.Equal(t, "name", m.Params[0].ParamName)
	assert.Equal(t, DirectionNone, m.Params[1].Direction)
	assert.Equal(t, "int", m.Params[1].Type.Name)
	assert.Equal(t, "value", m.Params[1].ParamName)

	// String getName();
	m = iface.Methods[1]
	assert.Equal(t, "getName", m.MethodName)
	assert.Equal(t, "String", m.ReturnType.Name)
	assert.Len(t, m.Params, 0)

	// oneway void fireAndForget(in String message);
	m = iface.Methods[2]
	assert.Equal(t, "fireAndForget", m.MethodName)
	assert.True(t, m.Oneway)
	assert.Equal(t, "void", m.ReturnType.Name)

	// @nullable IBinder getRemote();
	m = iface.Methods[3]
	assert.Equal(t, "getRemote", m.MethodName)
	require.Len(t, m.Annots, 1)
	assert.Equal(t, "nullable", m.Annots[0].Name)
	assert.Equal(t, "IBinder", m.ReturnType.Name)

	// List<String> getItems(int offset, int count);
	m = iface.Methods[4]
	assert.Equal(t, "getItems", m.MethodName)
	assert.Equal(t, "List", m.ReturnType.Name)
	require.Len(t, m.ReturnType.TypeArgs, 1)
	assert.Equal(t, "String", m.ReturnType.TypeArgs[0].Name)
}

func TestParseSimpleParcelable(t *testing.T) {
	doc, err := ParseFile("testdata/simple_parcelable.aidl")
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, "com.example", doc.Package.Name)

	require.Len(t, doc.Definitions, 1)
	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)
	assert.Equal(t, "SimpleData", parc.ParcName)

	require.Len(t, parc.Fields, 6)

	// int id;
	assert.Equal(t, "id", parc.Fields[0].FieldName)
	assert.Equal(t, "int", parc.Fields[0].Type.Name)
	assert.Nil(t, parc.Fields[0].DefaultValue)

	// String name = "default";
	assert.Equal(t, "name", parc.Fields[1].FieldName)
	assert.Equal(t, "String", parc.Fields[1].Type.Name)
	strLit, ok := parc.Fields[1].DefaultValue.(*StringLiteralExpr)
	require.True(t, ok)
	assert.Equal(t, "default", strLit.Value)

	// boolean active = true;
	assert.Equal(t, "active", parc.Fields[2].FieldName)
	assert.Equal(t, "boolean", parc.Fields[2].Type.Name)
	boolLit, ok := parc.Fields[2].DefaultValue.(*BoolLiteral)
	require.True(t, ok)
	assert.True(t, boolLit.Value)

	// long timestamp;
	assert.Equal(t, "timestamp", parc.Fields[3].FieldName)
	assert.Equal(t, "long", parc.Fields[3].Type.Name)

	// @nullable String description;
	assert.Equal(t, "description", parc.Fields[4].FieldName)
	require.Len(t, parc.Fields[4].Annots, 1)
	assert.Equal(t, "nullable", parc.Fields[4].Annots[0].Name)

	// int[] scores;
	assert.Equal(t, "scores", parc.Fields[5].FieldName)
	assert.Equal(t, "int", parc.Fields[5].Type.Name)
	assert.True(t, parc.Fields[5].Type.IsArray)
}

func TestParseGenerics(t *testing.T) {
	doc, err := ParseFile("testdata/generics.aidl")
	require.NoError(t, err)
	require.NotNil(t, doc)

	require.Len(t, doc.Definitions, 1)
	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)

	require.Len(t, parc.Fields, 3)

	// List<String> names;
	f := parc.Fields[0]
	assert.Equal(t, "names", f.FieldName)
	assert.Equal(t, "List", f.Type.Name)
	require.Len(t, f.Type.TypeArgs, 1)
	assert.Equal(t, "String", f.Type.TypeArgs[0].Name)

	// Map<String, int> values;
	f = parc.Fields[1]
	assert.Equal(t, "values", f.FieldName)
	assert.Equal(t, "Map", f.Type.Name)
	require.Len(t, f.Type.TypeArgs, 2)
	assert.Equal(t, "String", f.Type.TypeArgs[0].Name)
	assert.Equal(t, "int", f.Type.TypeArgs[1].Name)

	// @nullable List<IBinder> binders;
	f = parc.Fields[2]
	assert.Equal(t, "binders", f.FieldName)
	assert.Equal(t, "List", f.Type.Name)
	require.Len(t, f.Type.TypeArgs, 1)
	assert.Equal(t, "IBinder", f.Type.TypeArgs[0].Name)
}

func TestParseEnum(t *testing.T) {
	src := `package com.example;

@Backing(type="int")
enum Color {
    RED = 0,
    GREEN = 1,
    BLUE = 2,
}
`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	enum, ok := doc.Definitions[0].(*EnumDecl)
	require.True(t, ok)
	assert.Equal(t, "Color", enum.EnumName)
	require.NotNil(t, enum.BackingType)
	assert.Equal(t, "int", enum.BackingType.Name)

	require.Len(t, enum.Enumerators, 3)
	assert.Equal(t, "RED", enum.Enumerators[0].Name)
	assert.Equal(t, "GREEN", enum.Enumerators[1].Name)
	assert.Equal(t, "BLUE", enum.Enumerators[2].Name)

	intLit, ok := enum.Enumerators[0].Value.(*IntegerLiteral)
	require.True(t, ok)
	assert.Equal(t, "0", intLit.Value)
}

func TestParseEnumTrailingComma(t *testing.T) {
	src := `enum Foo { A, B, C, }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	enum, ok := doc.Definitions[0].(*EnumDecl)
	require.True(t, ok)
	assert.Len(t, enum.Enumerators, 3)
}

func TestParseEnumNoTrailingComma(t *testing.T) {
	src := `enum Foo { A, B, C }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	enum, ok := doc.Definitions[0].(*EnumDecl)
	require.True(t, ok)
	assert.Len(t, enum.Enumerators, 3)
}

func TestParseUnion(t *testing.T) {
	src := `package com.example;

union MyUnion {
    int intValue;
    String stringValue;
    boolean boolValue = false;
}
`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	u, ok := doc.Definitions[0].(*UnionDecl)
	require.True(t, ok)
	assert.Equal(t, "MyUnion", u.UnionName)
	require.Len(t, u.Fields, 3)

	assert.Equal(t, "intValue", u.Fields[0].FieldName)
	assert.Equal(t, "int", u.Fields[0].Type.Name)
	assert.Equal(t, "stringValue", u.Fields[1].FieldName)
	assert.Equal(t, "boolValue", u.Fields[2].FieldName)

	boolLit, ok := u.Fields[2].DefaultValue.(*BoolLiteral)
	require.True(t, ok)
	assert.False(t, boolLit.Value)
}

func TestParseAnnotationWithParams(t *testing.T) {
	src := `@Backing(type="byte")
enum Status { OK = 0 }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	enum, ok := doc.Definitions[0].(*EnumDecl)
	require.True(t, ok)
	require.Len(t, enum.Annots, 1)
	assert.Equal(t, "Backing", enum.Annots[0].Name)
	require.Contains(t, enum.Annots[0].Params, "type")

	strLit, ok := enum.Annots[0].Params["type"].(*StringLiteralExpr)
	require.True(t, ok)
	assert.Equal(t, "byte", strLit.Value)
}

func TestParseAnnotationWithoutParams(t *testing.T) {
	src := `interface IFoo {
    @nullable String bar();
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 1)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	require.Len(t, iface.Methods, 1)
	require.Len(t, iface.Methods[0].Annots, 1)
	assert.Equal(t, "nullable", iface.Methods[0].Annots[0].Name)
	assert.Nil(t, iface.Methods[0].Annots[0].Params)
}

func TestParseArrayTypes(t *testing.T) {
	src := `parcelable Foo {
    int[] ids;
    String[] names;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)

	assert.True(t, parc.Fields[0].Type.IsArray)
	assert.Equal(t, "int", parc.Fields[0].Type.Name)
	assert.True(t, parc.Fields[1].Type.IsArray)
	assert.Equal(t, "String", parc.Fields[1].Type.Name)
}

func TestParseOnewayInterface(t *testing.T) {
	src := `oneway interface ICallback {
    void onResult(int code);
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	assert.True(t, iface.Oneway)
	assert.Equal(t, "ICallback", iface.IntfName)
}

func TestParseParameterDirections(t *testing.T) {
	src := `interface IFoo {
    void bar(in int a, out int b, inout int c, int d);
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	require.Len(t, iface.Methods, 1)

	params := iface.Methods[0].Params
	require.Len(t, params, 4)
	assert.Equal(t, DirectionIn, params[0].Direction)
	assert.Equal(t, DirectionOut, params[1].Direction)
	assert.Equal(t, DirectionInOut, params[2].Direction)
	assert.Equal(t, DirectionNone, params[3].Direction)
}

func TestParseConstantExpressions(t *testing.T) {
	src := `interface IFoo {
    const int A = 1 + 2;
    const int B = 0xFF;
    const int C = 1 << 4;
    const int D = (1 + 2) * 3;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	require.Len(t, iface.Constants, 4)

	// 1 + 2
	binExpr, ok := iface.Constants[0].Value.(*BinaryExpr)
	require.True(t, ok)
	assert.Equal(t, TokenPlus, binExpr.Op)

	// 0xFF
	intLit, ok := iface.Constants[1].Value.(*IntegerLiteral)
	require.True(t, ok)
	assert.Equal(t, "0xFF", intLit.Value)

	// 1 << 4
	binExpr, ok = iface.Constants[2].Value.(*BinaryExpr)
	require.True(t, ok)
	assert.Equal(t, TokenLShift, binExpr.Op)
}

func TestParseMethodTransactionID(t *testing.T) {
	src := `interface IFoo {
    void bar() = 1;
    void baz() = 42;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	require.Len(t, iface.Methods, 2)
	assert.Equal(t, 1, iface.Methods[0].TransactionID)
	assert.Equal(t, 42, iface.Methods[1].TransactionID)
}

func TestParseForwardDeclaredParcelable(t *testing.T) {
	src := `parcelable Foo;`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)
	assert.Equal(t, "Foo", parc.ParcName)
	assert.Len(t, parc.Fields, 0)
}

func TestParseForwardDeclaredParcelableWithCppHeader(t *testing.T) {
	src := `parcelable Foo cpp_header "foo/bar.h";`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)
	assert.Equal(t, "Foo", parc.ParcName)
	assert.Equal(t, "foo/bar.h", parc.CppHeader)
}

func TestParseUnionWithConstants(t *testing.T) {
	src := `union MyUnion {
    const int TAG_INT = 0;
    int intValue;
    String stringValue;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	u, ok := doc.Definitions[0].(*UnionDecl)
	require.True(t, ok)
	assert.Len(t, u.Constants, 1)
	assert.Equal(t, "TAG_INT", u.Constants[0].ConstName)
	assert.Len(t, u.Fields, 2)
}

func TestParseQualifiedType(t *testing.T) {
	src := `parcelable Foo {
    android.os.IBinder binder;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)
	assert.Equal(t, "android.os.IBinder", parc.Fields[0].Type.Name)
}

func TestParseMultipleDefinitions(t *testing.T) {
	src := `package com.example;

parcelable Data {
    int x;
}

interface IService {
    Data getData();
}
`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	require.Len(t, doc.Definitions, 2)

	_, ok := doc.Definitions[0].(*ParcelableDecl)
	assert.True(t, ok)
	_, ok = doc.Definitions[1].(*InterfaceDecl)
	assert.True(t, ok)
}

func TestParseErrorBadToken(t *testing.T) {
	src := `package com.example;
42 broken`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestDefinitionInterface(t *testing.T) {
	iface := &InterfaceDecl{IntfName: "IFoo", Annots: []*Annotation{{Name: "a"}}}
	assert.Equal(t, "IFoo", iface.GetName())
	assert.Len(t, iface.GetAnnotations(), 1)

	parc := &ParcelableDecl{ParcName: "Bar"}
	assert.Equal(t, "Bar", parc.GetName())
	assert.Nil(t, parc.GetAnnotations())

	enum := &EnumDecl{EnumName: "Color"}
	assert.Equal(t, "Color", enum.GetName())

	union := &UnionDecl{UnionName: "Variant"}
	assert.Equal(t, "Variant", union.GetName())
}

func TestParseParcelableWithConstants(t *testing.T) {
	src := `parcelable Foo {
    const int MAX = 100;
    int value;
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc, ok := doc.Definitions[0].(*ParcelableDecl)
	require.True(t, ok)
	assert.Len(t, parc.Constants, 1)
	assert.Equal(t, "MAX", parc.Constants[0].ConstName)
	assert.Len(t, parc.Fields, 1)
}

func TestParseNoPackage(t *testing.T) {
	src := `interface IFoo { void bar(); }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	assert.Nil(t, doc.Package)
	require.Len(t, doc.Definitions, 1)
}

func TestParseEmptyFile(t *testing.T) {
	doc, err := Parse("test.aidl", []byte(""))
	require.NoError(t, err)
	assert.Nil(t, doc.Package)
	assert.Len(t, doc.Definitions, 0)
}

func TestParseCharLiteralExpr(t *testing.T) {
	src := `parcelable Foo { char c = 'A'; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	charLit, ok := parc.Fields[0].DefaultValue.(*CharLiteralExpr)
	require.True(t, ok)
	assert.Equal(t, "A", charLit.Value)
}

func TestParseNullDefault(t *testing.T) {
	src := `parcelable Foo { @nullable String s = null; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	_, ok := parc.Fields[0].DefaultValue.(*NullLiteral)
	assert.True(t, ok)
}

func TestParseFloatDefault(t *testing.T) {
	src := `parcelable Foo { float f = 3.14; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	floatLit, ok := parc.Fields[0].DefaultValue.(*FloatLiteral)
	require.True(t, ok)
	assert.Equal(t, "3.14", floatLit.Value)
}

func TestParseGroupedExpression(t *testing.T) {
	src := `interface T { const int X = (42); }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	intLit, ok := iface.Constants[0].Value.(*IntegerLiteral)
	require.True(t, ok)
	assert.Equal(t, "42", intLit.Value)
}

func TestParseQualifiedIdentExpr(t *testing.T) {
	src := `interface T { const int X = Foo.BAR; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	ident, ok := iface.Constants[0].Value.(*IdentExpr)
	require.True(t, ok)
	assert.Equal(t, "Foo.BAR", ident.Name)
}

func TestParseErrorMissingSemicolon(t *testing.T) {
	src := `package com.example`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected ;")
}

func TestParseErrorBadDefinition(t *testing.T) {
	src := `package com.example; foobar`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected definition")
}

func TestParseAnnotatedField(t *testing.T) {
	src := `parcelable Foo { @utf8InCpp String s; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	// The annotation is on the field, consumed by parseField before parseTypeSpecifier.
	require.Len(t, parc.Fields[0].Annots, 1)
	assert.Equal(t, "utf8InCpp", parc.Fields[0].Annots[0].Name)
}

func TestDirectionString(t *testing.T) {
	assert.Equal(t, "in", DirectionIn.String())
	assert.Equal(t, "out", DirectionOut.String())
	assert.Equal(t, "inout", DirectionInOut.String())
	assert.Equal(t, "", DirectionNone.String())
}

func TestConstExprPos(t *testing.T) {
	pos := Position{Filename: "f.aidl", Line: 1, Column: 1}

	assert.Equal(t, pos, (&IntegerLiteral{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&FloatLiteral{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&StringLiteralExpr{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&CharLiteralExpr{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&BoolLiteral{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&NullLiteral{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&IdentExpr{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&UnaryExpr{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&BinaryExpr{TokenPos: pos}).ExprPos())
	assert.Equal(t, pos, (&TernaryExpr{TokenPos: pos}).ExprPos())
}

func TestParseEnumAutoIncrement(t *testing.T) {
	src := `enum Foo { A, B = 5, C }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	enum := doc.Definitions[0].(*EnumDecl)
	require.Len(t, enum.Enumerators, 3)
	assert.Nil(t, enum.Enumerators[0].Value)
	assert.NotNil(t, enum.Enumerators[1].Value)
	assert.Nil(t, enum.Enumerators[2].Value)
}

func TestDefinitionNodeMarkers(t *testing.T) {
	// These are compile-time interface satisfaction markers.
	// Calling them covers the trivial constExprNode/definitionNode methods.
	var _ Definition = (*InterfaceDecl)(nil)
	var _ Definition = (*ParcelableDecl)(nil)
	var _ Definition = (*EnumDecl)(nil)
	var _ Definition = (*UnionDecl)(nil)

	// Cover the definitionNode() and GetAnnotations() methods.
	(&InterfaceDecl{}).definitionNode()
	(&ParcelableDecl{}).definitionNode()
	(&EnumDecl{}).definitionNode()
	(&UnionDecl{}).definitionNode()

	assert.Nil(t, (&EnumDecl{}).GetAnnotations())
	assert.Nil(t, (&UnionDecl{}).GetAnnotations())
}

func TestConstExprNodeMarkers(t *testing.T) {
	// Cover the constExprNode() marker methods.
	(&IntegerLiteral{}).constExprNode()
	(&FloatLiteral{}).constExprNode()
	(&StringLiteralExpr{}).constExprNode()
	(&CharLiteralExpr{}).constExprNode()
	(&BoolLiteral{}).constExprNode()
	(&NullLiteral{}).constExprNode()
	(&IdentExpr{}).constExprNode()
	(&UnaryExpr{}).constExprNode()
	(&BinaryExpr{}).constExprNode()
	(&TernaryExpr{}).constExprNode()
}

func TestParseBraceInitializer(t *testing.T) {
	src := `parcelable Foo { int[] a = {1, 2, 3}; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	assert.NotNil(t, parc.Fields[0].DefaultValue)
}

func TestParseBraceInitializerSingle(t *testing.T) {
	src := `parcelable Foo { int[] a = {42}; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	intLit, ok := parc.Fields[0].DefaultValue.(*IntegerLiteral)
	require.True(t, ok)
	assert.Equal(t, "42", intLit.Value)
}

func TestParseBraceInitializerEmpty(t *testing.T) {
	src := `parcelable Foo { int[] a = {}; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	ident, ok := parc.Fields[0].DefaultValue.(*IdentExpr)
	require.True(t, ok)
	assert.Equal(t, "{}", ident.Name)
}

func TestParseBraceInitializerTrailingComma(t *testing.T) {
	src := `parcelable Foo { int[] a = {1, 2,}; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	assert.NotNil(t, parc.Fields[0].DefaultValue)
}

func TestParseErrorInConstExpr(t *testing.T) {
	src := `interface T { const int X = ; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected constant expression")
}

func TestParseErrorInImport(t *testing.T) {
	src := `import ;`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInInterface(t *testing.T) {
	// Missing closing brace.
	src := `interface IFoo { void bar();`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInEnum(t *testing.T) {
	src := `enum Foo {`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInUnion(t *testing.T) {
	src := `union Foo {`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInConstant(t *testing.T) {
	src := `interface T { const int; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseFileNotExist(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.aidl")
	assert.Error(t, err)
}

func TestParseAnnotationMultipleParams(t *testing.T) {
	src := `@JavaPassthrough(annotation="@Foo", value="bar")
interface IFoo { void bar(); }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	require.Len(t, iface.Annots, 1)
	assert.Len(t, iface.Annots[0].Params, 2)
}

func TestParseAllExprOperators(t *testing.T) {
	// Exercises every binary operator precedence level through parsing.
	tests := []struct {
		name string
		expr string
	}{
		{"logical_or", "0 || 1"},
		{"logical_and", "1 && 1"},
		{"bitwise_or", "1 | 2"},
		{"bitwise_xor", "1 ^ 2"},
		{"bitwise_and", "3 & 1"},
		{"eq", "1 == 1"},
		{"neq", "1 != 2"},
		{"lt", "1 < 2"},
		{"gt", "2 > 1"},
		{"lte", "1 <= 2"},
		{"gte", "2 >= 1"},
		{"lshift", "1 << 2"},
		{"rshift", "4 >> 1"},
		{"add", "1 + 2"},
		{"sub", "3 - 1"},
		{"mul", "2 * 3"},
		{"div", "6 / 2"},
		{"mod", "7 % 3"},
		{"unary_neg", "-1"},
		{"unary_not", "~1"},
		{"unary_bang", "!0"},
		{"unary_plus", "+1"},
		{"ternary", "1 ? 2 : 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "interface T { const int X = " + tt.expr + "; }"
			doc, err := Parse("test.aidl", []byte(src))
			require.NoError(t, err)

			iface := doc.Definitions[0].(*InterfaceDecl)
			val, err := Evaluate(iface.Constants[0].Value)
			require.NoError(t, err)
			assert.NotNil(t, val)
		})
	}
}

func TestParseErrorMissingMethodName(t *testing.T) {
	src := `interface IFoo { void ; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingParenInMethod(t *testing.T) {
	src := `interface IFoo { void bar; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingTypeInField(t *testing.T) {
	src := `parcelable Foo { = 42; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingEnumName(t *testing.T) {
	src := `enum { A }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingUnionName(t *testing.T) {
	src := `union { int x; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingInterfaceName(t *testing.T) {
	src := `interface { void bar(); }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadConstValue(t *testing.T) {
	src := `interface T { const int X = ; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingConstAssign(t *testing.T) {
	src := `interface T { const int X 42; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingConstSemicolon(t *testing.T) {
	src := `interface T { const int X = 42 }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadFieldSemicolon(t *testing.T) {
	src := `parcelable Foo { int x }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadAnnotation(t *testing.T) {
	src := `@Backing(= "int") enum Foo { A }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadAnnotationValue(t *testing.T) {
	src := `@Backing(type = ) enum Foo { A }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadAnnotationClose(t *testing.T) {
	src := `@Backing(type = "int" enum Foo { A }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingGenericClose(t *testing.T) {
	src := `parcelable Foo { List<String x; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingArrayClose(t *testing.T) {
	src := `parcelable Foo { int[ x; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingMethodCloseParen(t *testing.T) {
	src := `interface IFoo { void bar(int x; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingMethodSemicolon(t *testing.T) {
	src := `interface IFoo { void bar() }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadTransactionID(t *testing.T) {
	src := `interface IFoo { void bar() = abc; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorMissingParcelableBrace(t *testing.T) {
	src := `parcelable Foo int x; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorTernaryMissingColon(t *testing.T) {
	src := `interface T { const int X = 1 ? 2; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorBadParenExpr(t *testing.T) {
	src := `interface T { const int X = (42; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInParcelableField(t *testing.T) {
	src := `parcelable Foo { int x = ; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseErrorInEnumeratorValue(t *testing.T) {
	src := `enum Foo { A = , B }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseUnterminatedString(t *testing.T) {
	src := `interface T { const String X = "unterminated; }`
	_, err := Parse("test.aidl", []byte(src))
	assert.Error(t, err)
}

func TestParseMethodWithAnnotationsOnParams(t *testing.T) {
	src := `interface IFoo {
    void bar(@nullable in String s);
}`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	require.Len(t, iface.Methods[0].Params, 1)
	param := iface.Methods[0].Params[0]
	require.Len(t, param.Annots, 1)
	assert.Equal(t, "nullable", param.Annots[0].Name)
	assert.Equal(t, DirectionIn, param.Direction)
}
