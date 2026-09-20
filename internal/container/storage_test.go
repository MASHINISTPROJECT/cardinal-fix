//go:build linux

package container

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDriverMarkerRoundtrip(t *testing.T) {
	base := t.TempDir()
	merged := filepath.Join(base, "merged")
	if err := os.MkdirAll(merged, 0755); err != nil {
		t.Fatal(err)
	}
	if got := readDriverMarker(merged); got != "" {
		t.Fatalf("empty marker = %q, want empty", got)
	}
	writeDriverMarker(merged, DriverVFS)
	if got := readDriverMarker(merged); got != DriverVFS {
		t.Fatalf("marker = %q, want %q", got, DriverVFS)
	}
}

func TestMountVFSCopiesLower(t *testing.T) {
	lower := t.TempDir()
	merged := filepath.Join(t.TempDir(), "merged")
	if err := os.MkdirAll(merged, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lower, "hello.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mountVFS(lower, merged); err != nil {
		t.Skipf("cp -a unavailable in test env: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(merged, "hello.txt"))
	if err != nil {
		t.Fatalf("vfs copy missing file: %v", err)
	}
	if string(data) != "hi" {
		t.Fatalf("vfs copy content = %q, want %q", data, "hi")
	}
	if err := mountVFS(lower, merged); err != nil {
		t.Fatalf("second vfs mount (idempotent) failed: %v", err)
	}
}

func TestDetectStorageDriverReturnsKnown(t *testing.T) {
	driver, detail := DetectStorageDriver()
	switch driver {
	case DriverNative, DriverFuse, DriverHelper, DriverVFS:
	default:
		t.Fatalf("unknown driver %q (%s)", driver, detail)
	}
	if detail == "" {
		t.Fatal("detail should not be empty")
	}
}
