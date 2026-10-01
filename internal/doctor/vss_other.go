//go:build !windows

package doctor

// checkVSS is a no-op on non-Windows platforms; VSS is Windows-only.
func checkVSS(r *Report) {}
