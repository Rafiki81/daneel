package billing

// Budget defines a spending limit for a specific tenant.
type Budget struct {
	Tenant string
	Limit  float64 // USD
	Period Period
}

// Alert fires when a threshold condition is met.
type Alert struct {
	Threshold func(spent, budget float64) bool
	Callback  func(tenant string, spent, budget float64)
}

// AtPercent returns a threshold function that fires when spent >= pct% of budget.
//
//	Alerts triggered when cost exceeds 80% of the budget:
//	    billing.AtPercent(80)
func AtPercent(pct float64) func(spent, budget float64) bool {
	return func(spent, budget float64) bool {
		if budget <= 0 {
			return false
		}
		return (spent/budget)*100 >= pct
	}
}

// AtAmount returns a threshold function that fires when spent >= usd dollars.
func AtAmount(usd float64) func(spent, budget float64) bool {
	return func(spent, budget float64) bool {
		return spent >= usd
	}
}
