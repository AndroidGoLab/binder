package view

import (
	"fmt"

	"github.com/AndroidGoLab/binder/parcel"
)

// KeyCharacterMap is a native C++ parcelable whose wire format is defined
// by JNI (android_view_KeyCharacterMap.cpp) and libinput (KeyCharacterMap.cpp).
//
// JNI envelope (nativeWriteToParcel):
//
//	int32  deviceId
//	bool   hasMap (int32: 0 or 1)
//
// If hasMap, KeyCharacterMap::writeToParcel follows:
//
//	CString  loadFileName  (null-terminated, no length prefix)
//	int32    type
//	bool     layoutOverlayApplied
//	int32    numKeys
//	per key:
//	  int32  keyCode
//	  int32  label
//	  int32  number
//	  repeated {int32(1), metaState, character, fallbackKeyCode, replacementKeyCode}
//	  int32(0)  sentinel
//	int32 numKeyRemapping,  pairs of (from, to) int32
//	int32 numKeysByScanCode, pairs of (scanCode, keyCode) int32
//	int32 numKeysByUsageCode, pairs of (usageCode, keyCode) int32
type KeyCharacterMap struct {
	DeviceId int32
}

const maxKeyCharMapKeys = 8192

var _ parcel.Parcelable = (*KeyCharacterMap)(nil)

func (s *KeyCharacterMap) MarshalParcel(
	p *parcel.Parcel,
) error {
	p.WriteInt32(s.DeviceId)
	p.WriteBool(false) // hasMap = false
	return nil
}

func (s *KeyCharacterMap) UnmarshalParcel(
	p *parcel.Parcel,
) error {
	var err error
	s.DeviceId, err = p.ReadInt32()
	if err != nil {
		return fmt.Errorf("KeyCharacterMap: reading deviceId: %w", err)
	}

	hasMap, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("KeyCharacterMap: reading hasMap: %w", err)
	}
	if !hasMap {
		return nil
	}

	// CString loadFileName (null-terminated, no length prefix).
	if _, err = p.ReadCString(); err != nil {
		return fmt.Errorf("KeyCharacterMap: reading loadFileName: %w", err)
	}

	// int32 type
	if _, err = p.ReadInt32(); err != nil {
		return fmt.Errorf("KeyCharacterMap: reading type: %w", err)
	}

	// bool layoutOverlayApplied
	if _, err = p.ReadBool(); err != nil {
		return fmt.Errorf("KeyCharacterMap: reading layoutOverlayApplied: %w", err)
	}

	// int32 numKeys
	numKeys, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("KeyCharacterMap: reading numKeys: %w", err)
	}
	if numKeys < 0 || numKeys > maxKeyCharMapKeys {
		return fmt.Errorf("KeyCharacterMap: numKeys %d out of range", numKeys)
	}

	for i := int32(0); i < numKeys; i++ {
		// keyCode, label, number
		for _, field := range []string{"keyCode", "label", "number"} {
			if _, err = p.ReadInt32(); err != nil {
				return fmt.Errorf("KeyCharacterMap: key[%d] %s: %w", i, field, err)
			}
		}
		// Sentinel-terminated behavior list.
		for {
			sentinel, serr := p.ReadInt32()
			if serr != nil {
				return fmt.Errorf("KeyCharacterMap: key[%d] behavior sentinel: %w", i, serr)
			}
			if sentinel == 0 {
				break
			}
			for _, field := range []string{"metaState", "character", "fallbackKeyCode", "replacementKeyCode"} {
				if _, err = p.ReadInt32(); err != nil {
					return fmt.Errorf("KeyCharacterMap: key[%d] behavior %s: %w", i, field, err)
				}
			}
		}
	}

	// Three mapping tables: keyRemapping, keysByScanCode, keysByUsageCode.
	for _, tableName := range []string{"keyRemapping", "keysByScanCode", "keysByUsageCode"} {
		count, cerr := p.ReadInt32()
		if cerr != nil {
			return fmt.Errorf("KeyCharacterMap: reading %s count: %w", tableName, cerr)
		}
		for j := int32(0); j < count; j++ {
			if _, err = p.ReadInt32(); err != nil {
				return fmt.Errorf("KeyCharacterMap: %s[%d] key: %w", tableName, j, err)
			}
			if _, err = p.ReadInt32(); err != nil {
				return fmt.Errorf("KeyCharacterMap: %s[%d] value: %w", tableName, j, err)
			}
		}
	}

	return nil
}
