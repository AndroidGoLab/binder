package codegen

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aidlparser "github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// parseAIDL is a test helper that parses an AIDL source string.
func parseAIDL(
	t *testing.T,
	src string,
) *aidlparser.Document {
	t.Helper()
	doc, err := aidlparser.Parse("test.aidl", []byte(src))
	require.NoError(t, err)
	return doc
}

// assertValidGo checks that the generated source is valid Go.
func assertValidGo(
	t *testing.T,
	src []byte,
) {
	t.Helper()
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	if err != nil {
		t.Logf("Generated source:\n%s", string(src))
	}
	assert.NoError(t, err, "generated Go source should be valid")
}

// assertFormattedGo checks that the source is already gofmt'd.
func assertFormattedGo(
	t *testing.T,
	src []byte,
) {
	t.Helper()
	formatted, err := format.Source(src)
	if assert.NoError(t, err, "source should be formattable") {
		assert.Equal(t, string(formatted), string(src), "source should be gofmt'd")
	}
}

func TestGenerateInterface_Simple(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		interface IServiceManager {
			IBinder getService(String name);
			IBinder checkService(String name);
			void addService(String name, IBinder service, boolean allowIsolated, int dumpPriority);
			String[] listServices(int dumpPriority);
			boolean isDeclared(String name);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "os", "android.os.IServiceManager")
	require.NoError(t, err)
	require.NotNil(t, src)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Check descriptor constant.
	assert.Contains(t, srcStr, `DescriptorIServiceManager = "android.os.IServiceManager"`)

	// Check transaction codes.
	assert.Contains(t, srcStr, "TransactionIServiceManagerGetService")
	assert.Contains(t, srcStr, "TransactionIServiceManagerCheckService")
	assert.Contains(t, srcStr, "TransactionIServiceManagerAddService")
	assert.Contains(t, srcStr, "TransactionIServiceManagerListServices")
	assert.Contains(t, srcStr, "TransactionIServiceManagerIsDeclared")

	// Check interface type.
	assert.Contains(t, srcStr, "type IServiceManager interface")
	assert.Contains(t, srcStr, "GetService(ctx context.Context, name string) (binder.IBinder, error)")

	// Check proxy struct.
	assert.Contains(t, srcStr, "type ServiceManagerProxy struct")
	assert.Contains(t, srcStr, "func NewServiceManagerProxy(")
	assert.Contains(t, srcStr, "var _ IServiceManager = (*ServiceManagerProxy)(nil)")

	// Check proxy methods use parcel operations.
	assert.Contains(t, srcStr, "WriteInterfaceToken(DescriptorIServiceManager)")
	assert.Contains(t, srcStr, "binder.ReadStatus(_reply)")

	// Check stub struct.
	assert.Contains(t, srcStr, "type ServiceManagerStub struct")
	assert.Contains(t, srcStr, "Impl      IServiceManager")
	assert.Contains(t, srcStr, "var _ binder.TransactionReceiver = (*ServiceManagerStub)(nil)")

	// Check stub OnTransaction method.
	assert.Contains(t, srcStr, "func (s *ServiceManagerStub) OnTransaction(")
	assert.Contains(t, srcStr, "case TransactionIServiceManagerGetService:")
	assert.Contains(t, srcStr, "case TransactionIServiceManagerIsDeclared:")
	assert.Contains(t, srcStr, "s.Impl.IsDeclared(ctx, _arg_name)")
	assert.Contains(t, srcStr, "binder.WriteStatus(_reply, _err)")
	assert.Contains(t, srcStr, "binder.WriteStatus(_reply, nil)")
}

func TestGenerateInterface_Oneway(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		oneway interface ICallback {
			void onResult(int code, String message);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "os", "android.os.ICallback")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)

	// Oneway methods should use FlagOneway.
	assert.Contains(t, srcStr, "binder.FlagOneway")
}

func TestGenerateInterface_WithConstants(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		interface IExample {
			const int VERSION = 1;
			const String DESCRIPTOR = "android.os.IExample";
			void doSomething();
			int getVersion();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "os", "android.os.IExample")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)

	// Check constants are generated with interface name prefix.
	assert.Contains(t, srcStr, "IExampleVERSION")
	assert.Contains(t, srcStr, "IExampleDESCRIPTOR")
}

func TestGenerateInterface_VoidReturn(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface ISimple {
			void doNothing();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.ISimple")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)

	// Void methods return only error.
	assert.Contains(t, srcStr, "DoNothing(ctx context.Context) error")
}

func TestGenerateParcelable_Simple(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		parcelable ServiceInfo {
			String name;
			int pid;
			boolean isRunning;
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "os", "android.os.ServiceInfo")
	require.NoError(t, err)
	require.NotNil(t, src)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Check struct fields (gofmt aligns fields, so check for field names and types).
	assert.Contains(t, srcStr, "type ServiceInfo struct")
	assert.Contains(t, srcStr, "Name")
	assert.Contains(t, srcStr, "string")
	assert.Contains(t, srcStr, "Pid")
	assert.Contains(t, srcStr, "int32")
	assert.Contains(t, srcStr, "IsRunning")
	assert.Contains(t, srcStr, "bool")

	// Check Parcelable interface compliance.
	assert.Contains(t, srcStr, "var _ parcel.Parcelable = (*ServiceInfo)(nil)")

	// Check MarshalParcel.
	assert.Contains(t, srcStr, "func (s *ServiceInfo) MarshalParcel(")
	assert.Contains(t, srcStr, "WriteParcelableHeader")
	assert.Contains(t, srcStr, "WriteParcelableFooter")

	// Check UnmarshalParcel.
	assert.Contains(t, srcStr, "func (s *ServiceInfo) UnmarshalParcel(")
	assert.Contains(t, srcStr, "ReadParcelableHeader")
	assert.Contains(t, srcStr, "SkipToParcelableEnd")

	// Check that position guards are emitted before each field read,
	// so parcels from older API levels with fewer fields don't cause
	// "read beyond end" errors.
	posGuard := "if p.Position() >= _endPos {"
	assert.Contains(t, srcStr, posGuard)
	// One guard per field (3 fields: name, pid, isRunning).
	assert.Equal(t, 3, strings.Count(srcStr, posGuard))
}

func TestGenerateParcelable_ForwardDeclared(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		parcelable NativeHandle cpp_header "android/native_handle.h";
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "os", "android.os.NativeHandle")
	require.NoError(t, err)

	// Forward-declared parcelables with cpp_header produce no output.
	assert.Nil(t, src)
}

func TestGenerateEnum_Simple(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		@Backing(type="int")
		enum Status {
			OK = 0,
			ERROR = 1,
			UNKNOWN = 2,
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.EnumDecl)

	src, err := GenerateEnum(decl, "os")
	require.NoError(t, err)
	require.NotNil(t, src)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Check type declaration.
	assert.Contains(t, srcStr, "type Status int32")

	// Check constants. Single-word all-caps names (OK, ERROR, UNKNOWN)
	// don't get snake-case conversion since they lack underscores.
	assert.Contains(t, srcStr, "StatusOK")
	assert.Contains(t, srcStr, "StatusERROR")
	assert.Contains(t, srcStr, "StatusUNKNOWN")
}

func TestGenerateEnum_LongBacking(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		@Backing(type="long")
		enum BigEnum {
			VALUE_A = 0,
			VALUE_B = 100,
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.EnumDecl)

	src, err := GenerateEnum(decl, "test")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	assert.Contains(t, srcStr, "type BigEnum int64")
}

func TestGenerateEnum_DefaultBacking(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		enum SimpleEnum {
			A = 0,
			B = 1,
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.EnumDecl)

	src, err := GenerateEnum(decl, "test")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// Default backing type is int32.
	assert.Contains(t, srcStr, "type SimpleEnum int32")
}

func TestGenerateUnion_Simple(t *testing.T) {
	doc := parseAIDL(t, `
		package android.os;
		union Result {
			int intValue;
			String stringValue;
			boolean boolValue;
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.UnionDecl)

	src, err := GenerateUnion(decl, "os", "android.os.Result")
	require.NoError(t, err)
	require.NotNil(t, src)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Check tag constants (gofmt may align them).
	assert.Contains(t, srcStr, "ResultTagIntValue")
	assert.Contains(t, srcStr, "ResultTagStringValue")
	assert.Contains(t, srcStr, "ResultTagBoolValue")

	// Check struct (gofmt aligns fields).
	assert.Contains(t, srcStr, "type Result struct")
	assert.Contains(t, srcStr, "Tag")
	assert.Contains(t, srcStr, "IntValue")
	assert.Contains(t, srcStr, "StringValue")
	assert.Contains(t, srcStr, "BoolValue")

	// Check accessors.
	assert.Contains(t, srcStr, "func (u *Result) GetIntValue()")
	assert.Contains(t, srcStr, "func (u *Result) SetIntValue(")

	// Check Parcelable interface compliance.
	assert.Contains(t, srcStr, "var _ parcel.Parcelable = (*Result)(nil)")

	// Check MarshalParcel/UnmarshalParcel.
	assert.Contains(t, srcStr, "func (u *Result) MarshalParcel(")
	assert.Contains(t, srcStr, "func (u *Result) UnmarshalParcel(")
}

func TestGenerateConstants(t *testing.T) {
	constants := []*aidlparser.ConstantDecl{
		{
			Type:      &aidlparser.TypeSpecifier{Name: "int"},
			ConstName: "MAX_COUNT",
			Value:     &aidlparser.IntegerLiteral{Value: "100"},
		},
		{
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
			ConstName: "DEFAULT_NAME",
			Value:     &aidlparser.StringLiteralExpr{Value: "default"},
		},
	}

	f := NewGoFile("test")
	err := GenerateConstants(constants, f, "MyType")
	require.NoError(t, err)

	src, err := f.Bytes()
	require.NoError(t, err)

	srcStr := string(src)
	assert.Contains(t, srcStr, "MyTypeMaxCount")
	assert.Contains(t, srcStr, "MyTypeDefaultName")
}

func TestDeriveProxyName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"IServiceManager", "ServiceManagerProxy"},
		{"IFoo", "FooProxy"},
		{"Foo", "FooProxy"},
		{"I", "IProxy"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, deriveProxyName(tt.input))
		})
	}
}

func TestSanitizeGoIdent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"name", "name"},
		{"type", "type_"},
		{"map", "map_"},
		{"interface", "interface_"},
		{"normal", "normal"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeGoIdent(tt.input))
		})
	}
}

func TestMethodSignature(t *testing.T) {
	t.Run("void_no_params", func(t *testing.T) {
		m := &aidlparser.MethodDecl{
			MethodName: "doNothing",
			ReturnType: &aidlparser.TypeSpecifier{Name: "void"},
		}
		sig := methodSignature(m, nil)
		assert.Equal(t, "DoNothing(ctx context.Context) error", sig)
	})

	t.Run("int_return_with_param", func(t *testing.T) {
		m := &aidlparser.MethodDecl{
			MethodName: "getCount",
			ReturnType: &aidlparser.TypeSpecifier{Name: "int"},
			Params: []*aidlparser.ParamDecl{
				{
					ParamName: "name",
					Type:      &aidlparser.TypeSpecifier{Name: "String"},
				},
			},
		}
		sig := methodSignature(m, nil)
		assert.Equal(t, "GetCount(ctx context.Context, name string) (int32, error)", sig)
	})
}

func TestLastPackageSegment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"android.os", "os"},
		{"com.example.service", "service"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, lastPackageSegment(tt.input))
		})
	}
}

func TestPackageFromQualified(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"android.os.IServiceManager", "android.os"},
		{"com.example.Foo", "com.example"},
		{"Foo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, packageFromQualified(tt.input))
		})
	}
}

func TestGenerateInterface_FromFile(t *testing.T) {
	doc, err := aidlparser.ParseFile("testdata/simple_interface.aidl")
	require.NoError(t, err)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "os", "android.os.IServiceManager")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)
}

func TestGenerateParcelable_FromFile(t *testing.T) {
	doc, err := aidlparser.ParseFile("testdata/simple_parcelable.aidl")
	require.NoError(t, err)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "os", "android.os.ServiceInfo")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)
}

func TestGenerateEnum_FromFile(t *testing.T) {
	doc, err := aidlparser.ParseFile("testdata/simple_enum.aidl")
	require.NoError(t, err)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.EnumDecl)

	src, err := GenerateEnum(decl, "os")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)
}

func TestGenerateUnion_FromFile(t *testing.T) {
	doc, err := aidlparser.ParseFile("testdata/simple_union.aidl")
	require.NoError(t, err)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.UnionDecl)

	src, err := GenerateUnion(decl, "os", "android.os.Result")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)
}

func TestConstExprToGo(t *testing.T) {
	tests := []struct {
		name     string
		expr     aidlparser.ConstExpr
		expected string
	}{
		{
			name:     "integer",
			expr:     &aidlparser.IntegerLiteral{Value: "42"},
			expected: "42",
		},
		{
			name:     "hex",
			expr:     &aidlparser.IntegerLiteral{Value: "0xFF"},
			expected: "0xFF",
		},
		{
			name:     "float",
			expr:     &aidlparser.FloatLiteral{Value: "3.14"},
			expected: "3.14",
		},
		{
			name:     "string",
			expr:     &aidlparser.StringLiteralExpr{Value: "hello"},
			expected: `"hello"`,
		},
		{
			name:     "bool_true",
			expr:     &aidlparser.BoolLiteral{Value: true},
			expected: "true",
		},
		{
			name:     "bool_false",
			expr:     &aidlparser.BoolLiteral{Value: false},
			expected: "false",
		},
		{
			name:     "null",
			expr:     &aidlparser.NullLiteral{},
			expected: "nil",
		},
		{
			name:     "ident",
			expr:     &aidlparser.IdentExpr{Name: "MY_CONST"},
			expected: "MyConst",
		},
		{
			name: "unary_minus",
			expr: &aidlparser.UnaryExpr{
				Op:      aidlparser.TokenMinus,
				Operand: &aidlparser.IntegerLiteral{Value: "1"},
			},
			expected: "-1",
		},
		{
			name: "binary_or",
			expr: &aidlparser.BinaryExpr{
				Op:    aidlparser.TokenPipe,
				Left:  &aidlparser.IntegerLiteral{Value: "1"},
				Right: &aidlparser.IntegerLiteral{Value: "2"},
			},
			expected: "(1 | 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := constExprToGo(tt.expr)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTokenToGoOp(t *testing.T) {
	tests := []struct {
		token    aidlparser.TokenKind
		expected string
	}{
		{aidlparser.TokenPlus, "+"},
		{aidlparser.TokenMinus, "-"},
		{aidlparser.TokenStar, "*"},
		{aidlparser.TokenSlash, "/"},
		{aidlparser.TokenPercent, "%"},
		{aidlparser.TokenAmp, "&"},
		{aidlparser.TokenPipe, "|"},
		{aidlparser.TokenCaret, "^"},
		{aidlparser.TokenTilde, "^"},
		{aidlparser.TokenBang, "!"},
		{aidlparser.TokenLShift, "<<"},
		{aidlparser.TokenRShift, ">>"},
		{aidlparser.TokenAmpAmp, "&&"},
		{aidlparser.TokenPipePipe, "||"},
		{aidlparser.TokenEqEq, "=="},
		{aidlparser.TokenBangEq, "!="},
		{aidlparser.TokenLAngle, "<"},
		{aidlparser.TokenRAngle, ">"},
		{aidlparser.TokenLessEq, "<="},
		{aidlparser.TokenGreaterEq, ">="},
	}

	for _, tt := range tests {
		t.Run(tt.token.String(), func(t *testing.T) {
			assert.Equal(t, tt.expected, tokenToGoOp(tt.token))
		})
	}
}

func TestGenerateInterface_NoMethods(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IEmpty {
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IEmpty")
	require.NoError(t, err)

	assertValidGo(t, src)
}

func TestGenerateInterface_MultipleParams(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IMulti {
			void send(int a, long b, float c, double d, boolean e, byte f, String g);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IMulti")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// Verify all parameter types are correct.
	assert.Contains(t, srcStr, "a int32")
	assert.Contains(t, srcStr, "b int64")
	assert.Contains(t, srcStr, "c float32")
	assert.Contains(t, srcStr, "d float64")
	assert.Contains(t, srcStr, "e bool")
	assert.Contains(t, srcStr, "f byte")
	assert.Contains(t, srcStr, "g string")
}

func TestGenerateParcelable_WithConstants(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		parcelable Config {
			const int DEFAULT_PORT = 8080;
			String host;
			int port;
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "test", "test.Config")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	assert.Contains(t, srcStr, "DefaultPort")
	assert.Contains(t, srcStr, "8080")
}

func TestGenerateInterface_ReservedKeywordParam(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IKeyword {
			void setType(String type);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IKeyword")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// "type" should be sanitized to "type_".
	assert.Contains(t, srcStr, "type_")
	assert.True(t, !strings.Contains(srcStr, " type string") || strings.Contains(srcStr, "type_ string"))
}

func TestGenerateInterface_ArrayReturn(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IArrays {
			String[] listNames();
			int[] getIds();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IArrays")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// Array returns should use ReadInt32 for count then loop.
	assert.Contains(t, srcStr, "ReadInt32()")
	assert.Contains(t, srcStr, "make([]string")
	assert.Contains(t, srcStr, "make([]int32")
}

func TestGenerateInterface_ArrayParam(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IArrayParam {
			void processItems(int[] ids, String[] names);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IArrayParam")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// Array params should be written with count + loop.
	assert.Contains(t, srcStr, "WriteInt32(int32(len(ids)))")
	assert.Contains(t, srcStr, "WriteInt32(int32(len(names)))")
}

func TestGenerateParcelable_WithArrayField(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		parcelable ArrayStruct {
			int[] ids;
			String[] names;
			int singleValue;
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "test", "test.ArrayStruct")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)
	// gofmt aligns struct fields, so check separately.
	assert.Contains(t, srcStr, "Ids")
	assert.Contains(t, srcStr, "[]int32")
	assert.Contains(t, srcStr, "Names")
	assert.Contains(t, srcStr, "[]string")
	// Marshal should write array count.
	assert.Contains(t, srcStr, "WriteInt32(int32(len(s.Ids)))")
}

func TestGenerateParcelable_WithParcelableArrayField(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		parcelable Inner {
			int value;
		}
		parcelable Outer {
			Inner[] items;
			int count;
		}
	`)

	require.Len(t, doc.Definitions, 2)
	decl := doc.Definitions[1].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "test", "test.Outer")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// NDK wire format: each parcelable element in an array must be
	// preceded by an int32(1) non-null indicator.
	assert.Contains(t, srcStr, "p.WriteInt32(1)")
	assert.Contains(t, srcStr, "_item.MarshalParcel(p)")

	// Unmarshal must read the non-null indicator before each element.
	assert.Contains(t, srcStr, "if _, _err = p.ReadInt32(); _err != nil")
	assert.Contains(t, srcStr, "s.Items[_i].UnmarshalParcel(p)")

	// Primitive arrays must NOT have non-null indicators.
	// (Covered by TestGenerateParcelable_WithArrayField, but verify
	// that the presence of a parcelable array didn't break primitives.)
	assert.Contains(t, srcStr, "WriteInt32(int32(len(s.Items)))")
}

func TestGenerateInterface_ParcelableArrayParam(t *testing.T) {
	// Without a registry, unknown types default to parcelable
	// (MarshalParcel/UnmarshalParcel), which is what we want to test.
	doc := parseAIDL(t, `
		package test;
		interface IBatch {
			void processItems(Item[] items);
			Item[] getItems();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IBatch")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Proxy marshal: non-null indicator before each parcelable array element.
	assert.Contains(t, srcStr, "_data.WriteInt32(1)")
	assert.Contains(t, srcStr, "_item.MarshalParcel(_data)")

	// Proxy unmarshal (return value): read non-null indicator before each element.
	assert.Contains(t, srcStr, "if _, _err = _reply.ReadInt32(); _err != nil")
	assert.Contains(t, srcStr, "_result[_i].UnmarshalParcel(_reply)")
}

func TestGenerateParcelable_WithFixedSizeByteArray(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		parcelable DescriptorInfo {
			byte[128] name;
			int width;
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.ParcelableDecl)

	src, err := GenerateParcelable(decl, "test", "test.DescriptorInfo")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)
	// Fixed-size byte arrays use WriteFixedByteArray/ReadFixedByteArray,
	// not WriteByteArray/ReadByteArray.
	assert.Contains(t, srcStr, "WriteFixedByteArray(s.Name, 128)")
	assert.Contains(t, srcStr, "ReadFixedByteArray(128)")
	assert.NotContains(t, srcStr, "WriteByteArray")
	assert.NotContains(t, srcStr, "ReadByteArray()")
}

func TestGenerateInterface_ListGenericReturn(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IGenericList {
			List<String> getItems();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IGenericList")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)
	// List<String> maps to []string and uses array read logic.
	assert.Contains(t, srcStr, "[]string")
	assert.Contains(t, srcStr, "make([]string")
}

func TestGenerateGenerator_EndToEnd(t *testing.T) {
	r := resolver.New([]string{"testdata"})
	outDir := t.TempDir()
	gen := NewGenerator(r, outDir)

	doc, err := aidlparser.ParseFile("testdata/simple_interface.aidl")
	require.NoError(t, err)

	err = r.ResolveDocument(doc, "testdata/simple_interface.aidl")
	require.NoError(t, err)

	err = gen.GenerateAll()
	require.NoError(t, err)

	// Verify the output file exists.
	outFile := filepath.Join(outDir, "android", "os", "iservicemanager.go")
	_, err = os.Stat(outFile)
	require.NoError(t, err, "generated file should exist at %s", outFile)

	// Read and verify it's valid Go.
	src, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assertValidGo(t, src)
}

// TestGeneratedCode_Compiles generates Go code from all test AIDL files,
// writes it to a temporary Go module, and runs `go build` to verify
// the generated code actually type-checks (not just parses).
func TestGeneratedCode_Compiles(t *testing.T) {
	aidlFiles, err := filepath.Glob("testdata/*.aidl")
	require.NoError(t, err)
	require.NotEmpty(t, aidlFiles, "no AIDL test files found")

	r := resolver.New([]string{"testdata"})
	outDir := t.TempDir()
	gen := NewGenerator(r, outDir)

	for _, f := range aidlFiles {
		err := r.ResolveFile(f)
		require.NoError(t, err, "resolving %s", f)
	}

	err = gen.GenerateAll()
	require.NoError(t, err, "generating code")

	// Verify at least one Go file was generated.
	goFiles, err := filepath.Glob(filepath.Join(outDir, "**", "*.go"))
	if err != nil || len(goFiles) == 0 {
		goFiles, _ = filepath.Glob(filepath.Join(outDir, "*", "*", "*.go"))
	}
	require.NotEmpty(t, goFiles, "no Go files generated in %s", outDir)

	// Create a Go module that imports the generated code.
	modRoot, _ := filepath.Abs("../../..") // project root
	goMod := "module test-gen\n\ngo 1.25.0\n\n" +
		"require github.com/xaionaro-go/binder v0.0.0\n\n" +
		"replace github.com/xaionaro-go/binder => " + modRoot + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(goMod), 0o644))

	// Run `go mod tidy` then `go build ./...` in the temp module.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = outDir
	tidyOut, err := tidy.CombinedOutput()
	require.NoError(t, err, "go mod tidy failed:\n%s", string(tidyOut))

	build := exec.Command("go", "build", "./...")
	build.Dir = outDir
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed on generated code:\n%s", string(buildOut))
}

func TestElementTypeSpec(t *testing.T) {
	t.Run("array_type", func(t *testing.T) {
		ts := &aidlparser.TypeSpecifier{Name: "String", IsArray: true}
		elem := elementTypeSpec(ts)
		assert.Equal(t, "String", elem.Name)
		assert.False(t, elem.IsArray)
	})

	t.Run("list_type", func(t *testing.T) {
		ts := &aidlparser.TypeSpecifier{
			Name: "List",
			TypeArgs: []*aidlparser.TypeSpecifier{
				{Name: "int"},
			},
		}
		elem := elementTypeSpec(ts)
		assert.Equal(t, "int", elem.Name)
	})
}

func TestDeriveStubName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"IServiceManager", "ServiceManagerStub"},
		{"IFoo", "FooStub"},
		{"Foo", "FooStub"},
		{"I", "IStub"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, deriveStubName(tt.input))
		})
	}
}

func TestGenerateInterface_StubPrimitiveReturn(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface ICounter {
			int getCount();
			void setCount(int count);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.ICounter")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Stub struct and compliance.
	assert.Contains(t, srcStr, "type CounterStub struct")
	assert.Contains(t, srcStr, "Impl      ICounter")
	assert.Contains(t, srcStr, "var _ binder.TransactionReceiver = (*CounterStub)(nil)")

	// OnTransaction dispatcher.
	assert.Contains(t, srcStr, "func (s *CounterStub) OnTransaction(")
	assert.Contains(t, srcStr, "case TransactionICounterGetCount:")
	assert.Contains(t, srcStr, "case TransactionICounterSetCount:")

	// GetCount: calls impl and writes result.
	assert.Contains(t, srcStr, "s.Impl.GetCount(ctx)")
	assert.Contains(t, srcStr, "_reply.WriteInt32(_result)")

	// SetCount: reads param from data, calls impl.
	assert.Contains(t, srcStr, "s.Impl.SetCount(ctx, _arg_count)")
}

func TestGenerateInterface_StubOneway(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		oneway interface INotify {
			void onEvent(int eventId, String detail);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.INotify")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// Oneway stubs return the error (no reply parcel, but error is
	// propagated for local handling / logging).
	assert.Contains(t, srcStr, "type NotifyStub struct")
	assert.Contains(t, srcStr, "return nil, _err")
	assert.NotContains(t, srcStr, "_ = _err")
	assert.Contains(t, srcStr, "s.Impl.OnEvent(ctx, _arg_eventId, _arg_detail)")
}

func TestGenerateInterface_StubNoMethods(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IEmpty {
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IEmpty")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)

	// Stub is still generated for empty interfaces.
	assert.Contains(t, srcStr, "type EmptyStub struct")
	assert.Contains(t, srcStr, "Impl      IEmpty")

	// Even with no methods, the stub's OnTransaction must validate the
	// interface token so that malformed transactions are rejected early.
	assert.Contains(t, srcStr, "ReadInterfaceToken()")
}

func TestGenerateInterface_StubVoidNoParams(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IPing {
			void ping();
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IPing")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	assert.Contains(t, srcStr, "type PingStub struct")
	// void + no params: should use := for _err since it's the first declaration.
	assert.Contains(t, srcStr, "s.Impl.Ping(ctx)")
}
