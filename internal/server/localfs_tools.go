package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
)

// RegisterLocalFSTools registers the list_local_files and read_local_file
// tools on the server. These are convenience tools that give the LLM
// visibility into the allowed local directories. This is a no-op if
// the server has no LocalFS configured.
func RegisterLocalFSTools(s *Server) {
	if s.LocalFS() == nil {
		return
	}
	registerListLocalFiles(s)
	registerReadLocalFile(s)
}

type listLocalFilesInput struct {
	Path string `json:"path,omitempty" jsonschema:"Relative path within an allowed directory. Omit or use '.' to list the root of each allowed directory."`
}

func registerListLocalFiles(srv *Server) {
	dirs := srv.LocalFS().Dirs()
	var sb strings.Builder
	sb.WriteString("List files in an allowed local directory.\n\n")
	sb.WriteString("Allowed directories:\n")
	for _, d := range dirs {
		mode := "read-only"
		if d.Mode == localfs.ModeReadWrite {
			mode = "read-write"
		}
		fmt.Fprintf(&sb, "  - %s (%s)\n", d.Path, mode)
	}
	sb.WriteString("\nPaths are relative to an allowed directory. Omit path or use '.' to list root contents.")

	AddTool(srv, &mcp.Tool{
		Name:        "list_local_files",
		Description: sb.String(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listLocalFilesInput) (*mcp.CallToolResult, any, error) {
		lfs := srv.LocalFS()
		if lfs == nil {
			return nil, nil, fmt.Errorf("local file access is not enabled")
		}

		entries, dir, err := lfs.ListDir(input.Path)
		if err != nil {
			return nil, nil, err
		}

		var out strings.Builder
		displayPath := input.Path
		if displayPath == "" || displayPath == "." {
			displayPath = dir
		} else {
			displayPath = dir + "/" + input.Path
		}
		fmt.Fprintf(&out, "Directory: %s\n\n", displayPath)

		if len(entries) == 0 {
			out.WriteString("(empty directory)")
		} else {
			for _, e := range entries {
				if e.IsDir {
					fmt.Fprintf(&out, "  [dir]  %s/\n", e.Name)
				} else {
					fmt.Fprintf(&out, "  %6d  %s\n", e.Size, e.Name)
				}
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: out.String()},
			},
		}, nil, nil
	})
}

type readLocalFileInput struct {
	Path string `json:"path" jsonschema:"Relative path to a file within an allowed directory"`
}

func registerReadLocalFile(srv *Server) {
	AddTool(srv, &mcp.Tool{
		Name: "read_local_file",
		Description: `Read a file from an allowed local directory.

Returns text content for text files. Binary files are not supported — use save_to on download tools to save binary files to disk instead.
Requires --allow-read-dir or --allow-write-dir. Content is truncated at 512 KB.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input readLocalFileInput) (*mcp.CallToolResult, any, error) {
		lfs := srv.LocalFS()
		if lfs == nil {
			return nil, nil, fmt.Errorf("local file access is not enabled")
		}

		if input.Path == "" {
			return nil, nil, fmt.Errorf("path is required")
		}

		data, dir, err := lfs.ReadFile(input.Path)
		if err != nil {
			return nil, nil, err
		}

		// Check if it looks like binary.
		if !isLikelyText(data) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Binary file (%d bytes) at %s/%s. Use save_to on download tools for binary files.", len(data), dir, input.Path)},
				},
			}, nil, nil
		}

		const maxSize = 512 * 1024 // 512 KB
		truncated := false
		if len(data) > maxSize {
			data = data[:maxSize]
			truncated = true
		}

		text := string(data)
		if truncated {
			text += "\n\n--- truncated at 512 KB ---"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// isLikelyText checks if data appears to be text content.
// Returns false if it contains null bytes or has a low ratio of printable characters.
func isLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	printable := 0
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b == '\n' || b == '\r' || b == '\t' || (b >= 32 && b < 127) {
			printable++
		}
	}
	return float64(printable)/float64(len(data)) > 0.85
}
