package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (e *Env) StateDocument() map[string]any {
	e.T.Helper()
	contents, err := os.ReadFile(filepath.Join(e.StateDir, "state.json"))
	if err != nil {
		e.T.Fatal(err)
	}
	return e.decodeJSON("state.json", contents)
}

func (e *Env) StatusJSON() map[string]any {
	e.T.Helper()
	result := e.Parachute("status", "--json").Run(2 * time.Minute)
	if result.Code != 0 {
		e.T.Fatalf("parachute status --json exited %d\n%s", result.Code, result.Stderr)
	}
	return e.decodeJSON("status", []byte(result.Stdout))
}

func (e *Env) Checksum(path string) string {
	e.T.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		e.T.Fatal(err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func (e *Env) decodeJSON(what string, contents []byte) map[string]any {
	e.T.Helper()
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		e.T.Fatalf("%s is not JSON: %v\n%s", what, err, contents)
	}
	return document
}
