//go:build e2e

package e2e

import (
	"context"
	"math"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/parcel"
)

// ---------------------------------------------------------------------------
// 1. TestParcelEdge_EmptyString
//
// Round-trip an empty string through both UTF-16 and UTF-8 wire formats.
// Also verifies against a real Android service (checkPermission with empty
// permission string).
// ---------------------------------------------------------------------------

func TestParcelEdge_EmptyString(t *testing.T) {
	t.Run("UTF16_RoundTrip", func(t *testing.T) {
		p := parcel.New()
		p.WriteString16("")

		p.SetPosition(0)
		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "", s, "empty string should round-trip via String16")
		// 4 bytes length(0) + 2 bytes null terminator padded to 4 = 8 bytes total.
		assert.Equal(t, 8, p.Len(), "empty String16 wire size should be 8 bytes")
	})

	t.Run("UTF8_RoundTrip", func(t *testing.T) {
		p := parcel.New()
		p.WriteString("")

		p.SetPosition(0)
		s, err := p.ReadString()
		require.NoError(t, err)
		assert.Equal(t, "", s, "empty string should round-trip via UTF-8 String")
		// 4 bytes length(0) + 1 byte null terminator padded to 4 = 8 bytes total.
		assert.Equal(t, 8, p.Len(), "empty UTF-8 String wire size should be 8 bytes")
	})

	t.Run("RealService_EmptyPermission", func(t *testing.T) {
		ctx := context.Background()
		driver := openBinder(t)
		am := getActivityManager(ctx, t, driver)

		codeCheckPerm := resolveCode(ctx, t, am, activityManagerDescriptor, "checkPermission")
		data := parcel.New()
		data.WriteInterfaceToken(activityManagerDescriptor)
		data.WriteString16("") // empty permission string
		data.WriteInt32(int32(os.Getpid()))
		data.WriteInt32(0) // uid 0

		reply, err := am.Transact(ctx, codeCheckPerm, 0, data)
		requireOrSkip(t, err)
		requireOrSkip(t, binder.ReadStatus(reply))

		val, err := reply.ReadInt32()
		require.NoError(t, err)
		// Root (uid 0) typically gets GRANTED (0), but DENIED (-1) is also valid
		// for an empty permission string on some Android versions.
		// The key assertion is that we successfully serialized and deserialized
		// the empty string parameter without corruption.
		t.Logf("checkPermission(empty, pid=%d, uid=0): %d (0=GRANTED, -1=DENIED)", os.Getpid(), val)
		assert.Contains(t, []int32{0, -1}, val, "result should be GRANTED or DENIED")
	})
}

// ---------------------------------------------------------------------------
// 2. TestParcelEdge_NullString
//
// The Go API supports null strings via WriteNullString16/WriteNullString.
// Verify round-trip and that ReadString16/ReadString return "" for null
// (they cannot distinguish null from empty on the read side).
// ---------------------------------------------------------------------------

func TestParcelEdge_NullString(t *testing.T) {
	t.Run("UTF16_NullRoundTrip", func(t *testing.T) {
		p := parcel.New()
		p.WriteNullString16()

		p.SetPosition(0)
		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "", s, "null String16 should read back as empty string")
		// Only the -1 length prefix: 4 bytes.
		assert.Equal(t, 4, p.Len(), "null String16 should be 4 bytes on the wire")
	})

	t.Run("UTF8_NullRoundTrip", func(t *testing.T) {
		p := parcel.New()
		p.WriteNullString()

		p.SetPosition(0)
		s, err := p.ReadString()
		require.NoError(t, err)
		assert.Equal(t, "", s, "null UTF-8 String should read back as empty string")
		assert.Equal(t, 4, p.Len(), "null UTF-8 String should be 4 bytes on the wire")
	})

	t.Run("NullAndEmptySequence", func(t *testing.T) {
		// Write null, then empty, then non-empty. Read them all back.
		p := parcel.New()
		p.WriteNullString16()
		p.WriteString16("")
		p.WriteString16("hello")

		p.SetPosition(0)

		s1, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "", s1, "null reads as empty")

		s2, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "", s2, "empty reads as empty")

		s3, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "hello", s3)
	})
}

// ---------------------------------------------------------------------------
// 3. TestParcelEdge_VeryLongString
//
// 100KB string through both UTF-16 and UTF-8 wire formats.
// Tests that grow() handles large allocations and alignment correctly.
// ---------------------------------------------------------------------------

func TestParcelEdge_VeryLongString(t *testing.T) {
	longStr := strings.Repeat("A", 100*1024) // 100KB of ASCII 'A'

	t.Run("UTF16_100KB", func(t *testing.T) {
		p := parcel.New()
		p.WriteString16(longStr)

		p.SetPosition(0)
		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, len(longStr), len(s), "100KB string length should be preserved")
		assert.Equal(t, longStr, s, "100KB string content should be preserved")
	})

	t.Run("UTF8_100KB", func(t *testing.T) {
		p := parcel.New()
		p.WriteString(longStr)

		p.SetPosition(0)
		s, err := p.ReadString()
		require.NoError(t, err)
		assert.Equal(t, len(longStr), len(s))
		assert.Equal(t, longStr, s)
	})

	t.Run("UTF16_100KB_WithTrailingData", func(t *testing.T) {
		// Verify alignment: write a long string, then an int32 after it.
		p := parcel.New()
		p.WriteString16(longStr)
		p.WriteInt32(42)

		p.SetPosition(0)
		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, longStr, s)

		v, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(42), v, "int32 after 100KB string should be readable")
	})

	t.Run("UTF16_MultiByteUnicode_100KB", func(t *testing.T) {
		// 100KB of multi-byte UTF-16 characters (CJK ideographs).
		cjk := strings.Repeat("\u4F60\u597D", 50*1024) // each pair is 4 UTF-8 bytes
		p := parcel.New()
		p.WriteString16(cjk)

		p.SetPosition(0)
		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, cjk, s, "large CJK string should round-trip")
	})
}

// ---------------------------------------------------------------------------
// 4. TestParcelEdge_MaxInt32
//
// Verify int32 boundary values (MaxInt32, MinInt32, 0, -1) round-trip.
// ---------------------------------------------------------------------------

func TestParcelEdge_MaxInt32(t *testing.T) {
	cases := []struct {
		name string
		val  int32
	}{
		{"MaxInt32", math.MaxInt32},
		{"MinInt32", math.MinInt32},
		{"Zero", 0},
		{"MinusOne", -1},
		{"One", 1},
	}

	t.Run("LocalRoundTrip", func(t *testing.T) {
		p := parcel.New()
		for _, tc := range cases {
			p.WriteInt32(tc.val)
		}

		p.SetPosition(0)
		for _, tc := range cases {
			v, err := p.ReadInt32()
			require.NoError(t, err, "reading %s", tc.name)
			assert.Equal(t, tc.val, v, "value mismatch for %s", tc.name)
		}
	})

	t.Run("Uint32Boundaries", func(t *testing.T) {
		p := parcel.New()
		p.WriteUint32(0)
		p.WriteUint32(math.MaxUint32)

		p.SetPosition(0)
		v0, err := p.ReadUint32()
		require.NoError(t, err)
		assert.Equal(t, uint32(0), v0)

		vMax, err := p.ReadUint32()
		require.NoError(t, err)
		assert.Equal(t, uint32(math.MaxUint32), vMax)
	})

	t.Run("Int64Boundaries", func(t *testing.T) {
		p := parcel.New()
		p.WriteInt64(math.MaxInt64)
		p.WriteInt64(math.MinInt64)
		p.WriteInt64(0)

		p.SetPosition(0)
		v1, err := p.ReadInt64()
		require.NoError(t, err)
		assert.Equal(t, int64(math.MaxInt64), v1)

		v2, err := p.ReadInt64()
		require.NoError(t, err)
		assert.Equal(t, int64(math.MinInt64), v2)

		v3, err := p.ReadInt64()
		require.NoError(t, err)
		assert.Equal(t, int64(0), v3)
	})

	// Test via real service: checkPermission with uid = MaxInt32 and MinInt32.
	t.Run("RealService_MaxMinUID", func(t *testing.T) {
		ctx := context.Background()
		driver := openBinder(t)
		am := getActivityManager(ctx, t, driver)

		for _, uid := range []int32{math.MaxInt32, math.MinInt32} {
			codeCheckPerm := resolveCode(ctx, t, am, activityManagerDescriptor, "checkPermission")
			data := parcel.New()
			data.WriteInterfaceToken(activityManagerDescriptor)
			data.WriteString16("android.permission.INTERNET")
			data.WriteInt32(int32(os.Getpid()))
			data.WriteInt32(uid)

			reply, err := am.Transact(ctx, codeCheckPerm, 0, data)
			requireOrSkip(t, err)
			requireOrSkip(t, binder.ReadStatus(reply))

			val, err := reply.ReadInt32()
			require.NoError(t, err)
			t.Logf("checkPermission(INTERNET, pid=%d, uid=%d): %d", os.Getpid(), uid, val)
		}
	})
}

// ---------------------------------------------------------------------------
// 5. TestParcelEdge_EmptyArray
//
// Write and read a zero-length typed list and byte array.
// ---------------------------------------------------------------------------

func TestParcelEdge_EmptyArray(t *testing.T) {
	t.Run("EmptyByteArray", func(t *testing.T) {
		p := parcel.New()
		p.WriteByteArray([]byte{})

		p.SetPosition(0)
		result, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.NotNil(t, result, "empty byte array should be non-nil")
		assert.Empty(t, result, "empty byte array should have zero length")
	})

	t.Run("NilByteArray", func(t *testing.T) {
		p := parcel.New()
		p.WriteByteArray(nil)

		p.SetPosition(0)
		result, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.Nil(t, result, "nil byte array should read back as nil")
	})

	t.Run("EmptyTypedList", func(t *testing.T) {
		p := parcel.New()
		err := parcel.WriteTypedList(p, []*edgeTestParcelable{})
		require.NoError(t, err)

		p.SetPosition(0)
		result, err := parcel.ReadTypedList(p, func() *edgeTestParcelable { return &edgeTestParcelable{} })
		require.NoError(t, err)
		assert.NotNil(t, result, "empty typed list should be non-nil")
		assert.Empty(t, result, "empty typed list should have zero length")
	})

	t.Run("NilTypedList", func(t *testing.T) {
		p := parcel.New()
		err := parcel.WriteTypedList[*edgeTestParcelable](p, nil)
		require.NoError(t, err)

		p.SetPosition(0)
		result, err := parcel.ReadTypedList(p, func() *edgeTestParcelable { return &edgeTestParcelable{} })
		require.NoError(t, err)
		assert.Nil(t, result, "nil typed list should read back as nil")
	})

	t.Run("EmptyThenNonEmpty", func(t *testing.T) {
		// Write an empty array followed by a non-empty one;
		// verify position tracking stays correct.
		p := parcel.New()
		p.WriteByteArray([]byte{})
		p.WriteByteArray([]byte{0xAA, 0xBB})

		p.SetPosition(0)
		r1, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.Empty(t, r1)

		r2, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.Equal(t, []byte{0xAA, 0xBB}, r2)
	})
}

// ---------------------------------------------------------------------------
// 6. TestParcelEdge_LargeArray
//
// Write and read a 10000-element typed list and a large byte array.
// ---------------------------------------------------------------------------

func TestParcelEdge_LargeArray(t *testing.T) {
	t.Run("LargeByteArray", func(t *testing.T) {
		data := make([]byte, 10000)
		for i := range data {
			data[i] = byte(i % 256)
		}

		p := parcel.New()
		p.WriteByteArray(data)

		p.SetPosition(0)
		result, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.Equal(t, data, result, "10000-byte array should round-trip exactly")
	})

	t.Run("LargeTypedList", func(t *testing.T) {
		const n = 10000
		items := make([]*edgeTestParcelable, n)
		for i := range items {
			items[i] = &edgeTestParcelable{
				Value: int32(i),
				Name:  strings.Repeat("x", i%10),
			}
		}

		p := parcel.New()
		err := parcel.WriteTypedList(p, items)
		require.NoError(t, err)

		p.SetPosition(0)
		result, err := parcel.ReadTypedList(p, func() *edgeTestParcelable { return &edgeTestParcelable{} })
		require.NoError(t, err)
		require.Len(t, result, n)

		for i, item := range result {
			assert.Equal(t, int32(i), item.Value, "item %d value mismatch", i)
			assert.Equal(t, strings.Repeat("x", i%10), item.Name, "item %d name mismatch", i)
		}
	})

	t.Run("LargeByteArrayAlignmentCheck", func(t *testing.T) {
		// 10001 bytes: not 4-byte aligned, tests padding.
		data := make([]byte, 10001)
		for i := range data {
			data[i] = byte(i % 251) // prime modulus to catch aliasing
		}

		p := parcel.New()
		p.WriteByteArray(data)
		p.WriteInt32(0x12345678) // sentinel after array

		p.SetPosition(0)
		result, err := p.ReadByteArray()
		require.NoError(t, err)
		assert.Equal(t, data, result)

		sentinel, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0x12345678), sentinel, "sentinel after large array should be intact")
	})
}

// ---------------------------------------------------------------------------
// 7. TestParcelEdge_NestedParcelable
//
// Deeply nested parcelable (3 levels) using header/footer protocol.
// ---------------------------------------------------------------------------

func TestParcelEdge_NestedParcelable(t *testing.T) {
	t.Run("ThreeLevelNesting", func(t *testing.T) {
		inner := &nestedParcelable{
			Tag:   "inner",
			Value: 42,
			Child: nil,
		}
		middle := &nestedParcelable{
			Tag:   "middle",
			Value: 100,
			Child: inner,
		}
		outer := &nestedParcelable{
			Tag:   "outer",
			Value: 999,
			Child: middle,
		}

		p := parcel.New()
		err := outer.MarshalParcel(p)
		require.NoError(t, err)

		p.SetPosition(0)
		result := &nestedParcelable{}
		err = result.UnmarshalParcel(p)
		require.NoError(t, err)

		assert.Equal(t, "outer", result.Tag)
		assert.Equal(t, int32(999), result.Value)
		require.NotNil(t, result.Child, "middle should be present")
		assert.Equal(t, "middle", result.Child.Tag)
		assert.Equal(t, int32(100), result.Child.Value)
		require.NotNil(t, result.Child.Child, "inner should be present")
		assert.Equal(t, "inner", result.Child.Child.Tag)
		assert.Equal(t, int32(42), result.Child.Child.Value)
		assert.Nil(t, result.Child.Child.Child, "leaf should have no child")
	})

	t.Run("NullChild", func(t *testing.T) {
		outer := &nestedParcelable{
			Tag:   "leaf",
			Value: 7,
			Child: nil,
		}

		p := parcel.New()
		err := outer.MarshalParcel(p)
		require.NoError(t, err)

		p.SetPosition(0)
		result := &nestedParcelable{}
		err = result.UnmarshalParcel(p)
		require.NoError(t, err)

		assert.Equal(t, "leaf", result.Tag)
		assert.Equal(t, int32(7), result.Value)
		assert.Nil(t, result.Child)
	})

	t.Run("FiveDeepNesting", func(t *testing.T) {
		// Build 5 levels of nesting.
		var node *nestedParcelable
		for depth := 5; depth >= 1; depth-- {
			node = &nestedParcelable{
				Tag:   strings.Repeat("L", depth),
				Value: int32(depth * 10),
				Child: node,
			}
		}

		p := parcel.New()
		err := node.MarshalParcel(p)
		require.NoError(t, err)

		p.SetPosition(0)
		result := &nestedParcelable{}
		err = result.UnmarshalParcel(p)
		require.NoError(t, err)

		// Walk the chain and verify.
		cur := result
		for depth := 1; depth <= 5; depth++ {
			require.NotNil(t, cur, "depth %d should be present", depth)
			assert.Equal(t, strings.Repeat("L", depth), cur.Tag, "depth %d tag", depth)
			assert.Equal(t, int32(depth*10), cur.Value, "depth %d value", depth)
			cur = cur.Child
		}
		assert.Nil(t, cur, "past depth 5 should be nil")
	})

	t.Run("SkipUnknownFieldsForwardCompat", func(t *testing.T) {
		// Simulate a newer version of a parcelable with extra fields.
		// The reader should skip to the parcelable end using the size header.
		p := parcel.New()
		headerPos := parcel.WriteParcelableHeader(p)
		p.WriteInt32(100)
		p.WriteInt32(200)
		p.WriteInt32(300) // unknown extra field
		p.WriteInt64(400) // another unknown field
		parcel.WriteParcelableFooter(p, headerPos)
		// Sentinel after the parcelable.
		p.WriteInt32(0xCAFE)

		p.SetPosition(0)
		endPos, err := parcel.ReadParcelableHeader(p)
		require.NoError(t, err)

		v1, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(100), v1)

		// Skip to end (as an older reader would).
		parcel.SkipToParcelableEnd(p, endPos)

		sentinel, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0xCAFE), sentinel)
	})
}

// ---------------------------------------------------------------------------
// 8. TestParcelEdge_BoolValues
//
// Android writes bool as int32 (0 or 1). Verify our WriteBool/ReadBool,
// and also test non-canonical true values (any non-zero int32 should be true).
// ---------------------------------------------------------------------------

func TestParcelEdge_BoolValues(t *testing.T) {
	t.Run("TrueFalse", func(t *testing.T) {
		p := parcel.New()
		p.WriteBool(true)
		p.WriteBool(false)

		p.SetPosition(0)
		vTrue, err := p.ReadBool()
		require.NoError(t, err)
		assert.True(t, vTrue, "true should round-trip as true")

		vFalse, err := p.ReadBool()
		require.NoError(t, err)
		assert.False(t, vFalse, "false should round-trip as false")
	})

	t.Run("WriteTrueAsInt1", func(t *testing.T) {
		// Verify that WriteBool(true) writes exactly 1 (not some other non-zero).
		p := parcel.New()
		p.WriteBool(true)

		p.SetPosition(0)
		raw, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), raw, "WriteBool(true) should write int32(1)")
	})

	t.Run("WriteFalseAsInt0", func(t *testing.T) {
		p := parcel.New()
		p.WriteBool(false)

		p.SetPosition(0)
		raw, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0), raw, "WriteBool(false) should write int32(0)")
	})

	t.Run("NonCanonicalTrue", func(t *testing.T) {
		// Android's ReadBool treats any non-zero int32 as true.
		// Verify our ReadBool does the same.
		nonZeroValues := []int32{2, -1, math.MaxInt32, math.MinInt32, 42}
		for _, nz := range nonZeroValues {
			p := parcel.New()
			p.WriteInt32(nz) // write raw int32 instead of WriteBool

			p.SetPosition(0)
			v, err := p.ReadBool()
			require.NoError(t, err)
			assert.True(t, v, "int32(%d) should read as true via ReadBool", nz)
		}
	})

	t.Run("RealService_Bool", func(t *testing.T) {
		ctx := context.Background()
		driver := openBinder(t)
		am := getActivityManager(ctx, t, driver)

		// isUserAMonkey returns a bool.
		reply := transactNoArg(ctx, t, am, activityManagerDescriptor, "isUserAMonkey")
		v, err := reply.ReadBool()
		require.NoError(t, err)
		assert.False(t, v, "should not be a monkey in test")

		// Verify the raw int32 is 0 (canonical false), not just any value
		// that happens to equal false. Re-read from a fresh parcel.
		reply2 := transactNoArg(ctx, t, am, activityManagerDescriptor, "isUserAMonkey")
		raw, err := reply2.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0), raw, "Android should write canonical 0 for false")
		t.Logf("isUserAMonkey raw int32: %d", raw)
	})
}

// ---------------------------------------------------------------------------
// 9. TestParcelEdge_Float64Precision
//
// Verify that float64 special values and full precision are preserved.
// ---------------------------------------------------------------------------

func TestParcelEdge_Float64Precision(t *testing.T) {
	t.Run("SpecialValues", func(t *testing.T) {
		values := []struct {
			name string
			val  float64
		}{
			{"Pi", math.Pi},
			{"E", math.E},
			{"MaxFloat64", math.MaxFloat64},
			{"SmallestNonzero", math.SmallestNonzeroFloat64},
			{"NegativeZero", math.Copysign(0, -1)},
			{"PositiveZero", 0.0},
			{"Inf", math.Inf(1)},
			{"NegInf", math.Inf(-1)},
		}

		p := parcel.New()
		for _, v := range values {
			p.WriteFloat64(v.val)
		}

		p.SetPosition(0)
		for _, v := range values {
			got, err := p.ReadFloat64()
			require.NoError(t, err, "reading %s", v.name)
			assert.Equal(t, math.Float64bits(v.val), math.Float64bits(got),
				"%s: bit-exact equality failed (want %v, got %v)", v.name, v.val, got)
		}
	})

	t.Run("NaN", func(t *testing.T) {
		p := parcel.New()
		p.WriteFloat64(math.NaN())

		p.SetPosition(0)
		got, err := p.ReadFloat64()
		require.NoError(t, err)
		assert.True(t, math.IsNaN(got), "NaN should round-trip as NaN")
	})

	t.Run("Float32Precision", func(t *testing.T) {
		values := []float32{
			math.MaxFloat32,
			math.SmallestNonzeroFloat32,
			float32(math.Pi),
			0.0,
			float32(math.Inf(1)),
			float32(math.Inf(-1)),
		}

		p := parcel.New()
		for _, v := range values {
			p.WriteFloat32(v)
		}

		p.SetPosition(0)
		for _, v := range values {
			got, err := p.ReadFloat32()
			require.NoError(t, err)
			assert.Equal(t, math.Float32bits(v), math.Float32bits(got),
				"float32 bit-exact equality failed for %v", v)
		}
	})

	t.Run("Float32NaN", func(t *testing.T) {
		p := parcel.New()
		p.WriteFloat32(float32(math.NaN()))

		p.SetPosition(0)
		got, err := p.ReadFloat32()
		require.NoError(t, err)
		assert.True(t, math.IsNaN(float64(got)), "float32 NaN should round-trip")
	})

	t.Run("MixedTypes", func(t *testing.T) {
		// Interleave float64 with other types to verify alignment.
		p := parcel.New()
		p.WriteFloat64(math.Pi)
		p.WriteInt32(42)
		p.WriteFloat64(math.E)
		p.WriteString16("test")
		p.WriteFloat64(math.Phi)

		p.SetPosition(0)

		v1, err := p.ReadFloat64()
		require.NoError(t, err)
		assert.Equal(t, math.Pi, v1)

		v2, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(42), v2)

		v3, err := p.ReadFloat64()
		require.NoError(t, err)
		assert.Equal(t, math.E, v3)

		v4, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "test", v4)

		v5, err := p.ReadFloat64()
		require.NoError(t, err)
		assert.Equal(t, math.Phi, v5)
	})
}

// ---------------------------------------------------------------------------
// 10. TestParcelEdge_FileDescriptor
//
// Test file descriptor serialization via the parcel library.
// Uses a pipe to create real FDs, tests both raw FD and ParcelFileDescriptor.
// ---------------------------------------------------------------------------

func TestParcelEdge_FileDescriptor(t *testing.T) {
	t.Run("RawFD_RoundTrip", func(t *testing.T) {
		// Create a pipe to get valid FDs.
		r, w, err := osPipe()
		require.NoError(t, err)
		defer syscall.Close(r)
		defer syscall.Close(w)

		p := parcel.New()
		p.WriteFileDescriptor(int32(r))

		assert.Len(t, p.Objects(), 1, "should have one binder object for FD")

		p.SetPosition(0)
		fdOut, err := p.ReadFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(r), fdOut, "FD should round-trip")
	})

	t.Run("ParcelFileDescriptor_NonNull", func(t *testing.T) {
		r, w, err := osPipe()
		require.NoError(t, err)
		defer syscall.Close(r)
		defer syscall.Close(w)

		p := parcel.New()
		p.WriteParcelFileDescriptor(int32(r))

		p.SetPosition(0)
		fdOut, err := p.ReadParcelFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(r), fdOut, "ParcelFileDescriptor should round-trip")
	})

	t.Run("ParcelFileDescriptor_Null", func(t *testing.T) {
		p := parcel.New()
		p.WriteParcelFileDescriptor(-1) // null PFD

		p.SetPosition(0)
		fdOut, err := p.ReadParcelFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(-1), fdOut, "null ParcelFileDescriptor should read as -1")
	})

	t.Run("MultipleFDs", func(t *testing.T) {
		r1, w1, err := osPipe()
		require.NoError(t, err)
		defer syscall.Close(r1)
		defer syscall.Close(w1)

		r2, w2, err := osPipe()
		require.NoError(t, err)
		defer syscall.Close(r2)
		defer syscall.Close(w2)

		p := parcel.New()
		p.WriteFileDescriptor(int32(r1))
		p.WriteFileDescriptor(int32(r2))

		assert.Len(t, p.Objects(), 2, "should have two binder objects for FDs")

		p.SetPosition(0)
		fd1, err := p.ReadFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(r1), fd1)

		fd2, err := p.ReadFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(r2), fd2)
	})

	t.Run("FDWithOtherData", func(t *testing.T) {
		r, w, err := osPipe()
		require.NoError(t, err)
		defer syscall.Close(r)
		defer syscall.Close(w)

		p := parcel.New()
		p.WriteInt32(0xBEEF)
		p.WriteFileDescriptor(int32(r))
		p.WriteString16("after-fd")

		p.SetPosition(0)

		v, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0xBEEF), v)

		fdOut, err := p.ReadFileDescriptor()
		require.NoError(t, err)
		assert.Equal(t, int32(r), fdOut)

		s, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "after-fd", s)
	})
}

// ===========================================================================
// Helper types and functions
// ===========================================================================

// edgeTestParcelable is a simple Parcelable for typed list tests.
type edgeTestParcelable struct {
	Value int32
	Name  string
}

func (p *edgeTestParcelable) MarshalParcel(out *parcel.Parcel) error {
	out.WriteInt32(p.Value)
	out.WriteString16(p.Name)
	return nil
}

func (p *edgeTestParcelable) UnmarshalParcel(in *parcel.Parcel) error {
	v, err := in.ReadInt32()
	if err != nil {
		return err
	}
	p.Value = v

	s, err := in.ReadString16()
	if err != nil {
		return err
	}
	p.Name = s
	return nil
}

// nestedParcelable is a recursive Parcelable for nesting tests.
// Wire format: header(size) + string16(tag) + int32(value) + int32(hasChild) + [child...]
type nestedParcelable struct {
	Tag   string
	Value int32
	Child *nestedParcelable
}

func (n *nestedParcelable) MarshalParcel(p *parcel.Parcel) error {
	headerPos := parcel.WriteParcelableHeader(p)
	p.WriteString16(n.Tag)
	p.WriteInt32(n.Value)
	if n.Child != nil {
		p.WriteInt32(1) // hasChild
		if err := n.Child.MarshalParcel(p); err != nil {
			return err
		}
	} else {
		p.WriteInt32(0) // no child
	}
	parcel.WriteParcelableFooter(p, headerPos)
	return nil
}

func (n *nestedParcelable) UnmarshalParcel(p *parcel.Parcel) error {
	endPos, err := parcel.ReadParcelableHeader(p)
	if err != nil {
		return err
	}

	n.Tag, err = p.ReadString16()
	if err != nil {
		return err
	}

	n.Value, err = p.ReadInt32()
	if err != nil {
		return err
	}

	hasChild, err := p.ReadInt32()
	if err != nil {
		return err
	}

	if hasChild != 0 {
		n.Child = &nestedParcelable{}
		if err := n.Child.UnmarshalParcel(p); err != nil {
			return err
		}
	}

	parcel.SkipToParcelableEnd(p, endPos)
	return nil
}

// osPipe creates a pipe using syscall and returns the read and write FDs.
func osPipe() (r, w int, err error) {
	var fds [2]int
	err = syscall.Pipe(fds[:])
	if err != nil {
		return 0, 0, err
	}
	return fds[0], fds[1], nil
}
