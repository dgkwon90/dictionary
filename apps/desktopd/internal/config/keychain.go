package config

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	keychainService          = "neulsang"
	keychainAccountGeminiKey = "gemini_api_key"
)

func LoadGeminiKeyFromKeychain() error {
	if os.Getenv("NEULSANG_GEMINI_API_KEY") != "" {
		return nil
	}

	value, err := keyring.Get(keychainService, keychainAccountGeminiKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	}

	return os.Setenv("NEULSANG_GEMINI_API_KEY", value)
}
