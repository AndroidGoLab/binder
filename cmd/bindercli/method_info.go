package main

// MethodInfo describes one method on a binder service interface.
type MethodInfo struct {
	Name       string
	Params     []ParamInfo
	ReturnType string
}
