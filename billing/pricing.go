package billing

// PricingTable maps model names to per-token prices in USD.
// Prices are per 1,000 tokens.
type PricingTable struct {
	models map[string][2]float64 // [promptPer1K, completionPer1K]
}

// NewPricingTable creates an empty PricingTable.
func NewPricingTable() *PricingTable {
	return &PricingTable{models: make(map[string][2]float64)}
}

// Add registers a model with its prompt and completion pricing (per 1K tokens).
func (pt *PricingTable) Add(model string, promptPer1K, completionPer1K float64) *PricingTable {
	pt.models[model] = [2]float64{promptPer1K, completionPer1K}
	return pt
}

// Cost computes the USD cost for promptTokens and completionTokens for model.
// Returns (promptCost, completionCost). If model is unknown, returns (0, 0).
func (pt *PricingTable) Cost(model string, promptTokens, completionTokens int) (float64, float64) {
	prices, ok := pt.models[model]
	if !ok {
		return 0, 0
	}
	promptCost := float64(promptTokens) * prices[0] / 1000.0
	completionCost := float64(completionTokens) * prices[1] / 1000.0
	return promptCost, completionCost
}

// OpenAIPricing returns a PricingTable with current OpenAI model pricing.
func OpenAIPricing() *PricingTable {
	return NewPricingTable().
		Add("gpt-4o", 0.005, 0.015).
		Add("gpt-4o-mini", 0.00015, 0.0006).
		Add("gpt-4-turbo", 0.01, 0.03).
		Add("gpt-4", 0.03, 0.06).
		Add("gpt-3.5-turbo", 0.0005, 0.0015).
		Add("o1", 0.015, 0.06).
		Add("o1-mini", 0.003, 0.012)
}

// AnthropicPricing returns a PricingTable with current Anthropic model pricing.
func AnthropicPricing() *PricingTable {
	return NewPricingTable().
		Add("claude-3-5-sonnet-20241022", 0.003, 0.015).
		Add("claude-3-5-haiku-20241022", 0.0008, 0.004).
		Add("claude-3-opus-20240229", 0.015, 0.075).
		Add("claude-3-sonnet-20240229", 0.003, 0.015).
		Add("claude-3-haiku-20240307", 0.00025, 0.00125)
}

// GooglePricing returns a PricingTable with current Google model pricing.
func GooglePricing() *PricingTable {
	return NewPricingTable().
		Add("gemini-1.5-pro", 0.00125, 0.005).
		Add("gemini-1.5-flash", 0.000075, 0.0003).
		Add("gemini-1.0-pro", 0.0005, 0.0015)
}
