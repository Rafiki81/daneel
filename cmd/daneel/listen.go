package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdListen(args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	slack := fs.Bool("slack", false, "listen on Slack")
	twitter := fs.Bool("twitter", false, "listen on Twitter/X")
	telegram := fs.Bool("telegram", false, "listen on Telegram")
	whatsapp := fs.Bool("whatsapp", false, "listen on WhatsApp")
	email := fs.Bool("email", false, "listen on Email")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daneel listen [--slack] [--twitter] [--telegram] [--whatsapp] [--email]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	enabled := []string{}
	if *slack {
		enabled = append(enabled, "slack")
	}
	if *twitter {
		enabled = append(enabled, "twitter")
	}
	if *telegram {
		enabled = append(enabled, "telegram")
	}
	if *whatsapp {
		enabled = append(enabled, "whatsapp")
	}
	if *email {
		enabled = append(enabled, "email")
	}

	if len(enabled) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	fmt.Printf("listening on: %v\n", enabled)
	fmt.Println("configure platform tokens in daneel.json and register agents")
	// Block forever (Ctrl-C to stop)
	select {}
}
