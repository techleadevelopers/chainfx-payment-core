package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleAgentRegistries(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	s.writeCachedDiscoveryJSON(w, r, "agent-registries:"+base, time.Minute, func() (any, error) {
		return s.agentRegistryIndex(base), nil
	})
}

func (s *Server) handleAGNTCYWellKnown(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	s.writeCachedDiscoveryJSON(w, r, "agntcy-well-known:"+base, time.Minute, func() (any, error) {
		return s.agntcyRecord(base)
	})
}

func (s *Server) handleOASFWellKnown(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	s.writeCachedDiscoveryJSON(w, r, "oasf-well-known:"+base, time.Minute, func() (any, error) {
		return s.oasfRecord(base)
	})
}

func (s *Server) handleAgentRegistryRecord(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	switch id {
	case "agntcy", "agntcy-oasf", "oasf":
		s.writeCachedDiscoveryJSON(w, r, "agent-registry-record:"+id+":"+base, time.Minute, func() (any, error) {
			return s.signedRegistryRecord(base, id)
		})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "registry record not found", "supported": []string{"agntcy-oasf", "oasf"}})
	}
}

func (s *Server) agentRegistryIndex(base string) map[string]any {
	return map[string]any{
		"agent":      "ChainFX Agent Pay",
		"version":    "1.0.0",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
		"registries": []map[string]any{
			{
				"id":          "mcp-registry",
				"name":        "Model Context Protocol Registry",
				"status":      "published",
				"manifest":    base + "/mcp/initialize",
				"server_json": base + "/.well-known/mcp-server.json",
				"protocol":    "mcp",
			},
			{
				"id":       "a2a-agent-card",
				"name":     "A2A Agent Card",
				"status":   "available",
				"manifest": base + "/.well-known/agent-card.json",
				"protocol": "a2a",
			},
			{
				"id":       "agntcy-oasf",
				"name":     "AGNTCY / OASF Agent Directory Record",
				"status":   "ready_to_publish",
				"manifest": base + "/.well-known/agntcy.json",
				"record":   base + "/agent/v1/registry-records/agntcy-oasf",
				"protocol": "oasf",
			},
			{
				"id":       "openapi",
				"name":     "OpenAPI Catalog",
				"status":   "available",
				"manifest": base + "/openapi.json",
				"protocol": "openapi",
			},
			{
				"id":       "x402",
				"name":     "x402 Capability Payments",
				"status":   "available",
				"manifest": base + "/.well-known/x402.json",
				"protocol": "x402",
			},
		},
		"trust": map[string]any{
			"jwks":           base + "/.well-known/jwks.json",
			"agent_card_sig": base + "/.well-known/agent-card.signature",
			"reputation":     base + "/.well-known/agent-reputation.json",
			"sla":            base + "/.well-known/agent-sla.json",
		},
	}
}

func (s *Server) agntcyRecord(base string) (map[string]any, error) {
	record := s.oasfRecordPayload(base)
	signed, err := s.signRegistryPayload(base, record)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":      "agntcy.oasf.agent_record",
		"record":      record,
		"provenance":  signed,
		"publishable": true,
	}, nil
}

func (s *Server) oasfRecord(base string) (map[string]any, error) {
	record := s.oasfRecordPayload(base)
	signed, err := s.signRegistryPayload(base, record)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":     "oasf.agent",
		"record":     record,
		"provenance": signed,
	}, nil
}

func (s *Server) signedRegistryRecord(base, id string) (map[string]any, error) {
	record := s.oasfRecordPayload(base)
	signed, err := s.signRegistryPayload(base, record)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":         id,
		"record":     record,
		"provenance": signed,
		"index":      base + "/agent/v1/registries",
	}, nil
}

func (s *Server) oasfRecordPayload(base string) map[string]any {
	return map[string]any{
		"name":        "ChainFX Agent Pay",
		"displayName": "ChainFX Agent Pay",
		"description": "Verifiable A2A payment agent and capability network for autonomous agents using BSC stablecoins.",
		"version":     "1.0.0",
		"provider": map[string]any{
			"name": "ChainFX",
			"url":  "https://www.chainfx.store",
		},
		"locators": map[string]any{
			"a2a":          base + "/a2a",
			"a2a_tasks":    base + "/a2a/tasks",
			"mcp":          base + "/mcp/initialize",
			"openapi":      base + "/openapi.json",
			"x402":         base + "/.well-known/x402.json",
			"agent_card":   base + "/.well-known/agent-card.json",
			"registry":     base + "/agent/v1/registries",
			"agent_pay":    base + "/agent-pay.json",
			"capabilities": base + "/marketplace/capabilities",
			"policy":       base + "/.well-known/agent-policy.json",
			"graph":        base + "/.well-known/capability-graph.json",
		},
		"skills": []map[string]any{
			{"id": "pay_pix_with_usdt", "category": "payments", "protocols": []string{"a2a"}, "assets": []string{"USDT"}, "networks": []string{"BSC"}, "countries": []string{"BR"}},
			{"id": "pay_card_bill_with_usdt", "category": "payments", "protocols": []string{"a2a"}, "assets": []string{"USDT"}, "networks": []string{"BSC"}, "countries": []string{"BR"}},
			{"id": "stablecoin_exchange", "category": "settlement", "protocols": []string{"a2a", "rest"}, "assets": []string{"USDT", "USDC"}, "networks": []string{"BSC"}},
			{"id": "capability_exchange", "category": "marketplace", "protocols": []string{"a2a", "mcp", "rest", "x402"}},
			{"id": "document_ocr", "category": "capability", "protocols": []string{"a2a", "mcp", "x402"}},
			{"id": "llm_chat", "category": "capability", "protocols": []string{"a2a", "mcp", "x402"}},
			{"id": "semantic_memory", "category": "capability", "protocols": []string{"a2a", "mcp", "x402"}},
		},
		"capability_constraints": map[string]any{
			"auth":              []string{"bearer", "x402_payment"},
			"task_lifecycle":    []string{"submitted", "working", "input_required", "completed", "failed", "canceled", "rejected"},
			"supported_assets":  []string{"USDT", "USDC"},
			"supported_network": []string{"BSC"},
			"supported_country": []string{"BR"},
		},
		"trust": map[string]any{
			"identity":      s.agentIdentityMetadata(base),
			"jwks":          base + "/.well-known/jwks.json",
			"signature":     base + "/.well-known/agent-card.signature",
			"reputation":    base + "/.well-known/agent-reputation.json",
			"sla":           base + "/.well-known/agent-sla.json",
			"observability": base + "/agent/v1/episodes",
		},
		"planning": map[string]any{
			"policy_discovery": base + "/.well-known/agent-policy.json",
			"capability_graph": base + "/.well-known/capability-graph.json",
			"semantic_aliases": []string{"pay pix", "quote usdt", "stablecoin swap", "ocr", "llm chat", "semantic memory", "pay-per-call api"},
		},
		"economics": map[string]any{
			"agent_pay": map[string]any{
				"funding_asset":   "USDT",
				"funding_network": "BSC",
				"payment_methods": []string{"pix", "credit_card"},
				"fees_bps":        map[string]int{"pix": s.cfg.M2MPixFeeBps, "credit_card": s.cfg.M2MCreditFeeBps},
			},
			"x402": map[string]any{
				"status":     "available",
				"endpoint":   base + "/x402/capabilities/{capability}/execute",
				"asset":      "USDT",
				"network":    "BSC",
				"settlement": "ERC20 transfer receipt verification",
			},
		},
		"metadata": map[string]any{
			"release_channel": "production",
			"registry_ready":  true,
			"generated_at":    time.Now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *Server) signRegistryPayload(base string, payload any) (map[string]any, error) {
	kid, _, priv := s.agentSigningMaterial(base)
	hash, canonical, err := canonicalJSONHash(payload)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(priv, canonical)
	return map[string]any{
		"algorithm":          agentIdentityAlg,
		"public_key_id":      kid,
		"jwks_url":           base + "/.well-known/jwks.json",
		"record_hash":        hash,
		"signature_encoding": "base64url",
		"signature":          base64.RawURLEncoding.EncodeToString(signature),
		"signed_at":          time.Now().UTC().Format(time.RFC3339),
	}, nil
}
