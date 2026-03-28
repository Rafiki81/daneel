package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Rafiki81/daneel"
)

func cmdTools(args []string, reg *daneel.Registry) {
	fs := flag.NewFlagSet("tools", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daneel tools <list|describe> [name]")
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
		listTools(reg)
	case "describe":
		if fs.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "usage: daneel tools describe <name>")
			os.Exit(1)
		}
		describeTool(reg, fs.Arg(1))
	default:
		fs.Usage()
		os.Exit(1)
	}
}

func listTools(reg *daneel.Registry) {
	tools := reg.Tools()
	if len(tools) == 0 {
		fmt.Println("no tools registered")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
	}
	_ = w.Flush()
}

func describeTool(reg *daneel.Registry, name string) {
	t := reg.FindTool(name)
	if t == nil {
		fmt.Fprintf(os.Stderr, "tool %q not found\n", name)
		os.Exit(1)
	}
	fmt.Printf("Name:   %s\n", t.Name)
	fmt.Printf("Desc:   %s\n", t.Description)
	if t.Schema != "" {
		fmt.Printf("Schema: %s\n", t.Schema)
	}
}
