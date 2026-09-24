package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeCompose(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadComposeYAML(t *testing.T, yaml string) *Config {
	t.Helper()
	cfg, err := LoadCompose(writeCompose(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadComposeEnvFileMultiple(t *testing.T) {
	path := writeCompose(t, `
services:
  app:
    image: alpine:latest
    env_file:
      - one.env
      - two.env
`)
	cfg, err := LoadCompose(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one.env", "two.env"}
	if got := cfg.Container["app"].EnvFile; !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("EnvFile = %v, want %v", got, want)
	}
}

func TestLoadComposeEnvFileSingle(t *testing.T) {
	path := writeCompose(t, `
services:
  app:
    image: alpine:latest
    env_file: just.env
`)
	cfg, err := LoadCompose(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"just.env"}
	if got := cfg.Container["app"].EnvFile; !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("EnvFile = %v, want %v", got, want)
	}
}

func TestLoadComposeEnvInheritedFromHost(t *testing.T) {
	t.Setenv("CARDINAL_TEST_INHERIT", "from-host")

	tests := []struct {
		name string
		yaml string
		want map[string]string
	}{
		{
			name: "list entry without value",
			yaml: `
services:
  app:
    image: alpine:latest
    environment:
      - PLAIN=value
      - CARDINAL_TEST_INHERIT
`,
			want: map[string]string{"PLAIN": "value", "CARDINAL_TEST_INHERIT": "from-host"},
		},
		{
			name: "map entry with null value",
			yaml: `
services:
  app:
    image: alpine:latest
    environment:
      CARDINAL_TEST_INHERIT:
`,
			want: map[string]string{"CARDINAL_TEST_INHERIT": "from-host"},
		},
		{
			name: "missing host var is skipped",
			yaml: `
services:
  app:
    image: alpine:latest
    environment:
      CARDINAL_TEST_DEFINITELY_UNSET_VAR
`,
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadComposeYAML(t, tt.yaml)
			if got := cfg.Container["app"].Env; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Env = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadComposeVolumesLongSyntax(t *testing.T) {
	path := writeCompose(t, `
services:
  app:
    image: alpine:latest
    volumes:
      - type: bind
        source: /host/data
        target: /container/data
        read_only: true
      - type: tmpfs
        target: /scratch
        tmpfs:
          size: 64m
`)
	cfg, err := LoadCompose(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/host/data:/container/data:ro", "tmpfs:/scratch:size=64m"}
	if got := cfg.Container["app"].Volumes; !reflect.DeepEqual(got, want) {
		t.Fatalf("Volumes = %v, want %v", got, want)
	}
}

func TestLoadComposePorts(t *testing.T) {
	cfg := loadComposeYAML(t, `
services:
  app:
    image: alpine:latest
    ports:
      - "8080:80"
      - "5353:53/udp"
      - "9000"
      - "8000-8002:80"
`)
	want := []string{"8080:80", "5353:53/udp", "0:9000", "8000-8002:80"}
	if got := cfg.Container["app"].Ports; !reflect.DeepEqual(got, want) {
		t.Fatalf("Ports = %v, want %v", got, want)
	}
}

func TestLoadComposeEnvFileInterpolation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV_FILE_NAME=prod.env\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(`
services:
  app:
    image: alpine:latest
    env_file:
      - ${ENV_FILE_NAME:-.env}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCompose(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"prod.env"}
	if got := cfg.Container["app"].EnvFile; !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("EnvFile = %v, want %v", got, want)
	}
}

func TestStringListUnmarshalTOML(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		var sl StringList
		if err := sl.UnmarshalTOML("single.env"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual([]string(sl), []string{"single.env"}) {
			t.Fatalf("StringList = %v", sl)
		}
	})
	t.Run("list", func(t *testing.T) {
		var sl StringList
		if err := sl.UnmarshalTOML([]interface{}{"a.env", "b.env"}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual([]string(sl), []string{"a.env", "b.env"}) {
			t.Fatalf("StringList = %v", sl)
		}
	})
	t.Run("invalid type", func(t *testing.T) {
		var sl StringList
		if err := sl.UnmarshalTOML(42); err == nil {
			t.Fatal("expected error for non-string TOML value")
		}
	})
}

func TestLoadTomlEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cardinal.toml")
	if err := os.WriteFile(path, []byte(`
[container.app]
image = "alpine:latest"
env_file = ["a.env", "b.env"]
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.env", "b.env"}
	if got := cfg.Container["app"].EnvFile; !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("EnvFile = %v, want %v", got, want)
	}
}

func TestLoadTomlEnvFileScalar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cardinal.toml")
	if err := os.WriteFile(path, []byte(`
[container.app]
image = "alpine:latest"
env_file = "single.env"
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"single.env"}
	if got := cfg.Container["app"].EnvFile; !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("EnvFile = %v, want %v", got, want)
	}
}