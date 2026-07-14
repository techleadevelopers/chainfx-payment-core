package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTierLimitSupportsMCPAvailabilityLoad(t *testing.T) {
	if got := tierLimit("sk_test_cfx_probe", "mcp_tool_read"); got != 900 {
		t.Fatalf("expected test MCP read limit 900/min, got %d", got)
	}
	if got := tierLimit("sk_live_cfx_probe", "mcp_tool_read"); got != 1800 {
		t.Fatalf("expected live MCP read limit 1800/min, got %d", got)
	}
	if got := tierLimit("", "mcp_tool_read"); got != 300 {
		t.Fatalf("expected anonymous MCP read limit 300/min, got %d", got)
	}
	if got := tierLimit("sk_live_cfx_probe", "mcp_financial"); got != 300 {
		t.Fatalf("expected live MCP financial limit 300/min, got %d", got)
	}
	if got := tierLimit("sk_live_cfx_probe", "mcp_abuse"); got != 30 {
		t.Fatalf("expected live MCP abuse limit 30/min, got %d", got)
	}
}

func TestMCPStaticDiscoveryUsesCachedJSON(t *testing.T) {
	s := New(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	s.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"tools"`) || !strings.Contains(rec.Body.String(), `"get_rates"`) {
		t.Fatalf("expected tools list response, got %s", rec.Body.String())
	}
}
