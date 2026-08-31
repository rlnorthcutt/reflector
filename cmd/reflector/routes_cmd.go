package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/rlnorthcutt/reflector/internal/config"
	"github.com/rlnorthcutt/reflector/internal/httpserver"
)

// runRoutesCmd implements `reflector routes`: it prints the resolved
// user/preset route table plus the built-in route list, so a user can see
// exactly what a `reflector serve` with the same flags would expose.
func runRoutesCmd(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}

	rs := loadRoutes(cfg.RoutesDir, cfg.Preset)
	for _, e := range rs.Errors {
		fmt.Fprintln(os.Stderr, "reflector: route error:", e)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)

	fmt.Fprintln(tw, "METHOD\tPATH\tSTATUS\tSOURCE")
	for _, r := range rs.Resolved {
		method := r.Method
		if method == "" {
			method = "ANY"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", method, r.Path, r.Status, r.Source)
	}
	_ = tw.Flush()

	strictNote := ""
	if cfg.StrictBuiltins {
		strictNote = " (--strict-builtins: built-ins always win instead)"
	}
	fmt.Printf("\nBuilt-in routes, shadowed by any matching route above%s:\n", strictNote)
	tw2 := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw2, "METHOD\tPATH\tSOURCE")
	for _, p := range httpserver.BuiltinPatterns() {
		fmt.Fprintf(tw2, "ANY\t%s\tbuiltin\n", p)
	}
	_ = tw2.Flush()

	if len(rs.Errors) > 0 {
		return fmt.Errorf("%d route error(s)", len(rs.Errors))
	}
	return nil
}
