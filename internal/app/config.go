package app

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr      string
	DataDir         string
	WebAuthnRPID    string
	WebAuthnName    string
	WebAuthnOrigins []string
	DevNoAuth       bool
}

func ConfigFromEnv() Config {
	listen := os.Getenv("ATTENTION_LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}

	dataDir := os.Getenv("ATTENTION_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(".attention", "data")
	}

	rpID := os.Getenv("ATTENTION_WEBAUTHN_RPID")
	if rpID == "" {
		rpID = "localhost"
	}
	rpName := os.Getenv("ATTENTION_WEBAUTHN_RP_NAME")
	if rpName == "" {
		rpName = "Attention CRM"
	}
	originsEnv := os.Getenv("ATTENTION_WEBAUTHN_ORIGINS")
	origins := []string{"http://localhost:8080"}
	if originsEnv != "" {
		parts := strings.Split(originsEnv, ",")
		origins = origins[:0]
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) == 0 {
			origins = []string{"http://localhost:8080"}
		}
	}

	devNoAuth := os.Getenv("ATTENTION_DEV_NOAUTH")

	return Config{
		ListenAddr:      listen,
		DataDir:         dataDir,
		WebAuthnRPID:    rpID,
		WebAuthnName:    rpName,
		WebAuthnOrigins: origins,
		DevNoAuth:       devNoAuth == "1" || strings.EqualFold(devNoAuth, "true"),
	}
}
