// slack-assistant example: a Slack bot backed by a local Ollama model.
//
// Usage:
//
//	ollama pull llama3.2
//	export SLACK_BOT_TOKEN=xoxb-...
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/bridge"
	slackconn "github.com/Rafiki81/daneel/connector/slack"
)

// timeParams is the input for the current_time tool.
type timeParams struct{}

func main() {
	timeTool := daneel.NewTool("current_time", "Get the current UTC time",
		func(ctx context.Context, _ timeParams) (string, error) {
			return fmt.Sprintf("Current UTC time: %s", time.Now().UTC().Format(time.RFC3339)), nil
		},
	)

	// WithLocalStack uses Ollama for both LLM and embeddings — no OpenAI key needed.
	agent := daneel.New("slack-assistant",
		daneel.WithInstructions("You are a helpful Slack assistant. Answer concisely."),
		daneel.WithLocalStack("llama3.2", "nomic-embed-text"),
		daneel.WithTools(timeTool),
		daneel.WithMaxTurns(10),
	)

	conn := slackconn.Listen(
		os.Getenv("SLACK_BOT_TOKEN"),
		slackconn.WithChannels("general"),
	)

	b := bridge.New(
		bridge.WithAgent(agent),
		bridge.WithConnector(conn),
		bridge.WithConcurrency(10),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Slack assistant running via Ollama — press Ctrl+C to stop")
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
