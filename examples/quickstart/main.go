// Quickstart example for Daneel — a simple agent with one tool.
//
// Usage:
//
//	export OPENAI_API_KEY=sk-...
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Rafiki81/daneel"
)

// WeatherParams defines the input for the weather tool.
type WeatherParams struct {
	City string `json:"city" desc:"City name to get weather for"`
}

func main() {
	// 1. Define a tool
	weatherTool := daneel.NewTool("get_weather", "Get current weather for a city",
		func(ctx context.Context, p WeatherParams) (string, error) {
			// Simulated weather data
			weather := map[string]string{
				"madrid":    "☀️ 28°C, sunny",
				"london":    "🌧️ 15°C, rainy",
				"tokyo":     "⛅ 22°C, partly cloudy",
				"new york":  "🌤️ 20°C, clear",
				"buenos aires": "❄️ 8°C, cold",
			}
			city := strings.ToLower(p.City)
			if w, ok := weather[city]; ok {
				return fmt.Sprintf("Weather in %s: %s", p.City, w), nil
			}
			return fmt.Sprintf("Weather in %s: 🌡️ 20°C, no data available", p.City), nil
		},
	)

	// 2. Create an agent with tools and permissions
	agent := daneel.New("weather-assistant",
		daneel.WithInstructions("You are a helpful weather assistant. Use the get_weather tool to answer weather questions. Be concise."),
		daneel.WithModel("gpt-4o"),
		daneel.WithTools(weatherTool),
		daneel.WithPermissions(
			daneel.AllowTools("get_weather"), // only this tool is allowed
		),
		daneel.WithMaxTurns(5),
	)

	// 3. Run the agent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := "What's the weather like in Madrid and Tokyo?"
	if len(os.Args) > 1 {
		input = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("User: %s\n", input)

	result, err := daneel.Run(ctx, agent, input)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", result.Output)
	fmt.Printf("\n--- Stats ---\n")
	fmt.Printf("Turns: %d\n", result.Turns)
	fmt.Printf("Tool calls: %d\n", len(result.ToolCalls))
	for _, tc := range result.ToolCalls {
		fmt.Printf("  - %s (%s) → %s\n", tc.Name, string(tc.Arguments), tc.Result)
	}
	fmt.Printf("Tokens: %d (prompt: %d, completion: %d)\n",
		result.Usage.TotalTokens, result.Usage.PromptTokens, result.Usage.CompletionTokens)
	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("Session: %s\n", result.SessionID)
}
