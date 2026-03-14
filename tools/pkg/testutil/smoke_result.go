package testutil

// SmokeResult summarizes the outcome of SmokeTestAllMethods.
type SmokeResult struct {
	Total    int
	Passed   int
	Panicked int
	Failed   int
}
