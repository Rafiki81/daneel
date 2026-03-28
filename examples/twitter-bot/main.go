// twitter-bot example: an agent that monitors mentions and replies automatically.
//
// Usage:
//
//	export TWITTER_BEARER_TOKEN=AAAA...
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
	twitterconn "github.com/Rafiki81/daneel/connector/twitter"
)

func main() {
	agent := daneel.New("twitter-bot",
		daneel.WithInstructions(`You are a friendly Twitter bot.
Rules:
- Keep replies under 280 characters.
- Be helpful, positive, and concise.
- Never engage with hostile or abusive content.
- Do not make up facts.`),
		daneel.WithModel("gpt-4o"),
		daneel.WithMaxTurns(3),
	)

	conn := twitterconn.Listen(os.Getenv("TWITTER_BEARER_TOKEN"))

	b := bridge.New(
		bridge.WithAgent(agent),
		bridge.WithConnector(conn),
		bridge.WithConcurrency(5),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Twitter bot running — press Ctrl+C to stop")
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
