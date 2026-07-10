package mobile

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"payment-gateway/internal/models"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dest)
}

func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

// sanitizeUser removes sensitive fields before sending to client.
func sanitizeUser(u *models.User) map[string]any {
	return map[string]any{
		"id":               u.ID,
		"email":            u.Email,
		"phone":            u.Phone,
		"full_name":        u.FullName,
		"wallet_address":   u.WalletAddress,
		"pix_key":          u.PixKey,
		"kyc_status":       u.KYCStatus,
		"biometry_enabled": u.BiometryEnabled,
		"two_factor_enabled": u.TwoFactorEnabled,
		"created_at":       u.CreatedAt,
	}
}
