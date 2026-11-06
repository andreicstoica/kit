package liftoff

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const idleAgentLabel = "com.andreicstoica.kit.idle"

func idleAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", idleAgentLabel+".plist"), nil
}

func EnableIdleMonitor(after time.Duration) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic scheduling currently requires macOS; run kit idle --apply from your scheduler")
	}
	if after < 15*time.Minute {
		return fmt.Errorf("idle timeout must be at least 15 minutes")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := idleAgentPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	logs, err := RunDir("idle-monitor")
	if err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	return WithWorkspaceLock("idle-sweep", func() error {
		if _, err = exec.Command("launchctl", "print", domain+"/"+idleAgentLabel).Output(); err == nil {
			if out, err := exec.Command("launchctl", "bootout", domain+"/"+idleAgentLabel).CombinedOutput(); err != nil {
				return fmt.Errorf("unload idle monitor: %w: %s", err, out)
			}
		}
		contents := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>idle</string><string>--apply</string><string>--after</string><string>%s</string></array>
<key>StartInterval</key><integer>300</integer><key>RunAtLoad</key><true/>
<key>ProcessType</key><string>Background</string>
<key>EnvironmentVariables</key><dict><key>PATH</key><string>%s</string><key>KIT_STATE_DIR</key><string>%s</string></dict>
<key>WorkingDirectory</key><string>%s</string>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, idleAgentLabel, html.EscapeString(binary), after, html.EscapeString(os.Getenv("PATH")), html.EscapeString(configDir()), html.EscapeString(configDir()), html.EscapeString(filepath.Join(logs, "idle.log")), html.EscapeString(filepath.Join(logs, "idle.log")))
		if err = os.WriteFile(path, []byte(contents), 0600); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(configDir(), "idle-state.json"), []byte("{}\n"), 0600); err != nil {
			return err
		}
		if out, err := exec.Command("launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
			return fmt.Errorf("load idle monitor: %w: %s", err, out)
		}
		return nil
	})
}

func DisableIdleMonitor() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic scheduling currently requires macOS")
	}
	path, err := idleAgentPath()
	if err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if _, err = exec.Command("launchctl", "print", domain+"/"+idleAgentLabel).Output(); err == nil {
		if out, err := exec.Command("launchctl", "bootout", domain+"/"+idleAgentLabel).CombinedOutput(); err != nil {
			return fmt.Errorf("unload idle monitor: %w: %s", err, out)
		}
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
