package mobile

import (
	"net/http"
	"strings"

	"payment-gateway/internal/models"
)

// handleDCACreate — POST /api/mobile/dca/create
func (s *Server) handleDCACreate(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromCtx(r)
	var req struct {
		TokenSymbol string  `json:"token_symbol"`
		AmountBRL   float64 `json:"amount_brl"`
		Frequency   string  `json:"frequency"` // daily | weekly | monthly
	}
	if err := decodeJSON(r, &req); err != nil || req.AmountBRL <= 0 || req.TokenSymbol == "" || req.Frequency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token_symbol, amount_brl e frequency obrigatórios"})
		return
	}
	freq := models.DCAFrequency(req.Frequency)
	if freq != models.DCADaily && freq != models.DCAWeekly && freq != models.DCAMonthly {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "frequency deve ser daily, weekly ou monthly"})
		return
	}
	asset, _, err := s.mobileAssetBySymbol(r.Context(), req.TokenSymbol)
	if err != nil && asset == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "erro interno"})
		return
	}
	if asset == nil || !asset.Active {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token_symbol invalido ou inativo"})
		return
	}
	minBRL := 0.0
	if s != nil && s.cfg != nil {
		minBRL = float64(s.cfg.OrderMinBrl)
	}
	if minBRL > 0 && req.AmountBRL < minBRL {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "amount_brl abaixo do mínimo"})
		return
	}
	strategy, err := mobileDB(s.db).CreateDCA(r.Context(), uid, strings.ToUpper(req.TokenSymbol), req.AmountBRL, freq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, strategy)
}

// handleDCAList — GET /api/mobile/dca/strategies
func (s *Server) handleDCAList(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromCtx(r)
	list, err := mobileDB(s.db).ListDCA(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"strategies": list, "count": len(list)})
}

// handleDCAUpdate — PUT /api/mobile/dca/{id}
func (s *Server) handleDCAUpdate(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromCtx(r)
	id := r.PathValue("id")
	var req struct {
		Active    *bool                `json:"active"`
		AmountBRL *float64             `json:"amount_brl"`
		Frequency *models.DCAFrequency `json:"frequency"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "payload inválido"})
		return
	}
	if err := mobileDB(s.db).UpdateDCA(r.Context(), id, uid, req.Active, req.AmountBRL, req.Frequency); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	strategy, _ := mobileDB(s.db).GetDCA(r.Context(), id)
	writeJSON(w, http.StatusOK, strategy)
}

// handleDCADelete — DELETE /api/mobile/dca/{id}
func (s *Server) handleDCADelete(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromCtx(r)
	id := r.PathValue("id")
	if err := mobileDB(s.db).DeleteDCA(r.Context(), id, uid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDCAStatus — GET /api/mobile/dca/{id}/status
func (s *Server) handleDCAStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	strategy, err := mobileDB(s.db).GetDCA(r.Context(), id)
	if err != nil || strategy == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "estratégia não encontrada"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             strategy.ID,
		"active":         strategy.Active,
		"total_invested": strategy.TotalInvested,
		"total_tokens":   strategy.TotalTokens,
		"next_execution": strategy.NextExecution,
	})
}
