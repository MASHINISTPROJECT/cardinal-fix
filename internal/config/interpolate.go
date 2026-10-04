package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func loadDotEnv(lookup map[string]string, dir string) {
	if dir == "" {
		return
	}
	path := filepath.Join(dir, ".env")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			if _, ok := lookup[parts[0]]; !ok {
				lookup[parts[0]] = strings.Trim(parts[1], `"'`)
			}
		}
	}
}

func composeLookup(dir string) func(string) (string, bool) {
	dotEnv := make(map[string]string)
	loadDotEnv(dotEnv, dir)
	return func(key string) (string, bool) {
		if v, ok := os.LookupEnv(key); ok {
			return v, true
		}
		v, ok := dotEnv[key]
		return v, ok
	}
}

// interpolate applies compose variable substitution: $VAR, ${VAR},
// ${VAR:-default}, ${VAR-default}, ${VAR:?error}, ${VAR?error} and $$ for a
// literal dollar sign.
func interpolate(s string, lookup func(string) (string, bool)) string {
	if !strings.Contains(s, "$") {
		return s
	}

	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '$' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		switch s[i+1] {
		case '$':
			b.WriteByte('$')
			i += 2
		case '{':
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			body := s[i+2 : i+2+end]
			b.WriteString(interpolateExpr(body, lookup))
			i += 2 + end + 1
		default:
			j := i + 1
			for j < len(s) && (isInterpNameChar(s[j])) {
				j++
			}
			name := s[i+1 : j]
			if name == "" {
				b.WriteByte('$')
				i++
				continue
			}
			if v, ok := lookup(name); ok {
				b.WriteString(v)
			}
			i = j
		}
	}
	return b.String()
}

func isInterpNameChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// interpolateExpr handles the part between ${ and }.
func interpolateExpr(body string, lookup func(string) (string, bool)) string {
	name := body
	op := ""
	def := ""
	for _, cut := range []string{":-", "-", ":?", "?", ":=", "="} {
		if idx := strings.Index(body, cut); idx > 0 {
			name = body[:idx]
			op = cut
			def = body[idx+len(cut):]
			break
		}
	}

	val, exists := lookup(name)
	switch op {
	case ":?":
		if !exists || val == "" {
			return "${" + body + "}"
		}
		return val
	case "?":
		if !exists {
			return "${" + body + "}"
		}
		return val
	case ":=", "=":
		return val
	default:
		if op == ":-" {
			if !exists || val == "" {
				return def
			}
			return val
		}
		if op == "-" {
			if !exists {
				return def
			}
			return val
		}
		if val != "" {
			return val
		}
		return ""
	}
}
