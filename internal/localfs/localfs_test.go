package localfs

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDirs creates a temporary directory structure for testing:
//
//	tmpdir/
//	  readonly/
//	    file.txt
//	    subdir/
//	      nested.txt
//	  readwrite/
//	    existing.txt
//	  outside/
//	    secret.txt
func setupTestDirs(t *testing.T) (readonlyDir, readwriteDir, outsideDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	readonlyDir = filepath.Join(tmpDir, "readonly")
	readwriteDir = filepath.Join(tmpDir, "readwrite")
	outsideDir = filepath.Join(tmpDir, "outside")

	for _, dir := range []string{
		readonlyDir,
		filepath.Join(readonlyDir, "subdir"),
		readwriteDir,
		outsideDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		filepath.Join(readonlyDir, "file.txt"):             "readonly content",
		filepath.Join(readonlyDir, "subdir", "nested.txt"): "nested content",
		filepath.Join(readwriteDir, "existing.txt"):        "readwrite content",
		filepath.Join(outsideDir, "secret.txt"):            "secret content",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return readonlyDir, readwriteDir, outsideDir
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid directories", func(t *testing.T) {
		fs, err := New([]Dir{
			{Path: tmpDir, Mode: ModeRead},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer fs.Close()
		if !fs.Enabled() {
			t.Fatal("expected Enabled() to be true")
		}
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := New([]Dir{
			{Path: filepath.Join(tmpDir, "nonexistent"), Mode: ModeRead},
		})
		if err == nil {
			t.Fatal("expected error for nonexistent directory")
		}
	})

	t.Run("file not directory", func(t *testing.T) {
		f := filepath.Join(tmpDir, "afile.txt")
		os.WriteFile(f, []byte("x"), 0644)
		_, err := New([]Dir{
			{Path: f, Mode: ModeRead},
		})
		if err == nil {
			t.Fatal("expected error for file path")
		}
	})

	t.Run("empty dirs", func(t *testing.T) {
		fs, err := New(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer fs.Close()
		if fs.Enabled() {
			t.Fatal("expected Enabled() to be false with no dirs")
		}
	})
}

func TestReadFile(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "read from readonly dir",
			path: "file.txt",
			want: "readonly content",
		},
		{
			name: "read nested subdir",
			path: "subdir/nested.txt",
			want: "nested content",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _, err := fs.ReadFile(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("got %q, want %q", string(data), tt.want)
			}
		})
	}
}

func TestReadFileFromReadWriteDir(t *testing.T) {
	_, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	data, _, err := fs.ReadFile("existing.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "readwrite content" {
		t.Errorf("got %q, want %q", string(data), "readwrite content")
	}
}

func TestPathTraversal(t *testing.T) {
	readonlyDir, _, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	// os.Root prevents all of these at the kernel level.
	tests := []struct {
		name string
		path string
	}{
		{
			name: "dot-dot traversal",
			path: "../outside/secret.txt",
		},
		{
			name: "double dot-dot traversal",
			path: "subdir/../../outside/secret.txt",
		},
		{
			name: "absolute path",
			path: "/etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := fs.ReadFile(tt.path)
			if err == nil {
				t.Fatal("expected error for path traversal attempt")
			}
		})
	}
}

func TestSymlinkEscape(t *testing.T) {
	readonlyDir, _, outsideDir := setupTestDirs(t)

	// Create a symlink inside readonly that points outside.
	symPath := filepath.Join(readonlyDir, "escape-link")
	if err := os.Symlink(filepath.Join(outsideDir, "secret.txt"), symPath); err != nil {
		t.Fatal(err)
	}

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	// os.Root should block following the symlink outside the root.
	_, _, err = fs.ReadFile("escape-link")
	if err == nil {
		t.Fatal("expected error for symlink escape")
	}
}

func TestSymlinkWithinDir(t *testing.T) {
	readonlyDir, _, _ := setupTestDirs(t)

	// Create a symlink inside readonly that points to another file within readonly.
	symPath := filepath.Join(readonlyDir, "internal-link")
	if err := os.Symlink("file.txt", symPath); err != nil {
		t.Fatal(err)
	}

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	// Symlink within the same root should work.
	data, _, err := fs.ReadFile("internal-link")
	if err != nil {
		t.Fatalf("expected symlink within dir to work: %v", err)
	}
	if string(data) != "readonly content" {
		t.Errorf("got %q, want %q", string(data), "readonly content")
	}
}

func TestWriteFile(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	t.Run("write to readwrite dir", func(t *testing.T) {
		_, err := fs.WriteFile("newfile.txt", []byte("new content"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify via os directly.
		data, err := os.ReadFile(filepath.Join(readwriteDir, "newfile.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new content" {
			t.Errorf("got %q, want %q", string(data), "new content")
		}
	})

	t.Run("write traversal denied", func(t *testing.T) {
		_, err := fs.WriteFile("../outside/escape.txt", []byte("should fail"))
		if err == nil {
			t.Fatal("expected error writing outside allowed dirs")
		}
	})
}

func TestWriteToReadOnlyDenied(t *testing.T) {
	readonlyDir, _, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	_, err = fs.WriteFile("attempt.txt", []byte("should fail"))
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestDisabledFS(t *testing.T) {
	fs, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	t.Run("read disabled", func(t *testing.T) {
		_, _, err := fs.ReadFile("anything.txt")
		if err == nil {
			t.Fatal("expected error when FS is disabled")
		}
	})

	t.Run("write disabled", func(t *testing.T) {
		_, err := fs.WriteFile("test.txt", []byte("x"))
		if err == nil {
			t.Fatal("expected error when FS is disabled")
		}
	})

	t.Run("open disabled", func(t *testing.T) {
		_, _, err := fs.OpenFile("anything.txt")
		if err == nil {
			t.Fatal("expected error when FS is disabled")
		}
	})

	t.Run("stat disabled", func(t *testing.T) {
		_, _, err := fs.Stat("anything.txt")
		if err == nil {
			t.Fatal("expected error when FS is disabled")
		}
	})
}

func TestOpenFile(t *testing.T) {
	readonlyDir, _, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	rc, _, err := fs.OpenFile("file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 100)
	n, _ := rc.Read(buf)
	if string(buf[:n]) != "readonly content" {
		t.Errorf("got %q, want %q", string(buf[:n]), "readonly content")
	}
}

func TestStat(t *testing.T) {
	readonlyDir, _, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	info, _, err := fs.Stat("file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name() != "file.txt" {
		t.Errorf("got name %q, want %q", info.Name(), "file.txt")
	}
	if info.Size() != int64(len("readonly content")) {
		t.Errorf("got size %d, want %d", info.Size(), len("readonly content"))
	}
}

func TestMultipleDirsFallthrough(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	// "existing.txt" only exists in readwriteDir, not readonlyDir.
	// The FS should try readonlyDir first (fail), then readwriteDir (succeed).
	data, dir, err := fs.ReadFile("existing.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "readwrite content" {
		t.Errorf("got %q, want %q", string(data), "readwrite content")
	}
	if dir != readwriteDir {
		t.Errorf("got dir %q, want %q", dir, readwriteDir)
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	fs, err := New([]Dir{
		{Path: tmpDir, Mode: ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should work before close.
	_, _, err = fs.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("unexpected error before close: %v", err)
	}

	// Close the FS.
	if err := fs.Close(); err != nil {
		t.Fatalf("unexpected error closing: %v", err)
	}

	// Should fail after close.
	_, _, err = fs.ReadFile("test.txt")
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestDirs(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	dirs := fs.Dirs()
	if len(dirs) != 2 {
		t.Fatalf("got %d dirs, want 2", len(dirs))
	}

	if dirs[0].Path != readonlyDir {
		t.Errorf("dirs[0].Path = %q, want %q", dirs[0].Path, readonlyDir)
	}
	if dirs[0].Mode != ModeRead {
		t.Errorf("dirs[0].Mode = %v, want ModeRead", dirs[0].Mode)
	}
	if dirs[1].Path != readwriteDir {
		t.Errorf("dirs[1].Path = %q, want %q", dirs[1].Path, readwriteDir)
	}
	if dirs[1].Mode != ModeReadWrite {
		t.Errorf("dirs[1].Mode = %v, want ModeReadWrite", dirs[1].Mode)
	}
}

func TestListDir(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	t.Run("list root of readonly dir", func(t *testing.T) {
		entries, dir, err := fs.ListDir(".")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir != readonlyDir {
			t.Errorf("got dir %q, want %q", dir, readonlyDir)
		}
		// readonly has: file.txt, subdir
		names := make(map[string]bool)
		for _, e := range entries {
			names[e.Name] = true
		}
		if !names["file.txt"] {
			t.Error("expected file.txt in listing")
		}
		if !names["subdir"] {
			t.Error("expected subdir in listing")
		}
	})

	t.Run("list empty path defaults to root", func(t *testing.T) {
		entries, _, err := fs.ListDir("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) == 0 {
			t.Error("expected non-empty listing")
		}
	})

	t.Run("list subdirectory", func(t *testing.T) {
		entries, dir, err := fs.ListDir("subdir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir != readonlyDir {
			t.Errorf("got dir %q, want %q", dir, readonlyDir)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
		if entries[0].Name != "nested.txt" {
			t.Errorf("got name %q, want %q", entries[0].Name, "nested.txt")
		}
		if entries[0].IsDir {
			t.Error("nested.txt should not be a directory")
		}
	})

	t.Run("entry types", func(t *testing.T) {
		entries, _, err := fs.ListDir(".")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, e := range entries {
			if e.Name == "subdir" && !e.IsDir {
				t.Error("subdir should be a directory")
			}
			if e.Name == "file.txt" && e.IsDir {
				t.Error("file.txt should not be a directory")
			}
			if e.Name == "file.txt" && e.Size != int64(len("readonly content")) {
				t.Errorf("file.txt size = %d, want %d", e.Size, len("readonly content"))
			}
		}
	})

	t.Run("traversal denied", func(t *testing.T) {
		_, _, err := fs.ListDir("../outside")
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		_, _, err := fs.ListDir("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
	})
}

func TestListDirDisabled(t *testing.T) {
	fs, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	_, _, err = fs.ListDir(".")
	if err == nil {
		t.Fatal("expected error when FS is disabled")
	}
}

func TestListDirFallthrough(t *testing.T) {
	readonlyDir, readwriteDir, _ := setupTestDirs(t)

	fs, err := New([]Dir{
		{Path: readonlyDir, Mode: ModeRead},
		{Path: readwriteDir, Mode: ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	// "subdir" only exists in readonlyDir. Should find it there.
	entries, dir, err := fs.ListDir("subdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != readonlyDir {
		t.Errorf("got dir %q, want %q", dir, readonlyDir)
	}
	if len(entries) != 1 || entries[0].Name != "nested.txt" {
		t.Errorf("unexpected entries: %v", entries)
	}
}
