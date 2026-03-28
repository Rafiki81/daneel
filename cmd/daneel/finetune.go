package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdFinetune(args []string) {
	fs := flag.NewFlagSet("finetune", flag.ExitOnError)
	dataset := fs.String("dataset", "", "path to JSONL training dataset")
	base := fs.String("base", "", "base model for fine-tuning")
	method := fs.String("method", "lora", "fine-tuning method (lora, qlora, full)")
	outputDir := fs.String("output", "./finetune-output", "output directory for the fine-tuned model")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daneel finetune --dataset <file> --base <model> [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *dataset == "" || *base == "" {
		fs.Usage()
		os.Exit(1)
	}

	if _, err := os.Stat(*dataset); err != nil {
		fmt.Fprintf(os.Stderr, "dataset file not found: %s\n", *dataset)
		os.Exit(1)
	}

	fmt.Printf("fine-tuning %s on %s (method: %s) → %s\n", *base, *dataset, *method, *outputDir)
	fmt.Println("(wire finetune.NewTrainer in your application to execute)")
}
