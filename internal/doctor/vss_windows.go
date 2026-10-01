//go:build windows

package doctor

import "os/exec"

// checkVSS reports whether the Volume Shadow Copy service is available for
// restic's --use-fs-snapshot to start on demand. VSS is a demand-start
// service: on a healthy machine it is normally STOPPED until something
// needs it, so "not currently running" is not a failure -- only "disabled"
// or "service missing entirely" are.
func checkVSS(r *Report) {
	out, err := exec.Command("sc", "qc", "VSS").CombinedOutput()
	if err != nil {
		r.add("windows-vss", false, "VSS service not found: "+firstLine(string(out), err))
		return
	}
	disabled := contains(string(out), "DISABLED")
	r.add("windows-vss", !disabled, vssDetail(disabled))
}

func vssDetail(disabled bool) string {
	if disabled {
		return "VSS service is present but disabled; restic --use-fs-snapshot will fail"
	}
	return "VSS service available (starts on demand)"
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
