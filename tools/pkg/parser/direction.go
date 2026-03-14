package parser

// Direction indicates the data flow direction for a method parameter.
type Direction int

const (
	DirectionNone  Direction = iota
	DirectionIn              // in
	DirectionOut             // out
	DirectionInOut           // inout
)

// String returns the AIDL keyword for the direction.
func (d Direction) String() string {
	switch d {
	case DirectionIn:
		return "in"
	case DirectionOut:
		return "out"
	case DirectionInOut:
		return "inout"
	default:
		return ""
	}
}
