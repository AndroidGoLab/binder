package parcel

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInt32RoundTrip(t *testing.T) {
	p := New()
	p.WriteInt32(42)
	p.WriteInt32(-1)
	p.WriteInt32(math.MaxInt32)
	p.WriteInt32(math.MinInt32)

	p.SetPosition(0)

	v, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(42), v)

	v, err = p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(-1), v)

	v, err = p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(math.MaxInt32), v)

	v, err = p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(math.MinInt32), v)
}

func TestUint32RoundTrip(t *testing.T) {
	p := New()
	p.WriteUint32(0)
	p.WriteUint32(math.MaxUint32)

	p.SetPosition(0)

	v, err := p.ReadUint32()
	require.NoError(t, err)
	assert.Equal(t, uint32(0), v)

	v, err = p.ReadUint32()
	require.NoError(t, err)
	assert.Equal(t, uint32(math.MaxUint32), v)
}

func TestInt64RoundTrip(t *testing.T) {
	p := New()
	p.WriteInt64(123456789012345)
	p.WriteInt64(math.MinInt64)

	p.SetPosition(0)

	v, err := p.ReadInt64()
	require.NoError(t, err)
	assert.Equal(t, int64(123456789012345), v)

	v, err = p.ReadInt64()
	require.NoError(t, err)
	assert.Equal(t, int64(math.MinInt64), v)
}

func TestUint64RoundTrip(t *testing.T) {
	p := New()
	p.WriteUint64(math.MaxUint64)

	p.SetPosition(0)

	v, err := p.ReadUint64()
	require.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64), v)
}

func TestBoolRoundTrip(t *testing.T) {
	p := New()
	p.WriteBool(true)
	p.WriteBool(false)

	p.SetPosition(0)

	v, err := p.ReadBool()
	require.NoError(t, err)
	assert.True(t, v)

	v, err = p.ReadBool()
	require.NoError(t, err)
	assert.False(t, v)
}

func TestFloat32RoundTrip(t *testing.T) {
	p := New()
	p.WriteFloat32(3.14)
	p.WriteFloat32(0.0)
	p.WriteFloat32(math.MaxFloat32)

	p.SetPosition(0)

	v, err := p.ReadFloat32()
	require.NoError(t, err)
	assert.InDelta(t, float32(3.14), v, 0.001)

	v, err = p.ReadFloat32()
	require.NoError(t, err)
	assert.Equal(t, float32(0.0), v)

	v, err = p.ReadFloat32()
	require.NoError(t, err)
	assert.Equal(t, float32(math.MaxFloat32), v)
}

func TestFloat64RoundTrip(t *testing.T) {
	p := New()
	p.WriteFloat64(math.Pi)
	p.WriteFloat64(math.SmallestNonzeroFloat64)

	p.SetPosition(0)

	v, err := p.ReadFloat64()
	require.NoError(t, err)
	assert.Equal(t, math.Pi, v)

	v, err = p.ReadFloat64()
	require.NoError(t, err)
	assert.Equal(t, math.SmallestNonzeroFloat64, v)
}

func TestByteRoundTrip(t *testing.T) {
	p := New()
	p.WritePaddedByte(0xFF)
	p.WritePaddedByte(0x00)

	p.SetPosition(0)

	v, err := p.ReadPaddedByte()
	require.NoError(t, err)
	assert.Equal(t, byte(0xFF), v)

	v, err = p.ReadPaddedByte()
	require.NoError(t, err)
	assert.Equal(t, byte(0x00), v)

	// Each byte consumes 4 bytes due to padding.
	assert.Equal(t, 8, p.Len())
}

func TestString16ASCII(t *testing.T) {
	p := New()
	p.WriteString16("hello")

	p.SetPosition(0)

	s, err := p.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "hello", s)
}

func TestString16Unicode(t *testing.T) {
	tests := []string{
		"\U0001F600",       // emoji (surrogate pair in UTF-16)
		"\u4F60\u597D",     // CJK characters
		"abc\U0001F600def", // mixed ASCII and emoji
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			p := New()
			p.WriteString16(tc)

			p.SetPosition(0)

			s, err := p.ReadString16()
			require.NoError(t, err)
			assert.Equal(t, tc, s)
		})
	}
}

func TestString16Empty(t *testing.T) {
	p := New()
	p.WriteString16("")

	p.SetPosition(0)

	s, err := p.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "", s)

	// Empty string writes length 0 (4 bytes) + null terminator (2 bytes, padded to 4) = 8 bytes.
	assert.Equal(t, 8, p.Len())
}

func TestString16Null(t *testing.T) {
	p := New()
	p.WriteNullString16()

	p.SetPosition(0)

	s, err := p.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "", s)

	// Null string writes only -1 as length (4 bytes).
	assert.Equal(t, 4, p.Len())
}

func TestNullableString16Null(t *testing.T) {
	p := New()
	p.WriteNullString16()

	p.SetPosition(0)

	s, err := p.ReadNullableString16()
	require.NoError(t, err)
	assert.Nil(t, s, "ReadNullableString16 should return nil for null string")
}

func TestNullableString16Empty(t *testing.T) {
	p := New()
	p.WriteString16("")

	p.SetPosition(0)

	s, err := p.ReadNullableString16()
	require.NoError(t, err)
	require.NotNil(t, s, "ReadNullableString16 should return non-nil for empty string")
	assert.Equal(t, "", *s)
}

func TestNullableString16Value(t *testing.T) {
	p := New()
	p.WriteString16("hello")

	p.SetPosition(0)

	s, err := p.ReadNullableString16()
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "hello", *s)
}

func TestStringUTF8RoundTrip(t *testing.T) {
	tests := []string{
		"hello world",
		"\U0001F600",
		"\u4F60\u597D",
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			p := New()
			p.WriteString(tc)

			p.SetPosition(0)

			s, err := p.ReadString()
			require.NoError(t, err)
			assert.Equal(t, tc, s)
		})
	}
}

func TestStringUTF8Empty(t *testing.T) {
	p := New()
	p.WriteString("")

	p.SetPosition(0)

	s, err := p.ReadString()
	require.NoError(t, err)
	assert.Equal(t, "", s)

	// Empty string writes length 0 (4 bytes) + null terminator (1 byte, padded to 4) = 8 bytes.
	assert.Equal(t, 8, p.Len())
}

func TestStringUTF8Null(t *testing.T) {
	p := New()
	p.WriteNullString()

	p.SetPosition(0)

	s, err := p.ReadString()
	require.NoError(t, err)
	assert.Equal(t, "", s)

	// Null string writes only -1 as length (4 bytes).
	assert.Equal(t, 4, p.Len())
}

func TestNullableStringNull(t *testing.T) {
	p := New()
	p.WriteNullString()

	p.SetPosition(0)

	s, err := p.ReadNullableString()
	require.NoError(t, err)
	assert.Nil(t, s, "ReadNullableString should return nil for null string")
}

func TestNullableStringEmpty(t *testing.T) {
	p := New()
	p.WriteString("")

	p.SetPosition(0)

	s, err := p.ReadNullableString()
	require.NoError(t, err)
	require.NotNil(t, s, "ReadNullableString should return non-nil for empty string")
	assert.Equal(t, "", *s)
}

func TestNullableStringValue(t *testing.T) {
	p := New()
	p.WriteString("hello")

	p.SetPosition(0)

	s, err := p.ReadNullableString()
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "hello", *s)
}

func TestInterfaceTokenRoundTrip(t *testing.T) {
	p := New()
	p.WriteInterfaceToken("android.os.IServiceManager")

	p.SetPosition(0)

	desc, err := p.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, "android.os.IServiceManager", desc)
}

func TestByteArrayRoundTrip(t *testing.T) {
	p := New()
	data := []byte{1, 2, 3, 4, 5}
	p.WriteByteArray(data)

	p.SetPosition(0)

	result, err := p.ReadByteArray()
	require.NoError(t, err)
	assert.Equal(t, data, result)
}

func TestByteArrayNil(t *testing.T) {
	p := New()
	p.WriteByteArray(nil)

	p.SetPosition(0)

	result, err := p.ReadByteArray()
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestByteArrayEmpty(t *testing.T) {
	p := New()
	p.WriteByteArray([]byte{})

	p.SetPosition(0)

	result, err := p.ReadByteArray()
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestStrongBinderRoundTrip(t *testing.T) {
	p := New()
	p.WriteStrongBinder(42)

	assert.Len(t, p.Objects(), 1)
	assert.Equal(t, uint64(0), p.Objects()[0])

	p.SetPosition(0)

	handle, err := p.ReadStrongBinder()
	require.NoError(t, err)
	assert.Equal(t, uint32(42), handle)
}

func TestReadStrongBinderRejectsNull(t *testing.T) {
	p := New()
	p.WriteNullStrongBinder()

	p.SetPosition(0)

	_, err := p.ReadStrongBinder()
	require.Error(t, err, "ReadStrongBinder should reject null binder")
	assert.Contains(t, err.Error(), "unexpected null binder")
}

func TestNullableStrongBinderAcceptsNull(t *testing.T) {
	p := New()
	p.WriteNullStrongBinder()

	p.SetPosition(0)

	_, valid, err := p.ReadNullableStrongBinder()
	require.NoError(t, err)
	assert.False(t, valid, "ReadNullableStrongBinder should report null as not-valid")
}

func TestFileDescriptorRoundTrip(t *testing.T) {
	p := New()
	p.WriteFileDescriptor(7)

	assert.Len(t, p.Objects(), 1)

	p.SetPosition(0)

	fd, err := p.ReadFileDescriptor()
	require.NoError(t, err)
	assert.Equal(t, int32(7), fd)
}

// errorParcelable is a test type that fails marshaling/unmarshaling.
type errorParcelable struct {
	shouldFail bool
}

func (ep *errorParcelable) MarshalParcel(
	p *Parcel,
) error {
	if ep.shouldFail {
		return fmt.Errorf("intentional marshal error")
	}
	return nil
}

func (ep *errorParcelable) UnmarshalParcel(
	p *Parcel,
) error {
	if ep.shouldFail {
		return fmt.Errorf("intentional unmarshal error")
	}
	return nil
}

// testParcelable is a test type implementing Parcelable.
type testParcelable struct {
	Value int32
	Name  string
}

func (tp *testParcelable) MarshalParcel(
	p *Parcel,
) error {
	p.WriteInt32(tp.Value)
	p.WriteString16(tp.Name)
	return nil
}

func (tp *testParcelable) UnmarshalParcel(
	p *Parcel,
) error {
	v, err := p.ReadInt32()
	if err != nil {
		return err
	}
	tp.Value = v

	s, err := p.ReadString16()
	if err != nil {
		return err
	}
	tp.Name = s
	return nil
}

func TestTypedListRoundTrip(t *testing.T) {
	items := []*testParcelable{
		{Value: 1, Name: "first"},
		{Value: 2, Name: "second"},
		{Value: 3, Name: "third"},
	}

	p := New()
	err := WriteTypedList(p, items)
	require.NoError(t, err)

	p.SetPosition(0)

	result, err := ReadTypedList(p, func() *testParcelable { return &testParcelable{} })
	require.NoError(t, err)
	require.Len(t, result, 3)

	for i, item := range result {
		assert.Equal(t, items[i].Value, item.Value)
		assert.Equal(t, items[i].Name, item.Name)
	}
}

func TestTypedListNil(t *testing.T) {
	p := New()
	err := WriteTypedList[*testParcelable](p, nil)
	require.NoError(t, err)

	p.SetPosition(0)

	result, err := ReadTypedList(p, func() *testParcelable { return &testParcelable{} })
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParcelableHeaderFooter(t *testing.T) {
	p := New()

	headerPos := WriteParcelableHeader(p)
	p.WriteInt32(100)
	p.WriteInt32(200)
	WriteParcelableFooter(p, headerPos)

	p.SetPosition(0)

	endPos, err := ReadParcelableHeader(p)
	require.NoError(t, err)

	v1, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(100), v1)

	v2, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(200), v2)

	assert.Equal(t, endPos, p.Position())
}

func TestParcelableHeaderFooterSkip(t *testing.T) {
	p := New()

	headerPos := WriteParcelableHeader(p)
	p.WriteInt32(100)
	p.WriteInt32(200)
	p.WriteInt32(300) // extra field for forward compat
	WriteParcelableFooter(p, headerPos)

	// Write something after the parcelable.
	p.WriteInt32(999)

	p.SetPosition(0)

	endPos, err := ReadParcelableHeader(p)
	require.NoError(t, err)

	// Read only the first field.
	v1, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(100), v1)

	// Skip remaining fields.
	SkipToParcelableEnd(p, endPos)

	// Read the value after the parcelable.
	v999, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(999), v999)
}

func TestMultipleWritesThenReads(t *testing.T) {
	p := New()
	p.WriteInt32(1)
	p.WriteInt64(2)
	p.WriteBool(true)
	p.WriteString16("test")
	p.WriteFloat64(3.14)

	p.SetPosition(0)

	v1, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(1), v1)

	v2, err := p.ReadInt64()
	require.NoError(t, err)
	assert.Equal(t, int64(2), v2)

	v3, err := p.ReadBool()
	require.NoError(t, err)
	assert.True(t, v3)

	v4, err := p.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "test", v4)

	v5, err := p.ReadFloat64()
	require.NoError(t, err)
	assert.InDelta(t, 3.14, v5, 0.001)
}

func TestReadBeyondEnd(t *testing.T) {
	p := New()
	p.WriteInt32(1)

	p.SetPosition(0)

	_, err := p.ReadInt32()
	require.NoError(t, err)

	_, err = p.ReadInt32()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read beyond end")
}

func TestRecycle(t *testing.T) {
	p := New()
	p.WriteInt32(42)
	p.WriteStrongBinder(1)

	assert.Greater(t, p.Len(), 0)
	assert.NotEmpty(t, p.Objects())

	p.Recycle()

	assert.Equal(t, 0, p.Len())
	assert.Equal(t, 0, p.Position())
	assert.Empty(t, p.Objects())
}

func TestFromBytes(t *testing.T) {
	// Create a parcel, serialize, then recreate from bytes.
	p1 := New()
	p1.WriteInt32(42)
	p1.WriteInt32(99)

	p2 := FromBytes(p1.Data())

	v1, err := p2.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(42), v1)

	v2, err := p2.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(99), v2)
}

func TestGrowZeroFillsPadding(t *testing.T) {
	p := New()

	// Write 3 bytes of data (will be padded to 4).
	p.WriteByteArray([]byte{0xAA, 0xBB, 0xCC})

	// The byte array: 4 bytes length + 4 bytes data (3 data + 1 padding).
	// Verify the padding byte is zero.
	data := p.Data()
	// Length prefix (int32 = 3): bytes 0-3
	// Data: bytes 4-6 = 0xAA, 0xBB, 0xCC
	// Padding byte: byte 7
	assert.Equal(t, byte(0), data[7], "padding byte should be zero")
}

func TestString16Alignment(t *testing.T) {
	p := New()
	// "ab" = 2 UTF-16 code units => 4 bytes data + 2 bytes null = 6 bytes, padded to 8.
	p.WriteString16("ab")
	p.WriteInt32(42)

	p.SetPosition(0)

	s, err := p.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "ab", s)

	v, err := p.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(42), v)
}

func TestReadBeyondEndAllTypes(t *testing.T) {
	empty := New()

	_, err := empty.ReadUint32()
	assert.Error(t, err)

	_, err = empty.ReadInt64()
	assert.Error(t, err)

	_, err = empty.ReadUint64()
	assert.Error(t, err)

	_, err = empty.ReadBool()
	assert.Error(t, err)

	_, err = empty.ReadFloat32()
	assert.Error(t, err)

	_, err = empty.ReadFloat64()
	assert.Error(t, err)

	_, err = empty.ReadPaddedByte()
	assert.Error(t, err)

	_, err = empty.ReadByteArray()
	assert.Error(t, err)

	_, err = empty.ReadString16()
	assert.Error(t, err)

	_, err = empty.ReadString()
	assert.Error(t, err)

	_, err = empty.ReadStrongBinder()
	assert.Error(t, err)

	_, err = empty.ReadFileDescriptor()
	assert.Error(t, err)

	_, err = ReadParcelableHeader(empty)
	assert.Error(t, err)

	_, err = empty.ReadInterfaceToken()
	assert.Error(t, err)
}

func TestReadStrongBinderWrongType(t *testing.T) {
	p := New()
	p.WriteFileDescriptor(5)

	p.SetPosition(0)

	_, err := p.ReadStrongBinder()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected binder type")
}

func TestReadFileDescriptorWrongType(t *testing.T) {
	p := New()
	p.WriteStrongBinder(5)

	p.SetPosition(0)

	_, err := p.ReadFileDescriptor()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected binder FD type")
}

func TestWriteTypedListMarshalError(t *testing.T) {
	items := []*errorParcelable{{shouldFail: true}}

	p := New()
	err := WriteTypedList(p, items)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshaling list item")
}

func TestReadTypedListUnmarshalError(t *testing.T) {
	// Write a list count but no data.
	p := New()
	p.WriteInt32(1) // 1 item

	p.SetPosition(0)

	_, err := ReadTypedList(p, func() *errorParcelable { return &errorParcelable{shouldFail: true} })
	assert.Error(t, err)
}

func TestReadByteArrayDataError(t *testing.T) {
	// Write a length but not enough data.
	p := New()
	p.WriteInt32(100) // claim 100 bytes

	p.SetPosition(0)

	_, err := p.ReadByteArray()
	assert.Error(t, err)
}

func TestReadString16DataError(t *testing.T) {
	// Write a char count but not enough data.
	p := New()
	p.WriteInt32(100) // claim 100 chars

	p.SetPosition(0)

	_, err := p.ReadString16()
	assert.Error(t, err)
}

func TestReadStringDataError(t *testing.T) {
	// Write a byte length but not enough data.
	p := New()
	p.WriteInt32(100) // claim 100 bytes

	p.SetPosition(0)

	_, err := p.ReadString()
	assert.Error(t, err)
}

func TestReadTypedListCountError(t *testing.T) {
	empty := New()
	_, err := ReadTypedList(empty, func() *testParcelable { return &testParcelable{} })
	assert.Error(t, err)
}

func TestMultipleStrongBinders(t *testing.T) {
	p := New()
	p.WriteStrongBinder(10)
	p.WriteStrongBinder(20)

	// Each binder object is flat_binder_object (24 bytes) + int32 stability (4 bytes) = 28 bytes.
	binderWithStabilitySize := uint64(flatBinderObjectSize + 4)
	assert.Len(t, p.Objects(), 2)
	assert.Equal(t, uint64(0), p.Objects()[0])
	assert.Equal(t, binderWithStabilitySize, p.Objects()[1])

	p.SetPosition(0)

	h1, err := p.ReadStrongBinder()
	require.NoError(t, err)
	assert.Equal(t, uint32(10), h1)

	h2, err := p.ReadStrongBinder()
	require.NoError(t, err)
	assert.Equal(t, uint32(20), h2)
}

func TestWriteReadFixedByteArray(t *testing.T) {
	p := New()

	// Write a short name into a fixed 128-byte array.
	name := []byte("test")
	p.WriteFixedByteArray(name, 128)

	p.SetPosition(0)

	result, err := p.ReadFixedByteArray(128)
	require.NoError(t, err)
	assert.Len(t, result, 128)
	assert.Equal(t, []byte("test"), result[:4])
	// Remaining bytes must be zero.
	for i := 4; i < 128; i++ {
		assert.Equal(t, byte(0), result[i], "byte at index %d should be 0", i)
	}
}

func TestWriteReadFixedByteArrayFull(t *testing.T) {
	p := New()

	// Write a full 128-byte array.
	data := make([]byte, 128)
	for i := range data {
		data[i] = byte(i)
	}
	p.WriteFixedByteArray(data, 128)

	p.SetPosition(0)

	result, err := p.ReadFixedByteArray(128)
	require.NoError(t, err)
	assert.Equal(t, data, result)
}

func TestReadFixedByteArraySizeMismatch(t *testing.T) {
	p := New()

	// Write with size 64, but try to read as fixed size 128.
	p.WriteInt32(64)
	p.WriteRawBytes(make([]byte, 64))

	p.SetPosition(0)

	_, err := p.ReadFixedByteArray(128)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "size mismatch")
}

func TestWriteFixedByteArrayTruncatesLongInput(t *testing.T) {
	p := New()

	// Input longer than fixed size should be truncated.
	long := make([]byte, 256)
	for i := range long {
		long[i] = byte(i % 256)
	}
	p.WriteFixedByteArray(long, 128)

	p.SetPosition(0)

	result, err := p.ReadFixedByteArray(128)
	require.NoError(t, err)
	assert.Equal(t, long[:128], result)
}
