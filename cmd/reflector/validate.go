package main

import (
	"fmt"
	"os"

	"github.com/rlnorthcutt/reflector/internal/identity"
	"github.com/rlnorthcutt/reflector/internal/routes"
)

// runValidate implements `reflector validate [dir]`: it loads and
// compiles every route file in dir (default routes.d), reporting every
// problem found rather than stopping at the first, and exits non-zero if
// any were found.
func runValidate(args []string) error {
	dir := "routes.d"
	if len(args) > 0 {
		dir = args[0]
	}

	list, errs := routes.LoadDir(os.DirFS("."), dir)
	if _, err := routes.BuildMux(list, identity.Identity{}); err != nil {
		errs = append(errs, err)
	}

	for _, e := range errs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d error(s) found in %s", len(errs), dir)
	}

	fmt.Printf("OK: %d route(s) in %s\n", len(list), dir)
	return nil
}
