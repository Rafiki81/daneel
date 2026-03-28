package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Rafiki81/daneel"
)

func cmdAgents(args []string, reg *daneel.Registry) {
	fs := flag.NewFlagSet("agents", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daneel agents <list|describe> [name]")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	sub := "list"
	if fs.NArg() > 0 {
		sub = fs.Arg(0)
	}

	switch sub {
	case "list":
		listAgents(reg)
	case "describe":
		if fs.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "usage: daneel agents describe <name>")
			os.Exit(1)
		}
		describeAgent(reg, fs.Arg(1))
	default:
		fs.Usage()
		os.Exit(1)
	}
}

func listAgents(reg *daneel.Registry) {
	agents := reg.Agents()
	if len(agents) == 0 {
		fmt.Println("no agents registered")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTOOLS\tHANDOFFS\tMAX_TURNS")
	for _, a := range agents {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", a.Name, len(a.Tools), len(a.Handoffs), a.MaxTurns)
	}
	_ = w.Flush()
}

func describeAgent(reg *daneel.Registry, name string) {
	a := reg.FindAgent(name)
	if a == nil {
		fmt.Fprintf(os.Stderr, "agent %q not found\n", name)
		os.Exit(1)
	}
	fmt.Printf("Name:         %s\n", a.Name)
	fmt.Printf("Max turns:    %d\n", a.MaxTurns)
	if len(a.Handoffs) > 0 {
		fmt.Printf("Handoffs:     %v\n", a.Handoffs)
	}
	if len(a.Tools) > 0 {
		fmt.Println("Tools:")
		for _, t := range a.Tools {
			fmt.Printf("  - %s: %s\n", t.Name, t.Description)
		}
	}
	if a.Instructions != "" {
		fmt.Printf("Instructions: %s\n", a.Instructions)
	}
}
