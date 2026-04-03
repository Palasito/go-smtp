package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Palasito/go-smtp/internal/configgen"
)

func runGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	interactive := fs.Bool("interactive", false, "run interactive configuration wizard")
	format := fs.String("format", "env", "output format: env or compose")
	output := fs.String("output", "", "output file path (default: stdout)")
	fs.Parse(args)

	if *format != "env" && *format != "compose" {
		fmt.Fprintf(os.Stderr, "Error: invalid format %q, must be \"env\" or \"compose\"\n", *format)
		os.Exit(1)
	}

	var w *os.File
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot create output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	} else {
		w = os.Stdout
	}

	var err error
	if *interactive {
		err = configgen.RunWizard(w, *format)
	} else {
		switch *format {
		case "env":
			err = configgen.GenerateEnvTemplate(w)
		case "compose":
			err = configgen.GenerateComposeTemplate(w)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		fmt.Fprintf(os.Stderr, "Configuration written to %s\n", *output)
	}
}
