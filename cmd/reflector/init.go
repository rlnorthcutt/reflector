package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	presets "github.com/rlnorthcutt/reflector/examples"
)

// runInit implements `reflector init [preset...]`: it extracts named
// preset packs (or all of them, if none are named) from the embedded
// examples filesystem to ./routes.d and ./payloads as plain editable
// files. Existing files are never overwritten unless --force is passed.
func runInit(args []string) error {
	fset := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fset.Bool("force", false, "overwrite existing files")
	if err := fset.Parse(args); err != nil {
		return err
	}

	names := fset.Args()
	if len(names) == 0 {
		names = presets.Names
	}
	for _, name := range names {
		if !isKnownPreset(name) {
			return fmt.Errorf("unknown preset %q (available: %s)", name, strings.Join(presets.Names, ", "))
		}
	}

	for _, name := range names {
		sub, err := fs.Sub(presets.FS, name)
		if err != nil {
			return fmt.Errorf("preset %q: %w", name, err)
		}
		if err := extractPreset(sub, *force); err != nil {
			return fmt.Errorf("preset %q: %w", name, err)
		}
		fmt.Printf("extracted preset %q\n", name)
	}
	return nil
}

func extractPreset(sub fs.FS, force bool) error {
	return fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}

		dest := filepath.FromSlash(path)
		if !force {
			if _, err := os.Stat(dest); err == nil {
				fmt.Printf("skip %s (already exists; use --force to overwrite)\n", dest)
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}
