package main

import (
	"encoding/json"
	"log"
	"os"

	"payment-gateway/internal/tron"
)

func main() {
	keys, err := tron.GenerateAccountKeys(3)
	if err != nil {
		log.Fatalf("failed to generate TRON keys: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(keys)
}
