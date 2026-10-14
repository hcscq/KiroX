package automation

import (
	"testing"
)

func TestSubscriptionPlans(t *testing.T) {
	plans := GetSupportedPlans()
	if len(plans) != 4 {
		t.Fatalf("expected 4 plans, got %d", len(plans))
	}

	tests := []struct {
		planType    string
		expectedIdx int
	}{
		{PlanKiroPro, 0},
		{PlanKiroProPlus, 1},
		{PlanKiroProMax, 2},
		{PlanKiroPower, 3},
		{"PRO+", 1},
		{"unknown", 0},
	}

	for _, tt := range tests {
		idx := GetPlanButtonIndex(tt.planType)
		if idx != tt.expectedIdx {
			t.Errorf("GetPlanButtonIndex(%s) = %d, expected %d", tt.planType, idx, tt.expectedIdx)
		}
	}
}

func TestExtractStripeCheckoutURL(t *testing.T) {
	raw := `{"status":"ok","encodedVerificationUrl":"https://checkout.stripe.com/c/pay/cs_live_b1LQYD5Lym7biaBQjoQoeogCBHGLM1ie7Tpdj8QMjl0bVl07SWq3FVrz5k#fidnandhYHdWcXxpYCc%2FJ2FgY2RwaXEn"}`
	extracted := extractStripeCheckoutURL(raw)
	expected := "https://checkout.stripe.com/c/pay/cs_live_b1LQYD5Lym7biaBQjoQoeogCBHGLM1ie7Tpdj8QMjl0bVl07SWq3FVrz5k#fidnandhYHdWcXxpYCc%2FJ2FgY2RwaXEn"
	if extracted != expected {
		t.Errorf("extractStripeCheckoutURL() = %s, expected %s", extracted, expected)
	}

	none := extractStripeCheckoutURL("no stripe url here")
	if none != "" {
		t.Errorf("expected empty string, got %s", none)
	}
}
