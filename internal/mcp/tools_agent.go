package mcp

// tools_agent.go — MCP tools for agent self-service: policy inspection,
// active grant listing and dry-run capability execution.
// These tools are registered in tools.go alongside the core tool list.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"payment-gateway/internal/database"
)

// ─── listAgentGrants ─────────────────────────────────────────────────────────

// toolListAgentGrants returns active access grants for the calling agent wallet.
func (s *Server) toolListAgentGrants(ctx context.Context, args map[string]any) (any, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database indisponivel")
	}
	wallet := strings.ToLower(strings.TrimSpace(stringArg(args, "agentWallet")))
	if wallet == "" {
		return nil, fmt.Errorf("agentWallet e obrigatorio")
	}
	grants, err := s.db.ListAgentActiveGrants(ctx, wallet)
	if err != nil {
		return nil, err
	}
	if grants == nil {
		grants = []*database.AgentGrantSummary{}
	}
	return map[string]any{
		"agentWallet": wallet,
		"grants":      grants,
		"count":       len(grants),
		"note":        "Only active, non-expired grants are listed. Use executeCapability with the grant's accessToken.",
	}, nil
}

// ─── getAgentPolicy ──────────────────────────────────────────────────────────

// toolGetAgentPolicy returns the execution policy and limits for an agent wallet.
func (s *Server) toolGetAgentPolicy(ctx context.Context, args map[string]any) (any, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database indisponivel")
	}
	wallet := strings.ToLower(strings.TrimSpace(stringArg(args, "agentWallet")))
	if wallet == "" {
		return nil, fmt.Errorf("agentWallet e obrigatorio")
	}

	policy, err := s.db.GetAgentPolicyByWallet(ctx, wallet)
	if err != nil {
		return nil, err
	}

	// Per-agent pricing override (if any)
	env := "sandbox"
	if s.cfg != nil && strings.EqualFold(s.cfg.Environment, "production") {
		env = "production"
	}
	pricing, _ := s.db.GetAgentPricingPolicy(ctx, wallet, env)

	// Current spend
	daily, monthly, _ := s.db.AgentPolicySpendUSDT(ctx, wallet)

	result := map[string]any{
		"agentWallet": wallet,
		"spend": map[string]any{
			"dailyUsdt":   fmt.Sprintf("%.6f", daily),
			"monthlyUsdt": fmt.Sprintf("%.6f", monthly),
		},
	}
	if policy != nil {
		result["policy"] = policy
	} else {
		result["policy"] = nil
		result["policyNote"] = "No policy found for this wallet. Policy is created on first /agent/connect."
	}
	if pricing != nil {
		result["pricingOverride"] = pricing
	} else {
		pixBps := 0
		if s.cfg != nil {
			pixBps = s.cfg.M2MPixFeeBps
		}
		ccBps := 0
		if s.cfg != nil {
			ccBps = s.cfg.M2MCreditFeeBps
		}
		result["pricingOverride"] = nil
		result["effectiveFees"] = map[string]any{
			"pixFeeBps":        pixBps,
			"creditCardFeeBps": ccBps,
			"source":           "global_env",
		}
	}
	return result, nil
}

// ─── dryRunCapability ────────────────────────────────────────────────────────

// toolDryRunCapability previews a capability execution without debiting quota.
// It returns routing, provider selection and a mock output — identical to a real
// executeCapability response, but tagged with "dry_run": true.
func (s *Server) toolDryRunCapability(ctx context.Context, args map[string]any) (any, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database indisponivel")
	}
	capability := firstNonEmptyMCP(stringArg(args, "capability"), stringArg(args, "id"))
	if capability == "" {
		return nil, fmt.Errorf("capability e obrigatoria")
	}

	// Look up capability metadata
	cap, err := s.db.GetMarketplaceCapability(ctx, capability)
	if err != nil {
		return nil, err
	}
	if cap == nil {
		return nil, fmt.Errorf("capability nao encontrada: %s", capability)
	}

	// Route selection (no quota debit)
	candidates, err := s.db.ListMarketplaceRouteCandidates(ctx, database.MarketplaceCapabilityExecuteInput{
		CapabilityID:      capability,
		RequestedProvider: stringArg(args, "provider"),
		RoutingMode:       firstNonEmptyMCP(stringArg(args, "routingMode"), "best_available"),
		Region:            stringArg(args, "region"),
		MaxLatencyMS:      intArg(args, "maxLatencyMs"),
		MaxCostScore:      intArg(args, "maxCostScore"),
		RequireReal:       boolArg(args, "requireReal"),
		Units:             maxIntMCP(intArg(args, "units"), 1),
	})
	if err != nil {
		return nil, err
	}

	var selectedRoute *database.MarketplaceRouteCandidate
	if len(candidates) > 0 {
		selectedRoute = candidates[0]
	}

	rawInput, _ := json.Marshal(args["input"])
	if args["input"] == nil {
		rawInput = json.RawMessage(`{}`)
	}

	mockOutput := map[string]any{
		"mode":       "dry_run",
		"dry_run":    true,
		"capability": capability,
		"operation":  firstNonEmptyMCP(stringArg(args, "operation"), "execute"),
		"status":     "would_execute",
		"note":       "No quota was deducted. This is a preview of the route and mock output.",
	}
	if selectedRoute != nil {
		mockOutput["provider"] = selectedRoute.ProviderSlug
		mockOutput["providerName"] = selectedRoute.ProviderName
		mockOutput["routeName"] = selectedRoute.RouteName
		mockOutput["routingMode"] = selectedRoute.RoutingMode
		mockOutput["costScore"] = selectedRoute.CostScore
		mockOutput["latencyMs"] = selectedRoute.LatencyMS
	}
	mockOutput["inputEcho"] = json.RawMessage(rawInput)

	return map[string]any{
		"dry_run":     true,
		"capability":  cap,
		"route": map[string]any{
			"selected":   selectedRoute,
			"candidates": candidates,
		},
		"preview": mockOutput,
		"units":   maxIntMCP(intArg(args, "units"), 1),
		"nextStep": "Call executeCapability with the same parameters and a valid accessToken to run for real.",
		"timestamp": time.Now().UTC(),
	}, nil
}

// ─── listAgentPaymentIntents ─────────────────────────────────────────────────

// toolListAgentPaymentIntents lists recent M2M payment intents for an agent wallet.
func (s *Server) toolListAgentPaymentIntents(ctx context.Context, args map[string]any) (any, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database indisponivel")
	}
	wallet := strings.ToLower(strings.TrimSpace(stringArg(args, "agentWallet")))
	if wallet == "" {
		return nil, fmt.Errorf("agentWallet e obrigatorio")
	}
	statusFilter := strings.TrimSpace(stringArg(args, "status"))
	intents, err := s.db.ListAgentPaymentIntentsByWallet(ctx, wallet, statusFilter, 20)
	if err != nil {
		return nil, err
	}
	if intents == nil {
		intents = []database.AgentPaymentIntent{}
	}
	return map[string]any{
		"agentWallet": wallet,
		"intents":     intents,
		"count":       len(intents),
	}, nil
}
