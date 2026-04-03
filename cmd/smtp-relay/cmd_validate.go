package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Palasito/go-smtp/internal/configgen"
)

func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	file := fs.String("file", "", "path to .env file to validate")
	fs.Parse(args)

	if *file == "" {
		fmt.Fprintln(os.Stderr, "Error: --file is required")
		fs.Usage()
		os.Exit(1)
	}

	findings, err := configgen.ValidateEnvFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	errors := 0
	warnings := 0
	for _, f := range findings {
		fmt.Println(f.String())
		if f.Severity == configgen.SeverityError {
			errors++
		} else {
			warnings++
		}
	}

	fmt.Printf("\n%d error(s), %d warning(s)\n", errors, warnings)
	if errors > 0 {
		os.Exit(1)
	}
}
