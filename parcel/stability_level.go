package parcel

// StabilityLevel represents the binder stability level from
// android/binder/Stability.h, written as int32 after each
// flat_binder_object by Parcel::finishFlattenBinder.
type StabilityLevel int32

const (
	// StabilityUndeclared is the default stability for null binder objects.
	StabilityUndeclared = StabilityLevel(0)

	// StabilitySystem is the stability for system (non-VNDK) binder objects.
	// This matches getLocalLevel() in system builds.
	StabilitySystem = StabilityLevel(0b001100) // 12
)
