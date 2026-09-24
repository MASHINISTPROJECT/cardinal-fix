package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInterpolate(t *testing.T) {
	lookup := func(key string) (string, bool) {
		switch key {
		case "NAME":
			return "world", true
		case "EMPTY":
			return "", true
		}
		return "", false
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "hello", want: "hello"},
		{name: "dollar variable", in: "hi $NAME", want: "hi world"},
		{name: "braced variable", in: "hi ${NAME}", want: "hi world"},
		{name: "unset becomes empty", in: "x${NOPE}y", want: "xy"},
		{name: "default when unset", in: "${NOPE:-fallback}", want: "fallback"},
		{name: "default when empty", in: "${EMPTY:-fallback}", want: "fallback"},
		{name: "plain default when set", in: "${NAME:-fallback}", want: "world"},
		{name: "dash keeps empty value", in: "${EMPTY-fallback}", want: ""},
		{name: "dash default when unset", in: "${NOPE-fallback}", want: "fallback"},
		{name: "error form unsets stay literal", in: "${NOPE:?boom}", want: "${NOPE:?boom}"},
		{name: "bang form keeps empty value", in: "${EMPTY?boom}", want: ""},
		{name: "error form set returns value", in: "${NAME:?boom}", want: "world"},
		{name: "escaped dollar", in: "cost $$5", want: "cost $5"},
		{name: "embedded in word", in: "pre${NOPE:-x}post", want: "prexpost"},
		{name: "trailing part", in: "$NAME.log", want: "world.log"},
		{name: "unterminated braced stays", in: "a${NAME", want: "a${NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpolate(tt.in, lookup); got != tt.want {
				t.Fatalf("interpolate(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoadComposeInterpolation(t *testing.T) {
	t.Setenv("CARDINAL_INTERP_HOST", "from-host")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_PASSWORD=secret-db\nCARDINAL_INTERP_DOTENV=from-dotenv\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(`
services:
  app:
    image: "${REGISTRY:-docker.io}/alpine:${TAG:-latest}"
    environment:
      DB_PASSWORD: ${DB_PASSWORD:-nope}
      CARDINAL_INTERP_DOTENV:
    ports:
      - "${WEB_PORT:-8080}:80"
    command: echo ${CARDINAL_INTERP_HOST}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCompose(path)
	if err != nil {
		t.Fatal(err)
	}
	app := cfg.Container["app"]

	if app.Image != "docker.io/alpine:latest" {
		t.Errorf("Image = %q, want docker.io/alpine:latest", app.Image)
	}
	wantEnv := map[string]string{
		"DB_PASSWORD":            "secret-db",
		"CARDINAL_INTERP_DOTENV": "from-dotenv",
	}
	if !reflect.DeepEqual(app.Env, wantEnv) {
		t.Errorf("Env = %v, want %v", app.Env, wantEnv)
	}
	wantPorts := []string{"8080:80"}
	if !reflect.DeepEqual(app.Ports, wantPorts) {
		t.Errorf("Ports = %v, want %v", app.Ports, wantPorts)
	}
	if app.Command != "echo from-host" {
		t.Errorf("Command = %q, want %q", app.Command, "echo from-host")
	}
}

func TestLoadComposeDotEnvMissingEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(`
services:
  app:
    image: alpine:latest
`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCompose(path); err != nil {
		t.Fatal(err)
	}
}
