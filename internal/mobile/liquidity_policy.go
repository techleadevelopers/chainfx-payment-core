package mobile

import (
	"strings"

	"payment-gateway/internal/liquidity"
)

func normalizeMobileBuyNetwork(network string) string {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "", "BSC", "BINANCE", "BEP20", "BEP-20":
		return "BSC"
	case "POL", "POLYGON", "MATIC":
		return "POLYGON"
	case "BTC", "BITCOIN":
		return "BITCOIN"
	default:
		return ""
	}
}

func (s *Server) mobileLiquidityPairSupported(asset, network string) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	asset = strings.ToUpper(strings.TrimSpace(asset))
	network = normalizeMobileBuyNetwork(network)
	if asset == "" || network == "" {
		return false
	}
	policy := liquidity.NewPairPolicy(s.cfg.LiquidityAllowedPairs)
	if !policy.Empty() {
		return policy.Allows(asset, network)
	}
	return containsCSVFoldMobile(s.cfg.LiquidityAllowedAssets, asset) &&
		containsCSVFoldMobile(s.cfg.LiquidityAllowedNetworks, network)
}

func (s *Server) mobileLiquiditySupportedPairs() []map[string]any {
	if s == nil || s.cfg == nil {
		return nil
	}
	policy := liquidity.NewPairPolicy(s.cfg.LiquidityAllowedPairs)
	pairs := policy.Pairs()
	out := make([]map[string]any, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, map[string]any{
			"asset":            pair.Asset,
			"network":          pair.Network,
			"contract_address": pair.ContractAddress,
			"decimals":         pair.Decimals,
		})
	}
	return out
}

func (s *Server) mobileLiquiditySupportedTokens() []string {
	seen := map[string]bool{}
	var out []string
	for _, pair := range s.mobileLiquiditySupportedPairs() {
		asset, _ := pair["asset"].(string)
		asset = strings.ToUpper(strings.TrimSpace(asset))
		if asset != "" && !seen[asset] {
			seen[asset] = true
			out = append(out, asset)
		}
	}
	return out
}

func (s *Server) mobileLiquiditySupportedNetworks() []string {
	seen := map[string]bool{}
	var out []string
	for _, pair := range s.mobileLiquiditySupportedPairs() {
		network, _ := pair["network"].(string)
		network = strings.ToUpper(strings.TrimSpace(network))
		if network != "" && !seen[network] {
			seen[network] = true
			out = append(out, network)
		}
	}
	return out
}

func containsCSVFoldMobile(raw, value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		if strings.ToUpper(strings.TrimSpace(item)) == value {
			return true
		}
	}
	return false
}
