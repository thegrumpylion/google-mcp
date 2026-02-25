// Package localfs provides secure local filesystem access for MCP tools.
// All file access is gated through an FS instance that enforces directory
// allowlists using os.Root (Go 1.25+) for kernel-enforced path containment.
// Symlink escapes and ../../../ traversals are prevented by the OS, not
// by userspace path manipulation.
package localfs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Mode controls the access level for an allowed directory.
type Mode int

const (
	// ModeRead allows reading files from the directory.
	ModeRead Mode = iota
	// ModeReadWrite allows reading and writing files in the directory.
	ModeReadWrite
)

// Dir represents an allowed directory with its access mode.
type Dir struct {
	// Path is the path to the directory (will be resolved to absolute).
	Path string
	// Mode is the access level (read or read-write).
	Mode Mode
}

// openDir holds an os.Root handle along with its mode.
type openDir struct {
	root *os.Root
	mode Mode
	path string // original resolved path, for error messages
}

// FS gates all local file access through a set of allowed directories.
// Each directory is backed by an os.Root which enforces path containment
// at the kernel level. If no directories are configured, all operations
// return errors — local file access is opt-in only.
type FS struct {
	dirs []openDir
}

// New creates a new FS with the given allowed directories.
// Each directory is opened as an os.Root and validated.
// Returns an error if any directory path is invalid or does not exist.
// The caller should call Close when the FS is no longer needed.
func New(dirs []Dir) (*FS, error) {
	opened := make([]openDir, 0, len(dirs))
	for _, d := range dirs {
		abs, err := filepath.Abs(d.Path)
		if err != nil {
			return nil, fmt.Errorf("allowed dir %q: resolving path: %w", d.Path, err)
		}

		root, err := os.OpenRoot(abs)
		if err != nil {
			// Close any already-opened roots before returning.
			for _, o := range opened {
				o.root.Close()
			}
			return nil, fmt.Errorf("allowed dir %q: %w", d.Path, err)
		}
		opened = append(opened, openDir{root: root, mode: d.Mode, path: abs})
	}
	return &FS{dirs: opened}, nil
}

// Close releases all os.Root handles.
func (fs *FS) Close() error {
	var firstErr error
	for _, d := range fs.dirs {
		if err := d.root.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Enabled returns true if any directories are configured.
func (fs *FS) Enabled() bool {
	return len(fs.dirs) > 0
}

// ReadFile reads a file from an allowed directory.
// The path must be relative to one of the configured directories.
// Returns the file contents and the directory it was read from.
func (fs *FS) ReadFile(path string) ([]byte, string, error) {
	if !fs.Enabled() {
		return nil, "", fmt.Errorf("local file access is not enabled (use --allow-read-dir or --allow-write-dir)")
	}
	if path == "" {
		return nil, "", fmt.Errorf("path is required")
	}

	// Try each directory in order.
	var lastErr error
	for _, d := range fs.dirs {
		data, err := d.root.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		return data, d.path, nil
	}

	return nil, "", fmt.Errorf("cannot read %q: %w", path, lastErr)
}

// OpenFile opens a file from an allowed directory for streaming.
// The caller must close the returned ReadCloser.
// Returns the file handle and the directory it was opened from.
func (fs *FS) OpenFile(path string) (io.ReadCloser, string, error) {
	if !fs.Enabled() {
		return nil, "", fmt.Errorf("local file access is not enabled (use --allow-read-dir or --allow-write-dir)")
	}
	if path == "" {
		return nil, "", fmt.Errorf("path is required")
	}

	var lastErr error
	for _, d := range fs.dirs {
		f, err := d.root.Open(path)
		if err != nil {
			lastErr = err
			continue
		}
		return f, d.path, nil
	}

	return nil, "", fmt.Errorf("cannot open %q: %w", path, lastErr)
}

// WriteFile writes data to a file in an allowed read-write directory.
// The path must be relative to one of the configured read-write directories.
// Creates the file if it doesn't exist, truncates if it does.
// Returns the directory it was written to.
func (fs *FS) WriteFile(path string, data []byte) (string, error) {
	if !fs.Enabled() {
		return "", fmt.Errorf("local file access is not enabled (use --allow-read-dir or --allow-write-dir)")
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	var lastErr error
	for _, d := range fs.dirs {
		if d.mode == ModeRead {
			lastErr = fmt.Errorf("directory %s is read-only", d.path)
			continue
		}
		if err := d.root.WriteFile(path, data, 0644); err != nil {
			lastErr = err
			continue
		}
		return d.path, nil
	}

	return "", fmt.Errorf("cannot write %q: %w", path, lastErr)
}

// Stat returns file info from an allowed directory.
func (fs *FS) Stat(path string) (os.FileInfo, string, error) {
	if !fs.Enabled() {
		return nil, "", fmt.Errorf("local file access is not enabled (use --allow-read-dir or --allow-write-dir)")
	}
	if path == "" {
		return nil, "", fmt.Errorf("path is required")
	}

	var lastErr error
	for _, d := range fs.dirs {
		info, err := d.root.Stat(path)
		if err != nil {
			lastErr = err
			continue
		}
		return info, d.path, nil
	}

	return nil, "", fmt.Errorf("cannot stat %q: %w", path, lastErr)
}
