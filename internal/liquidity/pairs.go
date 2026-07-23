package liquidity

import (
	"strings"
)

type Pair struct {
	Asset           string
	Network         string
	ContractAddress string
	Decimals        int
}

type PairPolicy struct {
	pairs map[string]Pair
}

func NewPairPolicy(raw string) PairPolicy {
	policy := PairPolicy{pairs: map[string]Pair{}}
	for _, item := range splitPolicyItems(raw) {
		pair, ok := ParsePair(item)
		if !ok {
			continue
		}
		policy.pairs[pairKey(pair.Asset, pair.Network)] = pair
	}
	return policy
}

func ParsePair(raw string) (Pair, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) < 2 {
		return Pair{}, false
	}
	pair := Pair{
		Asset:           strings.ToUpper(strings.TrimSpace(parts[0])),
		Network:         normalizeNetwork(parts[1]),
		ContractAddress: "",
		Decimals:        18,
	}
	if pair.Asset == "" || pair.Network == "" {
		return Pair{}, false
	}
	if len(parts) >= 3 {
		pair.ContractAddress = strings.TrimSpace(parts[2])
	}
	if len(parts) >= 4 {
		pair.Decimals = atoiDefault(parts[3], pair.Decimals)
	}
	return pair, true
}

func (p PairPolicy) Empty() bool {
	return len(p.pairs) == 0
}

func (p PairPolicy) Allows(asset, network string) bool {
	_, ok := p.Resolve(asset, network)
	return ok
}

func (p PairPolicy) Resolve(asset, network string) (Pair, bool) {
	if len(p.pairs) == 0 {
		return Pair{}, false
	}
	pair, ok := p.pairs[pairKey(asset, network)]
	return pair, ok
}

func (p PairPolicy) Pairs() []Pair {
	out := make([]Pair, 0, len(p.pairs))
	for _, pair := range p.pairs {
		out = append(out, pair)
	}
	return out
}

func pairKey(asset, network string) string {
	return strings.ToUpper(strings.TrimSpace(asset)) + ":" + normalizeNetwork(network)
}

func splitPolicyItems(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
}

func atoiDefault(raw string, fallback int) int {
	n := 0
	for _, ch := range strings.TrimSpace(raw) {
		if ch < '0' || ch > '9' {
			return fallback
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}
