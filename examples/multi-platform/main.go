// multi-platform example: run one agent on Slack and Telegram simultaneously.
//
// Usage:
//
//	export SLACK_BOT_TOKEN=xoxb-...
//	export TELEGRAM_BOT_TOKEN=123:ABC...
//	export OPENAI_API_KEY=sk-...
//	go run .
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/bridge"
	slackconn "github.com/Rafiki81/daneel/connector/slack"
	telegramconn "github.com/Rafiki81/daneel/connector/telegram"
)

func main() {
	agent := daneel.New("multi-platform-assistant",
		daneel.WithInstructions("You are a helpful assistant available on Slack and Telegram. Be concise."),
		daneel.WithModel("gpt-4o"),
		daneel.WithMaxTurns(10),
	)

	slackConn := slackconn.Listen(
		os.Getenv("SLACK_BOT_TOKEN"),
		slackconn.WithChannels("general"),
	)
	telegramConn := telegramconn.Listen(os.Getenv("TELEGRAM_BOT_TOKEN"))

	b := bridge.New(
		bridge.WithAgent(agent),
		bridge.WithConnector(slackConn),
		bridge.WithConnector(telegramConn),
		bridge.WithConcurrency(20),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("multi-platform agent running — press Ctrl+C to stop")
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
