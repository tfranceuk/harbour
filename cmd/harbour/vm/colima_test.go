package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func writeColimaMounts(t *testing.T, config string) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	profileDir := filepath.Join(homeDir, ".harbour", "colima", "harbour")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	configPath := filepath.Join(profileDir, "colima.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
}

func TestColimaHasExactMountsMatchesSingleWritableMount(t *testing.T) {
	writeColimaMounts(t, "mounts:\n  - location: /workspace\n    writable: true\n")

	ok, err := Colima{cfg: Config{Profile: "harbour"}}.HasExactMounts([]Mount{
		{Location: "/workspace", Writable: true},
	})
	if err != nil {
		t.Fatalf("HasExactMounts() returned error: %v", err)
	}
	if !ok {
		t.Fatal("HasExactMounts() = false, want true")
	}
}

func TestColimaHasExactMountsMatchesWorkspacePlusReadOnlyExtra(t *testing.T) {
	// Colima may list mounts in any order; the extra mount is read-only.
	writeColimaMounts(t, ""+
		"mounts:\n"+
		"  - location: /Users/me/dotscripts\n"+
		"    writable: false\n"+
		"  - location: /workspace\n"+
		"    writable: true\n")

	ok, err := Colima{cfg: Config{Profile: "harbour"}}.HasExactMounts([]Mount{
		{Location: "/workspace", Writable: true},
		{Location: "/Users/me/dotscripts", Writable: false},
	})
	if err != nil {
		t.Fatalf("HasExactMounts() returned error: %v", err)
	}
	if !ok {
		t.Fatal("HasExactMounts() = false, want true")
	}
}

func TestColimaHasExactMountsRejectsMissingExtraMount(t *testing.T) {
	// Only the workspace is mounted, but an extra mount is expected.
	writeColimaMounts(t, "mounts:\n  - location: /workspace\n    writable: true\n")

	ok, err := Colima{cfg: Config{Profile: "harbour"}}.HasExactMounts([]Mount{
		{Location: "/workspace", Writable: true},
		{Location: "/Users/me/dotscripts", Writable: false},
	})
	if err != nil {
		t.Fatalf("HasExactMounts() returned error: %v", err)
	}
	if ok {
		t.Fatal("HasExactMounts() = true, want false")
	}
}

func TestColimaHasExactMountsRejectsWrongMode(t *testing.T) {
	// The extra mount is expected read-only but is mounted writable.
	writeColimaMounts(t, ""+
		"mounts:\n"+
		"  - location: /workspace\n"+
		"    writable: true\n"+
		"  - location: /Users/me/dotscripts\n"+
		"    writable: true\n")

	ok, err := Colima{cfg: Config{Profile: "harbour"}}.HasExactMounts([]Mount{
		{Location: "/workspace", Writable: true},
		{Location: "/Users/me/dotscripts", Writable: false},
	})
	if err != nil {
		t.Fatalf("HasExactMounts() returned error: %v", err)
	}
	if ok {
		t.Fatal("HasExactMounts() = true, want false")
	}
}
