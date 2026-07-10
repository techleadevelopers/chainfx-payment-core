package mobile

import "net/http"

// ============================================
// 🔷 SETTINGS HANDLERS
// ============================================

// handleGetSettings - GET /api/mobile/settings
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	// TODO: Buscar configurações do banco de dados usando userID
	// userID := userIDFromCtx(r)
	// settings, err := s.db.GetSettings(r.Context(), userID)

	settings := map[string]any{
		"dark_mode":             true,
		"language":              "pt-BR",
		"currency":              "BRL",
		"notifications_enabled": true,
		"daily_limit":           10000.00,
	}

	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSettings - PUT /api/mobile/settings
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DarkMode            *bool   `json:"dark_mode"`
		Language            *string `json:"language"`
		Currency            *string `json:"currency"`
		NotificationsEnabled *bool  `json:"notifications_enabled"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid payload"})
		return
	}

	// TODO: Atualizar configurações no banco
	// userID := userIDFromCtx(r)
	// err := s.db.UpdateSettings(r.Context(), userID, req)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "configurações atualizadas",
	})
}

// handleGetLimits - GET /api/mobile/settings/limits
func (s *Server) handleGetLimits(w http.ResponseWriter, r *http.Request) {
	// TODO: Buscar limites do usuário
	// userID := userIDFromCtx(r)
	// limits, err := s.db.GetUserLimits(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]any{
		"daily_limit":         10000.00,
		"used_today":          2500.00,
		"remaining":           7500.00,
		"max_per_transaction": 5000.00,
	})
}