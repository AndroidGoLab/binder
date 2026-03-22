package codegen

import (
	"github.com/AndroidGoLab/binder/tools/pkg/parser"
)

// identityParamMapping associates an AIDL parameter name with the
// CallerIdentity field it corresponds to and the AIDL type that must match.
type identityParamMapping struct {
	// IdentityField is the CallerIdentity struct field name
	// (e.g. "PackageName", "UID").
	IdentityField string

	// AIDLType is the AIDL type the parameter must have for the mapping
	// to apply (e.g. "String" for PackageName, "int" for UID).
	AIDLType string
}

// identityParamNames maps AIDL parameter names to their CallerIdentity field
// mappings. Only unambiguously caller-identity names are included;
// names like "packageName" are excluded because they may refer to a
// target package rather than the calling package.
var identityParamNames = map[string]identityParamMapping{
	// PackageName — the calling app's package name.
	"callingPackage": {IdentityField: "PackageName", AIDLType: "String"},
	"opPackageName":  {IdentityField: "PackageName", AIDLType: "String"},

	// AttributionTag — the calling feature / attribution tag.
	"attributionTag":  {IdentityField: "AttributionTag", AIDLType: "String"},
	"callingFeatureId": {IdentityField: "AttributionTag", AIDLType: "String"},

	// UserID — the Android user ID of the caller.
	"userId":        {IdentityField: "UserID", AIDLType: "int"},
	"userHandle":    {IdentityField: "UserID", AIDLType: "int"},
	"callingUserId": {IdentityField: "UserID", AIDLType: "int"},

	// PID — the process ID of the caller.
	"callingPid": {IdentityField: "PID", AIDLType: "int"},
	"appPid":     {IdentityField: "PID", AIDLType: "int"},

	// UID — the user ID (Linux UID) of the caller.
	"callingUid": {IdentityField: "UID", AIDLType: "int"},
	"appUid":     {IdentityField: "UID", AIDLType: "int"},
}

// identityFieldForParam returns the CallerIdentity field name that the given
// AIDL parameter maps to, or "" if the parameter is not an identity param.
// Matching is based on both the AIDL parameter name and its type.
func identityFieldForParam(param *parser.ParamDecl) string {
	mapping, ok := identityParamNames[param.ParamName]
	if !ok {
		return ""
	}

	if param.Type == nil {
		return ""
	}

	// Only match simple (non-array, non-generic) types.
	if param.Type.IsArray || len(param.Type.TypeArgs) > 0 {
		return ""
	}

	if param.Type.Name != mapping.AIDLType {
		return ""
	}

	return mapping.IdentityField
}

// identityWriteExpr maps AIDL types used in identity parameters to
// the parcel write expression format string. The %s placeholder is
// replaced with the value expression (e.g. "_identity.PackageName").
var identityWriteExpr = map[string]string{
	"String": "_data.WriteString16(%s)",
	"int":    "_data.WriteInt32(%s)",
}

// classifyParams separates method parameters into regular parameters and
// identity-auto-filled parameters. For each identity parameter, it records
// which CallerIdentity field to use and the original parameter for parcel
// serialization.
func classifyParams(
	params []*parser.ParamDecl,
) (regular []*parser.ParamDecl, identity map[int]string) {
	identity = make(map[int]string)

	for i, param := range params {
		field := identityFieldForParam(param)
		if field == "" {
			regular = append(regular, param)
			continue
		}
		identity[i] = field
	}

	return regular, identity
}
