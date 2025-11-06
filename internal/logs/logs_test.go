package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempHome(t *testing.T) string {
	d := t.TempDir()
	t.Setenv("HOME", d)
	return d
}

func TestAppendMigrationLog_WritesLine(t *testing.T) {
	withTempHome(t)
	if err := AppendMigrationLog("src", "tgt", "passed"); err != nil {
		t.Fatalf("AppendMigrationLog: %v", err)
	}
	p, err := logFilePath()
	if err != nil { t.Fatalf("logFilePath: %v", err) }
	b, err := os.ReadFile(p)
	if err != nil { t.Fatalf("read log: %v", err) }
	line := string(b)
	if !strings.Contains(line, "source=src") || !strings.Contains(line, "target=tgt") || !strings.Contains(line, "outcome=passed") {
		t.Fatalf("unexpected log line: %s", line)
	}
}

func TestBeginRunLog_AndAppendRunDetail(t *testing.T) {
	withTempHome(t)
	path, err := BeginRunLog()
	if err != nil { t.Fatalf("BeginRunLog: %v", err) }
	if !strings.Contains(filepath.Base(path), "run-") {
		t.Fatalf("run path not named run-*: %s", path)
	}
	AppendRunDetail(path, "hello")
	b, err := os.ReadFile(path)
	if err != nil { t.Fatalf("read run: %v", err) }
	if !strings.Contains(string(b), "hello") {
		t.Fatalf("expected to find 'hello' in run log, got: %s", string(b))
	}
}

func TestAppendRunDetailCurrent_UsesGlobalPath(t *testing.T) {
	withTempHome(t)
	path, err := BeginRunLog()
	if err != nil { t.Fatalf("BeginRunLog: %v", err) }
	SetCurrentRunPath(path)
	AppendRunDetailCurrent("line-1")
	b, err := os.ReadFile(path)
	if err != nil { t.Fatalf("read run: %v", err) }
	if !strings.Contains(string(b), "line-1") {
		t.Fatalf("expected to find 'line-1' in run log")
	}
}
