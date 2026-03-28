// github-reviewer example: an agent that reviews GitHub pull requests.
//
// Usage:
//
//	export GITHUB_TOKEN=ghp_...
//	export OPENAI_API_KEY=sk-...
//	go run . --repo owner/repo --pr 42
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Rafiki81/daneel"
	ghub "github.com/Rafiki81/daneel/platform/github"
)

func main() {
	repo := flag.String("repo", "", "GitHub repository in owner/repo format (required)")
	prNum := flag.Int("pr", 0, "Pull request number (required)")
	flag.Parse()

	if *repo == "" || *prNum == 0 {
		flag.Usage()
		log.Fatal("--repo and --pr are required")
	}

	tools := ghub.Tools(os.Getenv("GITHUB_TOKEN"))

	agent := daneel.New("github-reviewer",
		daneel.WithInstructions(`You are an expert code reviewer. When asked to review a PR:
1. Use list_github_issues or search_github_code to gather context.
2. Provide structured feedback: summary, potential bugs, security concerns, and suggestions.
Keep your review concise and actionable.`),
		daneel.WithModel("gpt-4o"),
		daneel.WithTools(tools...),
		daneel.WithMaxTurns(8),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	prompt := fmt.Sprintf("Please review the latest open issues in %s and suggest improvements for PR #%d.", *repo, *prNum)
	result, err := daneel.Run(ctx, agent, prompt)
	if err != nil {
		log.Fatalf("review failed: %v", err)
	}

	fmt.Println(result.Output)
}
