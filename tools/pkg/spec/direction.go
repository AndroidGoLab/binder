package spec

// Direction indicates parameter directionality in AIDL.
type Direction string

const (
	DirectionIn    Direction = "in"
	DirectionOut   Direction = "out"
	DirectionInOut Direction = "inout"
)
