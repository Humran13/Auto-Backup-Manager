//go:build windows

package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const taskName = `\Auto-Backup-Manager\HourlyBackup`
const maintainTaskName = `\Auto-Backup-Manager\DailyMaintain`

// taskXML is a full Task Scheduler task definition rather than a bare
// `schtasks /Create` flag set, because only the XML form exposes every
// setting the spec requires together: hourly repetition, StartWhenAvailable
// (catch up a backup missed while the PC was off), MultipleInstancesPolicy
// IgnoreNew (never run two overlapping backups), running as SYSTEM so it
// works with nobody logged in, and RunLevel HighestAvailable for VSS access.
const taskXMLTemplate = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>%s</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <StartBoundary>2024-01-01T00:00:00</StartBoundary>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>%s</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT6H</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`

// Install registers the hourly backup task and daily maintenance task in
// Task Scheduler, running as SYSTEM (S-1-5-18) so no user session is needed.
func Install(abmExePath string) error {
	if err := installTask(taskName, "Auto-Backup-Manager hourly backup run", "PT1H", abmExePath, "backup now --all"); err != nil {
		return err
	}
	return installTask(maintainTaskName, "Auto-Backup-Manager daily retention/prune", "P1D", abmExePath, "maintain --all --prune")
}

func installTask(name, description, interval, exePath, args string) error {
	xml := fmt.Sprintf(taskXMLTemplate, description, interval, exePath, args)

	tmpFile, err := os.CreateTemp("", "abm-task-*.xml")
	if err != nil {
		return fmt.Errorf("creating task definition: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	// schtasks /XML expects UTF-16LE with BOM.
	utf16 := toUTF16LE(xml)
	if _, err := tmpFile.Write(utf16); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	cmd := exec.Command("schtasks", "/Create", "/TN", name, "/XML", tmpFile.Name(), "/F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks /Create %s: %w: %s", name, err, out)
	}
	return nil
}

func toUTF16LE(s string) []byte {
	// BOM + naive UTF-16LE encoding (ASCII-range content only, which is all
	// this template ever contains).
	out := []byte{0xFF, 0xFE}
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

// Uninstall removes both scheduled tasks. It never touches backup
// repositories or configuration.
func Uninstall() error {
	_ = exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()
	_ = exec.Command("schtasks", "/Delete", "/TN", maintainTaskName, "/F").Run()
	return nil
}

// Status returns the current task state as reported by Task Scheduler.
func Status() (string, error) {
	out, err := exec.Command("schtasks", "/Query", "/TN", taskName, "/V", "/FO", "LIST").CombinedOutput()
	return string(out), err
}

func defaultExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join(`C:\Program Files\Auto-Backup-Manager`, "abm.exe")
	}
	return exe
}
