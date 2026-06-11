package render

import (
	"sort"
	"strconv"
	"strings"
	"text/template"
)

// KV is a key/value pair used to feed maps to templates in a stable order.
type KV struct {
	Key   string
	Value string
}

// sortedKV flattens a string map into key-sorted pairs for deterministic
// template iteration.
func sortedKV(m map[string]string) []KV {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]KV, 0, len(keys))
	for _, k := range keys {
		out = append(out, KV{Key: k, Value: m[k]})
	}
	return out
}

// funcMap holds the template helpers shared by every target. The helpers are
// intentionally small and deterministic.
var funcMap = template.FuncMap{
	// quote renders a YAML/HCL-safe double-quoted string.
	"quote": func(s string) string { return strconv.Quote(s) },
	// indent prefixes every line with n spaces (matches Helm's indent).
	"indent": func(n int, s string) string {
		pad := strings.Repeat(" ", n)
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			if l != "" {
				lines[i] = pad + l
			}
		}
		return strings.Join(lines, "\n")
	},
	// upper uppercases a string (used for env/identifier derivations).
	"upper": strings.ToUpper,
}

// mustParse compiles a named template with the shared func map, panicking on
// error. Templates are compile-time constants embedded in the binary, so a
// parse failure is always a programming error.
func mustParse(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(funcMap).Parse(body))
}
