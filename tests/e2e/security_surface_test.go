package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIKeyInQueryIsRejectedWhenProductionGuardEnabled(t *testing.T) {
	c := newE2EClient(t)
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/developers/dashboard?apiKey="+c.apiKey, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("query secret request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected query api key to be rejected, got %d", resp.StatusCode)
	}
}

func TestInternalRouteRequiresHMAC(t *testing.T) {
	c := newE2EClient(t)
	resp := c.post("/internal/email/test", map[string]any{"to": "nobody@example.com"}, "")
	requireStatus(t, resp, 401, 403)
}

func TestMCPWithoutAuthFails(t *testing.T) {
	c := newE2EClient(t)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/mcp/initialize", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("MCP unauthenticated request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected MCP without auth to fail, got %d", resp.StatusCode)
	}
}
