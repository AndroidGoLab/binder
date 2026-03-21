package parcel

import (
	"fmt"
	"math"
)

const (
	// maxListCount is the maximum number of items allowed when reading a typed
	// list from a parcel. This guards against OOM from malicious or corrupted
	// parcels while allowing legitimate large lists.
	maxListCount = 1_000_000
)

// WriteTypedList writes a list of Parcelable items.
// Writes int32 count (or -1 for nil slice), then marshals each item.
func WriteTypedList[T Parcelable](
	p *Parcel,
	items []T,
) error {
	if items == nil {
		p.WriteInt32(-1)
		return nil
	}

	if len(items) > math.MaxInt32 {
		return fmt.Errorf("parcel: WriteTypedList count %d exceeds int32 max", len(items))
	}

	p.WriteInt32(int32(len(items)))
	for i, item := range items {
		// Non-null indicator (1) before each element, matching AOSP's
		// writeTypedList which writes 1 for present elements.
		p.WriteInt32(1)
		if err := item.MarshalParcel(p); err != nil {
			return fmt.Errorf("parcel: marshaling list item %d: %w", i, err)
		}
	}

	return nil
}

// ReadTypedList reads a list of Parcelable items using the provided factory
// to create new instances. Returns nil if the count is -1.
func ReadTypedList[T Parcelable](
	p *Parcel,
	factory func() T,
) ([]T, error) {
	count, err := p.ReadInt32()
	if err != nil {
		return nil, err
	}

	if count < 0 {
		return nil, nil
	}

	if int(count) > maxListCount {
		return nil, fmt.Errorf("parcel: ReadTypedList count %d exceeds limit %d", count, maxListCount)
	}

	items := make([]T, count)
	for i := range items {
		// Read per-element null indicator, matching AOSP's readTypedList.
		// 0 means null element (skip), non-zero means present.
		nullInd, err := p.ReadInt32()
		if err != nil {
			return nil, fmt.Errorf("parcel: reading list item %d null indicator: %w", i, err)
		}
		if nullInd == 0 {
			continue
		}

		items[i] = factory()
		if err := items[i].UnmarshalParcel(p); err != nil {
			return nil, fmt.Errorf("parcel: unmarshaling list item %d: %w", i, err)
		}
	}

	return items, nil
}
