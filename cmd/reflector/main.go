// Command reflector is a single-binary HTTP echo server for load-balancer
// demos and testing. See PLAN.md for the full design; run with --help for
// the flag reference.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "reflector:", err)
		os.Exit(1)
	}
}

// dispatch routes to a subcommand when args[0] names one; otherwise (no
// args, or args[0] looks like a flag) it starts the server, preserving
// v0.1's plain `reflector [flags]` invocation.
func dispatch(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "validate":
			return runValidate(args[1:])
		case "routes":
			return runRoutesCmd(args[1:])
		case "init":
			return runInit(args[1:])
		case "-h", "--help", "help":
			printUsage()
			return nil
		}
	}
	return runServe(args)
}

func printUsage() {
	fmt.Println(`reflector - identity-aware HTTP echo server for load-balancer demos

Usage:
  reflector [flags]              start the server
  reflector validate [dir]       validate route files in dir (default routes.d)
  reflector routes [flags]       print the resolved route table
  reflector init [preset...]     extract preset route packs to routes.d/ and payloads/

License: MIT, see LICENSE (bundled in release archives) or
https://github.com/rlnorthcutt/reflector for the full text.`)
}
