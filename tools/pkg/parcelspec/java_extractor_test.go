package parcelspec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSpecs_SimpleWriteToParcel(t *testing.T) {
	src := `
package com.example;

public class Simple implements Parcelable {
    private int mValue;
    private String mName;

    @Override
    public void writeToParcel(Parcel parcel, int flags) {
        parcel.writeInt(mValue);
        parcel.writeString8(mName);
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Len(t, specs, 1)

	spec := specs[0]
	require.Equal(t, "com.example", spec.Package)
	require.Equal(t, "Simple", spec.Type)
	require.Len(t, spec.Fields, 2)

	require.Equal(t, "Value", spec.Fields[0].Name)
	require.Equal(t, "int32", spec.Fields[0].Type)
	require.Empty(t, spec.Fields[0].Condition)

	require.Equal(t, "Name", spec.Fields[1].Name)
	require.Equal(t, "string8", spec.Fields[1].Type)
	require.Empty(t, spec.Fields[1].Condition)
}

func TestExtractSpecs_AllWriteMethods(t *testing.T) {
	src := `
package com.example;

public class AllTypes implements Parcelable {
    @Override
    public void writeToParcel(Parcel parcel, int flags) {
        parcel.writeBoolean(mActive);
        parcel.writeInt(mCount);
        parcel.writeLong(mTimestamp);
        parcel.writeFloat(mSpeed);
        parcel.writeDouble(mLatitude);
        parcel.writeString8(mLabel);
        parcel.writeString(mDescription);
        parcel.writeBundle(mExtras);
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Len(t, specs, 1)

	fields := specs[0].Fields
	require.Len(t, fields, 8)

	expected := []struct {
		name     string
		specType string
	}{
		{"Active", "bool"},
		{"Count", "int32"},
		{"Timestamp", "int64"},
		{"Speed", "float32"},
		{"Latitude", "float64"},
		{"Label", "string8"},
		{"Description", "string16"},
		{"Extras", "bundle"},
	}

	for i, exp := range expected {
		require.Equal(t, exp.name, fields[i].Name, "field %d name", i)
		require.Equal(t, exp.specType, fields[i].Type, "field %d type", i)
	}
}

func TestExtractSpecs_ConditionalWrites(t *testing.T) {
	src := `
package com.example;

public class Conditional implements Parcelable {
    private static final int HAS_SPEED_MASK = 1 << 0;
    private static final int HAS_ALT_MASK = 1 << 1;

    private int mFieldsMask;
    private float mSpeed;
    private double mAltitude;

    public boolean hasSpeed() {
        return (mFieldsMask & HAS_SPEED_MASK) != 0;
    }

    public boolean hasAltitude() {
        return (mFieldsMask & HAS_ALT_MASK) != 0;
    }

    @Override
    public void writeToParcel(Parcel parcel, int flags) {
        parcel.writeInt(mFieldsMask);
        if (hasSpeed()) {
            parcel.writeFloat(mSpeed);
        }
        if (hasAltitude()) {
            parcel.writeDouble(mAltitude);
        }
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Len(t, specs, 1)

	fields := specs[0].Fields
	require.Len(t, fields, 3)

	require.Equal(t, "FieldsMask", fields[0].Name)
	require.Equal(t, "int32", fields[0].Type)
	require.Empty(t, fields[0].Condition)

	require.Equal(t, "Speed", fields[1].Name)
	require.Equal(t, "float32", fields[1].Type)
	require.Equal(t, "FieldsMask & 1", fields[1].Condition)

	require.Equal(t, "Altitude", fields[2].Name)
	require.Equal(t, "float64", fields[2].Type)
	require.Equal(t, "FieldsMask & 2", fields[2].Condition)
}

func TestExtractSpecs_NoWriteToParcel(t *testing.T) {
	src := `
package com.example;

public class NoParcel {
    private int mValue;

    public void someMethod() {
        doStuff();
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Empty(t, specs)
}

func TestExtractSpec_LastLocationRequest(t *testing.T) {
	src, err := os.ReadFile(
		"../3rdparty/frameworks-base/location/java/android/location/LastLocationRequest.java",
	)
	if err != nil {
		t.Skip("3rdparty submodules not available")
	}

	specs := ExtractSpecs(string(src), "android.location")
	require.Len(t, specs, 1)

	spec := specs[0]
	require.Equal(t, "android.location", spec.Package)
	require.Equal(t, "LastLocationRequest", spec.Type)
	require.Len(t, spec.Fields, 3)

	require.Equal(t, "HiddenFromAppOps", spec.Fields[0].Name)
	require.Equal(t, "bool", spec.Fields[0].Type)
	require.Empty(t, spec.Fields[0].Condition)

	require.Equal(t, "AdasGnssBypass", spec.Fields[1].Name)
	require.Equal(t, "bool", spec.Fields[1].Type)
	require.Empty(t, spec.Fields[1].Condition)

	require.Equal(t, "LocationSettingsIgnored", spec.Fields[2].Name)
	require.Equal(t, "bool", spec.Fields[2].Type)
	require.Empty(t, spec.Fields[2].Condition)
}

func TestExtractSpec_Location(t *testing.T) {
	src, err := os.ReadFile(
		"../3rdparty/frameworks-base/core/java/android/location/Location.java",
	)
	if err != nil {
		t.Skip("3rdparty submodules not available")
	}

	specs := ExtractSpecs(string(src), "android.location")
	require.Len(t, specs, 1)

	spec := specs[0]
	require.Equal(t, "android.location", spec.Package)
	require.Equal(t, "Location", spec.Type)

	// Location.writeToParcel has 17 fields.
	require.GreaterOrEqual(t, len(spec.Fields), 17)

	// Verify the first few unconditional fields.
	require.Equal(t, "Provider", spec.Fields[0].Name)
	require.Equal(t, "string8", spec.Fields[0].Type)
	require.Empty(t, spec.Fields[0].Condition)

	require.Equal(t, "FieldsMask", spec.Fields[1].Name)
	require.Equal(t, "int32", spec.Fields[1].Type)
	require.Empty(t, spec.Fields[1].Condition)

	require.Equal(t, "TimeMs", spec.Fields[2].Name)
	require.Equal(t, "int64", spec.Fields[2].Type)
	require.Empty(t, spec.Fields[2].Condition)

	require.Equal(t, "ElapsedRealtimeNs", spec.Fields[3].Name)
	require.Equal(t, "int64", spec.Fields[3].Type)
	require.Empty(t, spec.Fields[3].Condition)

	// Verify conditional fields have conditions.
	// ElapsedRealtimeUncertaintyNs is conditional on HAS_ELAPSED_REALTIME_UNCERTAINTY_MASK (1 << 8 = 256).
	require.Equal(t, "ElapsedRealtimeUncertaintyNs", spec.Fields[4].Name)
	require.Equal(t, "float64", spec.Fields[4].Type)
	require.Equal(t, "FieldsMask & 256", spec.Fields[4].Condition)

	// LatitudeDegrees and LongitudeDegrees are unconditional.
	require.Equal(t, "LatitudeDegrees", spec.Fields[5].Name)
	require.Equal(t, "float64", spec.Fields[5].Type)
	require.Empty(t, spec.Fields[5].Condition)

	require.Equal(t, "LongitudeDegrees", spec.Fields[6].Name)
	require.Equal(t, "float64", spec.Fields[6].Type)
	require.Empty(t, spec.Fields[6].Condition)

	// AltitudeMeters is conditional on HAS_ALTITUDE_MASK (1 << 0 = 1).
	require.Equal(t, "AltitudeMeters", spec.Fields[7].Name)
	require.Equal(t, "float64", spec.Fields[7].Type)
	require.Equal(t, "FieldsMask & 1", spec.Fields[7].Condition)

	// SpeedMetersPerSecond is conditional on HAS_SPEED_MASK (1 << 1 = 2).
	require.Equal(t, "SpeedMetersPerSecond", spec.Fields[8].Name)
	require.Equal(t, "float32", spec.Fields[8].Type)
	require.Equal(t, "FieldsMask & 2", spec.Fields[8].Condition)

	// The last field should be Extras (opaque, unconditional).
	lastField := spec.Fields[len(spec.Fields)-1]
	require.Equal(t, "Extras", lastField.Name)
	require.Equal(t, "bundle", lastField.Type)
	require.Empty(t, lastField.Condition)
}

func TestExtractSpecs_WriteStringList(t *testing.T) {
	src := `
package com.example;

public class Desc implements Parcelable {
    @Override
    public void writeToParcel(Parcel dest, int flags) {
        dest.writeStringList(mMimeTypes);
        dest.writeLong(mTimeStamp);
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Len(t, specs, 1)

	fields := specs[0].Fields
	require.Len(t, fields, 2)

	require.Equal(t, "MimeTypes", fields[0].Name)
	require.Equal(t, "string_list", fields[0].Type)

	require.Equal(t, "TimeStamp", fields[1].Name)
	require.Equal(t, "int64", fields[1].Type)
}

func TestExtractSpecs_ForLoop(t *testing.T) {
	src := `
package com.example;

public class Container implements Parcelable {
    private int mFlags;
    private ArrayList<Item> mItems;

    @Override
    public void writeToParcel(Parcel dest, int flags) {
        dest.writeInt(mFlags);
        final int N = mItems.size();
        dest.writeInt(N);
        for (int i=0; i<N; i++) {
            Item item = mItems.get(i);
            dest.writeString8(item.mName);
            dest.writeInt(item.mValue);
        }
    }
}
`

	specs := ExtractSpecs(src, "com.example")
	require.Len(t, specs, 1)

	fields := specs[0].Fields
	require.Len(t, fields, 2, "expected mFlags + repeated Items")

	require.Equal(t, "Flags", fields[0].Name)
	require.Equal(t, "int32", fields[0].Type)

	require.Equal(t, "Items", fields[1].Name)
	require.Equal(t, "repeated", fields[1].Type)
	require.Len(t, fields[1].Elements, 2)
	require.Equal(t, "Name", fields[1].Elements[0].Name)
	require.Equal(t, "string8", fields[1].Elements[0].Type)
	require.Equal(t, "Value", fields[1].Elements[1].Name)
	require.Equal(t, "int32", fields[1].Elements[1].Type)
}

func TestExtractSpec_ClipDescription(t *testing.T) {
	src, err := os.ReadFile(
		"../3rdparty/frameworks-base/core/java/android/content/ClipDescription.java",
	)
	if err != nil {
		t.Skip("3rdparty submodules not available")
	}

	specs := ExtractSpecs(string(src), "android.content")
	require.Len(t, specs, 1)

	spec := specs[0]
	require.Equal(t, "ClipDescription", spec.Type)

	// Find MimeTypes field -- should be string_list, not opaque.
	var mimeField *FieldSpec
	for i := range spec.Fields {
		if spec.Fields[i].Name == "MimeTypes" {
			mimeField = &spec.Fields[i]
			break
		}
	}
	require.NotNil(t, mimeField, "MimeTypes field must exist")
	require.Equal(t, "string_list", mimeField.Type)
}

func TestExtractSpec_ClipData(t *testing.T) {
	src, err := os.ReadFile(
		"../3rdparty/frameworks-base/core/java/android/content/ClipData.java",
	)
	if err != nil {
		t.Skip("3rdparty submodules not available")
	}

	specs := ExtractSpecs(string(src), "android.content")
	require.NotEmpty(t, specs)

	// Find the ClipData spec (might have inner classes too).
	var clipSpec *ParcelableSpec
	for i := range specs {
		if specs[i].Type == "ClipData" {
			clipSpec = &specs[i]
			break
		}
	}
	require.NotNil(t, clipSpec, "ClipData spec must exist")

	// Find the repeated Items field.
	var itemsField *FieldSpec
	for i := range clipSpec.Fields {
		if clipSpec.Fields[i].Name == "Items" {
			itemsField = &clipSpec.Fields[i]
			break
		}
	}
	require.NotNil(t, itemsField, "Items repeated field must exist")
	require.Equal(t, "repeated", itemsField.Type)
	require.NotEmpty(t, itemsField.Elements, "Items must have element fields")

	// The first element should be the Text char_sequence.
	require.Equal(t, "Text", itemsField.Elements[0].Name)
	require.Equal(t, "char_sequence", itemsField.Elements[0].Type)

	// HtmlText is the second element.
	require.Equal(t, "HtmlText", itemsField.Elements[1].Name)
	require.Equal(t, "string8", itemsField.Elements[1].Type)

	// There should be no standalone "N" int32 field (it was subsumed by repeated).
	for _, f := range clipSpec.Fields {
		require.NotEqual(t, "N", f.Name, "count field N should be subsumed by repeated")
	}
}

func TestDeriveFieldName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"mProvider", "Provider"},
		{"mFieldsMask", "FieldsMask"},
		{"mTimeMs", "TimeMs"},
		{"provider", "Provider"},
		{"x", "x"},
		{"mX", "X"},
		{"item.mText", "Text"},
		{"item.mHtmlText", "HtmlText"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.expected, deriveFieldName(tc.input))
		})
	}
}
