package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// renderEmbedded walks an embedded template tree rooted at root and produces
// artifacts under outPrefix. Files ending in ".tmpl" are executed against data
// and emitted with the suffix stripped; all other files are copied verbatim.
func renderEmbedded(fsys fs.FS, root, outPrefix string, data any) ([]Artifact, error) {
	var arts []Artifact
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}

		rel := strings.TrimPrefix(p, root+"/")
		out := path.Join(outPrefix, rel)

		mode := fs.FileMode(0o644)
		if strings.HasSuffix(rel, ".tmpl") {
			out = strings.TrimSuffix(out, ".tmpl")
			rendered, err := renderTemplate(rel, string(raw), data)
			if err != nil {
				return err
			}
			raw = rendered
		}
		if strings.HasSuffix(out, ".sh") {
			mode = 0o755
		}

		arts = append(arts, Artifact{Path: out, Mode: mode, Content: raw})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return arts, nil
}

// renderTemplate executes one embedded template against data.
func renderTemplate(name, body string, data any) ([]byte, error) {
	t := mustParse(name, body)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}
