package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file is found whatever the case of its name, and a missing one isn't, with and without the
// case-insensitive directory search (which Windows doesn't need)
func TestFileExistCase(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	if err := os.MkdirAll(dir+"/Chars/KFM", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/Chars/KFM/Common.CNS", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	old := caseSensitiveFiles
	t.Cleanup(func() { caseSensitiveFiles = old })
	for _, search := range []bool{false, true} {
		if !search && old {
			continue // A case-sensitive system needs the search
		}
		caseSensitiveFiles = search
		got := FileExist(dir + "/chars/kfm/common.cns")
		if !strings.EqualFold(got, dir+"/Chars/KFM/Common.CNS") {
			t.Errorf("search %v: other case: %q", search, got)
		}
		if got := FileExist(dir + "/chars/kfm/missing.cns"); got != "" {
			t.Errorf("search %v: missing file found: %q", search, got)
		}
		if got := FileExist(dir + "/chars/kfm"); got != "" {
			t.Errorf("search %v: a directory counts as a file: %q", search, got)
		}
	}
}

// A file that isn't there, deep in folders: what SearchFile asks for most while loading characters
func BenchmarkFileExistMissing(b *testing.B) {
	dir := filepath.ToSlash(b.TempDir()) + "/data/brawler/levels/sub"
	if err := os.MkdirAll(dir, 0755); err != nil {
		b.Fatal(err)
	}
	old := caseSensitiveFiles
	defer func() { caseSensitiveFiles = old }()
	for _, search := range []bool{true, false} {
		caseSensitiveFiles = search
		name := "no search"
		if search {
			name = "case search"
		}
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FileExist(dir + "/common.cmd")
			}
		})
	}
}
