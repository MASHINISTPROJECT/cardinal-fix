//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cardinal/internal/log"
	"cardinal/internal/overlayutil"
)

type StorageDriver string

const (
	DriverNative StorageDriver = "native"
	DriverFuse   StorageDriver = "fuse"
	DriverHelper StorageDriver = "helper"
	DriverVFS    StorageDriver = "vfs"
)

func driverMarkerPath(merged string) string {
	return filepath.Join(filepath.Dir(merged), "driver")
}

func writeDriverMarker(merged string, d StorageDriver) {
	_ = os.WriteFile(driverMarkerPath(merged), []byte(string(d)+"\n"), 0600)
}

func readDriverMarker(merged string) StorageDriver {
	data, err := os.ReadFile(driverMarkerPath(merged))
	if err != nil {
		return ""
	}
	return StorageDriver(strings.TrimSpace(string(data)))
}

func DetectStorageDriver() (StorageDriver, string) {
	if !IsRootless() {
		return DriverNative, "running as root, kernel overlayfs"
	}
	if _, err := exec.LookPath("fuse-overlayfs"); err == nil {
		return DriverFuse, "fuse-overlayfs available"
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		if _, err := exec.LookPath("cp"); err == nil {
			return DriverHelper, "fuse-overlayfs missing, passwordless sudo helper may work, vfs copy available"
		}
		return DriverHelper, "fuse-overlayfs missing, try sudo helper"
	}
	return DriverVFS, "fuse-overlayfs missing, falling back to vfs copy (no CoW)"
}

// MountOverlayWithFallback tries kernel overlay first, then fuse-overlayfs,
// then the privileged helper (E), then a rootless vfs copy (D).
// It never requires new Go dependencies and always returns
// a human-actionable error aggregating every attempt.
func MountOverlayWithFallback(lower, upper, work, merged string) (StorageDriver, error) {
	for _, d := range []string{upper, work, merged} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	if driver := readDriverMarker(merged); driver == DriverVFS {
		if nonEmptyDir(merged) {
			return DriverVFS, nil
		}
	}
	var attempts []string
	if err := overlayutil.MountOverlay(lower, upper, work, merged); err == nil {
		if IsRootless() {
			writeDriverMarker(merged, DriverNative)
			return DriverNative, nil
		}
		writeDriverMarker(merged, DriverNative)
		return DriverNative, nil
	} else {
		attempts = append(attempts, fmt.Sprintf("native overlay mount: %v", err))
	}
	if _, err := exec.LookPath("fuse-overlayfs"); err == nil {
		if err := MountRootlessOverlay(lower, upper, work, merged); err == nil {
			writeDriverMarker(merged, DriverFuse)
			return DriverFuse, nil
		} else {
			attempts = append(attempts, fmt.Sprintf("fuse-overlayfs: %v", err))
		}
	} else {
		attempts = append(attempts, "fuse-overlayfs: not found in $PATH (apt install fuse-overlayfs)")
	}
	if IsRootless() {
		if err := MountViaHelper(lower, upper, work, merged); err == nil {
			writeDriverMarker(merged, DriverHelper)
			return DriverHelper, nil
		} else {
			attempts = append(attempts, fmt.Sprintf("helper (sudo cardinal helper-mount): %v", err))
		}
	}
	if err := mountVFS(lower, merged); err == nil {
		log.Warn("using vfs copy driver for %s (no CoW, higher disk usage). Install fuse-overlayfs for overlayfs.", merged)
		writeDriverMarker(merged, DriverVFS)
		return DriverVFS, nil
	} else {
		// Leave merged empty so the next `cardinal start` retries a clean
		// copy instead of resuming a half-copied rootfs.
		_ = os.RemoveAll(merged)
		_ = os.MkdirAll(merged, 0755)
		attempts = append(attempts, fmt.Sprintf("vfs copy: %v", err))
	}
	hint := "run `cardinal doctor`, install with `sudo apt install -y fuse-overlayfs slirp4netns uidmap`, or run rootful with `sudo cardinal run`"
	if IsRootless() {
		hint += "; vfs fallback also failed, see attempts"
	}
	return "", fmt.Errorf("overlay: no storage driver succeeded:\n  - %s\n%s", strings.Join(attempts, "\n  - "), hint)
}

func UnmountOverlayWithFallback(merged string) {
	if readDriverMarker(merged) == DriverVFS {
		return
	}
	overlayutil.UnmountOverlay(merged)
}

func nonEmptyDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(2)
	if err != nil {
		return len(names) > 0
	}
	return len(names) > 0
}

func mountVFS(lower, merged string) error {
	if nonEmptyDir(merged) {
		return nil
	}
	if _, err := exec.LookPath("cp"); err != nil {
		return fmt.Errorf("cp not found in $PATH, cannot do vfs copy")
	}
	lowerDot := strings.TrimSuffix(lower, "/") + "/."
	out, err := exec.Command("cp", "-a", lowerDot, merged).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp -a %s -> %s: %s: %w", lower, merged, strings.TrimSpace(string(out)), err)
	}
	return nil
}
