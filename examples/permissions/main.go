// permissions example: demonstrates fine-grained tool and handoff controls.
//
// This example creates a customer-support pipeline where:
//   - A triage agent classifies requests and hands off to specialists.
//   - Each specialist has restricted tool access via PermissionRules.
//   - Sensitive tools (like refund) require explicit allow-listing.
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
	"time"

	"github.com/Rafiki81/daneel"
)

type orderParams struct {
	OrderID string `json:"order_id" desc:"The order identifier"`
}

type refundParams struct {
	OrderID string  `json:"order_id" desc:"The order identifier"`
	Amount  float64 `json:"amount"   desc:"Refund amount in USD"`
}

type faqParams struct {
	Topic string `json:"topic" desc:"FAQ topic to look up"`
}

func main() {
	lookupOrder := daneel.NewTool("lookup_order", "Retrieve order status",
		func(ctx context.Context, p orderParams) (string, error) {
			return fmt.Sprintf("Order %s: shipped, expected 2025-08-01", p.OrderID), nil
		},
	)

	issueRefund := daneel.NewTool("issue_refund", "Issue a refund for an order",
		func(ctx context.Context, p refundParams) (string, error) {
			return fmt.Sprintf("Refund of $%.2f issued for order %s", p.Amount, p.OrderID), nil
		},
	)

	lookupFAQ := daneel.NewTool("lookup_faq", "Look up an FAQ answer",
		func(ctx context.Context, p faqParams) (string, error) {
			answers := map[string]string{
				"returns":  "Items can be returned within 30 days.",
				"shipping": "Standard shipping takes 5-7 business days.",
			}
			if a, ok := answers[p.Topic]; ok {
				return a, nil
			}
			return "No FAQ entry found for: " + p.Topic, nil
		},
	)

	billingAgent := daneel.New("billing",
		daneel.WithInstructions("You handle billing issues: order lookups and refunds."),
		daneel.WithModel("gpt-4o"),
		daneel.WithTools(lookupOrder, issueRefund),
		daneel.WithPermissions(
			daneel.AllowTools("lookup_order", "issue_refund"),
		),
		daneel.WithMaxTurns(5),
	)

	faqAgent := daneel.New("faq",
		daneel.WithInstructions("You answer frequently asked questions using the lookup_faq tool."),
		daneel.WithModel("gpt-4o"),
		daneel.WithTools(lookupFAQ),
		daneel.WithPermissions(
			daneel.AllowTools("lookup_faq"),
			daneel.DenyTools("lookup_order", "issue_refund"),
		),
		daneel.WithMaxTurns(3),
	)

	triage := daneel.New("triage",
		daneel.WithInstructions(`You are a customer support triage agent.
Classify the user request and hand off to the right specialist:
- Billing/order/refund questions -> billing agent
- General FAQ questions -> faq agent
Do not answer questions yourself; always hand off.`),
		daneel.WithModel("gpt-4o"),
		daneel.WithHandoffs(billingAgent, faqAgent),
		daneel.WithMaxTurns(3),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queries := []string{
		"I need a refund for order ORD-9912, I paid $49.99",
		"What is your return policy?",
	}

	for _, q := range queries {
		fmt.Printf("\nUser: %s\n", q)
		result, err := daneel.Run(ctx, triage, q)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}
		fmt.Printf("Agent: %s\n", result.Output)
	}
}
