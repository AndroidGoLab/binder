package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aidlparser "github.com/xaionaro-go/binder/tools/pkg/parser"
)

func TestIdentityFieldForParam(t *testing.T) {
	tests := []struct {
		name     string
		param    *aidlparser.ParamDecl
		expected string
	}{
		{
			name: "callingPackage_string",
			param: &aidlparser.ParamDecl{
				ParamName: "callingPackage",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "PackageName",
		},
		{
			name: "opPackageName_string",
			param: &aidlparser.ParamDecl{
				ParamName: "opPackageName",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "PackageName",
		},
		{
			name: "attributionTag_string",
			param: &aidlparser.ParamDecl{
				ParamName: "attributionTag",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "AttributionTag",
		},
		{
			name: "callingFeatureId_string",
			param: &aidlparser.ParamDecl{
				ParamName: "callingFeatureId",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "AttributionTag",
		},
		{
			name: "userId_int",
			param: &aidlparser.ParamDecl{
				ParamName: "userId",
				Type:      &aidlparser.TypeSpecifier{Name: "int"},
			},
			expected: "UserID",
		},
		{
			name: "callingPid_int",
			param: &aidlparser.ParamDecl{
				ParamName: "callingPid",
				Type:      &aidlparser.TypeSpecifier{Name: "int"},
			},
			expected: "PID",
		},
		{
			name: "callingUid_int",
			param: &aidlparser.ParamDecl{
				ParamName: "callingUid",
				Type:      &aidlparser.TypeSpecifier{Name: "int"},
			},
			expected: "UID",
		},
		{
			name: "appUid_int",
			param: &aidlparser.ParamDecl{
				ParamName: "appUid",
				Type:      &aidlparser.TypeSpecifier{Name: "int"},
			},
			expected: "UID",
		},
		{
			name: "packageName_not_identity",
			param: &aidlparser.ParamDecl{
				ParamName: "packageName",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "",
		},
		{
			name: "callingPackage_wrong_type",
			param: &aidlparser.ParamDecl{
				ParamName: "callingPackage",
				Type:      &aidlparser.TypeSpecifier{Name: "int"},
			},
			expected: "",
		},
		{
			name: "callingUid_wrong_type",
			param: &aidlparser.ParamDecl{
				ParamName: "callingUid",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "",
		},
		{
			name: "callingPackage_array_not_identity",
			param: &aidlparser.ParamDecl{
				ParamName: "callingPackage",
				Type:      &aidlparser.TypeSpecifier{Name: "String", IsArray: true},
			},
			expected: "",
		},
		{
			name: "regular_param",
			param: &aidlparser.ParamDecl{
				ParamName: "provider",
				Type:      &aidlparser.TypeSpecifier{Name: "String"},
			},
			expected: "",
		},
		{
			name: "nil_type",
			param: &aidlparser.ParamDecl{
				ParamName: "callingPackage",
				Type:      nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := identityFieldForParam(tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifyParams(t *testing.T) {
	params := []*aidlparser.ParamDecl{
		{
			ParamName: "provider",
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
		},
		{
			ParamName: "callingPackage",
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
		},
		{
			ParamName: "attributionTag",
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
		},
		{
			ParamName: "listenerId",
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
		},
	}

	regular, identity := classifyParams(params)

	require.Len(t, regular, 2)
	assert.Equal(t, "provider", regular[0].ParamName)
	assert.Equal(t, "listenerId", regular[1].ParamName)

	require.Len(t, identity, 2)
	assert.Equal(t, "PackageName", identity[1])
	assert.Equal(t, "AttributionTag", identity[2])
}

func TestClassifyParams_NoIdentity(t *testing.T) {
	params := []*aidlparser.ParamDecl{
		{
			ParamName: "name",
			Type:      &aidlparser.TypeSpecifier{Name: "String"},
		},
		{
			ParamName: "value",
			Type:      &aidlparser.TypeSpecifier{Name: "int"},
		},
	}

	regular, identity := classifyParams(params)

	require.Len(t, regular, 2)
	assert.Empty(t, identity)
}

func TestGenerateInterface_IdentityParamAutoFilled(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface ILocationManager {
			void registerListener(
				String provider,
				String callingPackage,
				String attributionTag,
				String listenerId
			);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.ILocationManager")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// The interface type should omit identity params, matching the proxy
	// signature so that *Proxy satisfies the interface.
	assert.Contains(t, srcStr, "RegisterListener(ctx context.Context, provider string, listenerId string) error")

	// The proxy method signature should omit identity params but keep
	// regular params. Check the proxy func declaration specifically.
	assert.Contains(t, srcStr, "func (p *LocationManagerProxy) RegisterListener(\n\tctx context.Context,\n\tprovider string,\n\tlistenerId string,\n) error {")

	// Identity should be fetched from the binder.
	assert.Contains(t, srcStr, "_identity := p.Remote.Identity()")

	// Identity fields should be used in parcel writing.
	assert.Contains(t, srcStr, "_identity.PackageName")
	assert.Contains(t, srcStr, "_identity.AttributionTag")
}

func TestGenerateInterface_IdentityParamIntType(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IService {
			void doWork(String name, int callingUid, int callingPid);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IService")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// The proxy method signature should only have the regular param.
	assert.Contains(t, srcStr, "func (p *ServiceProxy) DoWork(\n\tctx context.Context,\n\tname string,\n) error {")

	// The interface should omit identity params, matching the proxy.
	assert.Contains(t, srcStr, "DoWork(ctx context.Context, name string) error")

	// Identity fields should be written via _identity.
	assert.Contains(t, srcStr, "_identity.UID")
	assert.Contains(t, srcStr, "_identity.PID")
	assert.Contains(t, srcStr, "_data.WriteInt32(_identity.UID)")
	assert.Contains(t, srcStr, "_data.WriteInt32(_identity.PID)")
}

func TestGenerateInterface_NoIdentityParams(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface ISimple {
			void doWork(String name, int value);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.ISimple")
	require.NoError(t, err)

	assertValidGo(t, src)

	srcStr := string(src)

	// No identity usage when there are no identity params.
	assert.NotContains(t, srcStr, "_identity")
	assert.NotContains(t, srcStr, "p.Remote.Identity()")

	// All params should be in the proxy signature.
	assert.Contains(t, srcStr, "name string")
	assert.Contains(t, srcStr, "value int32")
}

func TestGenerateInterface_AmbiguousPackageNameNotAutoFilled(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IInstaller {
			void install(String packageName, String callingPackage);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IInstaller")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// The proxy signature should keep "packageName" (ambiguous) but
	// omit "callingPackage" (unambiguous identity param).
	assert.Contains(t, srcStr, "func (p *InstallerProxy) Install(\n\tctx context.Context,\n\tpackageName string,\n) error {")

	// "callingPackage" should be auto-filled from identity.
	assert.Contains(t, srcStr, "_identity.PackageName")
}

func TestGenerateInterface_IdentityParamWithReturn(t *testing.T) {
	doc := parseAIDL(t, `
		package test;
		interface IService {
			int getCount(String opPackageName, String name);
		}
	`)

	require.Len(t, doc.Definitions, 1)
	decl := doc.Definitions[0].(*aidlparser.InterfaceDecl)

	src, err := GenerateInterface(decl, "test", "test.IService")
	require.NoError(t, err)

	assertValidGo(t, src)
	assertFormattedGo(t, src)

	srcStr := string(src)

	// The proxy signature should omit opPackageName and return (int32, error).
	assert.Contains(t, srcStr, "func (p *ServiceProxy) GetCount(\n\tctx context.Context,\n\tname string,\n) (int32, error) {")

	// The interface should omit identity params, matching the proxy.
	assert.Contains(t, srcStr, "GetCount(ctx context.Context, name string) (int32, error)")

	// opPackageName should be auto-filled from identity.
	assert.Contains(t, srcStr, "_identity.PackageName")
}
