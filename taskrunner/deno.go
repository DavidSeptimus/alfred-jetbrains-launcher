package taskrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// denoDetector reads the `tasks` map from deno.json / deno.jsonc. A task value
// is a string command, or (newer Deno) an object with a "command" field.
type denoDetector struct{}

var denoConfigNames = []string{"deno.json", "deno.jsonc"}

func (denoDetector) Runner() Runner { return RunnerDeno }

func (denoDetector) Available(dir string) bool {
	return anyFileExists(dir, denoConfigNames...)
}

func (denoDetector) Tasks(dir string) ([]Task, error) {
	source := firstExisting(dir, denoConfigNames)
	data, err := os.ReadFile(filepath.Join(dir, source))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Tasks map[string]json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(stripJSONComments(data), &doc); err != nil {
		return nil, err
	}
	runnable := onPath("deno")

	tasks := make([]Task, 0, len(doc.Tasks))
	for name, raw := range doc.Tasks {
		tasks = append(tasks, Task{
			Name:     name,
			Runner:   RunnerDeno,
			Command:  []string{"deno", "task", name},
			Cwd:      dir,
			Source:   source,
			Desc:     denoTaskDesc(raw),
			Runnable: runnable,
		})
	}
	sortByName(tasks)
	return tasks, nil
}

// denoTaskDesc extracts the command from a string task or an object task's
// "command" field.
func denoTaskDesc(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Command
	}
	return ""
}

// stripJSONComments removes // line and /* */ block comments so a .jsonc file
// parses with encoding/json. It is string-aware: comment markers inside a quoted
// string (e.g. a "https://…" task command, or a "/* */" literal) are left
// untouched, so it is safe to run on a plain deno.json too.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inStr, escaped := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inStr {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/': // line comment — skip to (but keep) the newline
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					out = append(out, data[i])
				}
				continue
			case '*': // block comment — skip through the closing */
				i += 2
				for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
					i++
				}
				i++ // the loop's i++ then steps past the closing '/'
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
