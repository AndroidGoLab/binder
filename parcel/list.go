package parcel

import (
	"fmt"
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

	p.WriteInt32(int32(len(items)))
	for i, item := range items {
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

	items := make([]T, count)
	for i := range items {
		items[i] = factory()
		if err := items[i].UnmarshalParcel(p); err != nil {
			return nil, fmt.Errorf("parcel: unmarshaling list item %d: %w", i, err)
		}
	}

	return items, nil
}
