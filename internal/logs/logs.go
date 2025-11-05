package logs

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
    "strings"
)

func configDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, ".config", "gerrit-migrator"), nil
}

func logFilePath() (string, error) {
    dir, err := configDir()
    if err != nil { return "", err }
    if err := os.MkdirAll(dir, 0o700); err != nil { return "", err }
    return filepath.Join(dir, "migrations.log"), nil
}

// AppendMigrationLog writes a single line with timestamp, source, target, outcome
// outcome should be "passed", "failed" or "trial"
func AppendMigrationLog(source, target, outcome string) error {
    p, err := logFilePath()
    if err != nil { return err }
    f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
    if err != nil { return err }
    defer f.Close()
    ts := time.Now().Format(time.RFC3339)
    line := fmt.Sprintf("%s\tsource=%s\ttarget=%s\toutcome=%s\n", ts, source, target, outcome)
    _, err = f.WriteString(line)
    return err
}

// Detailed per-run log
func runsDir() (string, error) {
    dir, err := configDir()
    if err != nil { return "", err }
    runDir := filepath.Join(dir, "runs")
    if err := os.MkdirAll(runDir, 0o700); err != nil { return "", err }
    return runDir, nil
}

func BeginRunLog() (string, error) {
    rd, err := runsDir()
    if err != nil { return "", err }
    ts := time.Now().Format("20060102T150405Z0700")
    path := filepath.Join(rd, fmt.Sprintf("run-%s.log", ts))
    f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
    if err != nil { return "", err }
    defer f.Close()
    _, _ = f.WriteString(fmt.Sprintf("=== Migration run started %s ===\n", time.Now().Format(time.RFC3339)))
    return path, nil
}

func AppendRunDetail(runPath, line string) {
    if strings.TrimSpace(runPath) == "" { return }
    f, err := os.OpenFile(runPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
    if err != nil { return }
    defer f.Close()
    ts := time.Now().Format(time.RFC3339)
    _, _ = f.WriteString(fmt.Sprintf("%s  %s\n", ts, line))
}

// Global current run path for convenience
var currentRunPath string

func SetCurrentRunPath(p string) { currentRunPath = p }

func AppendRunDetailCurrent(line string) {
    AppendRunDetail(currentRunPath, line)
}


