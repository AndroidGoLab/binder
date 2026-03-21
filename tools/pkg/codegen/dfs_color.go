package codegen

// dfsColor represents the state of a node during depth-first search.
type dfsColor int

const (
	white dfsColor = iota // not visited
	gray                  // in progress (on the DFS stack)
	black                 // finished
)
