package app

import (
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr          string
	DataDir             string
	PublicOrigin        string
	WebAuthnRPID        string
	WebAuthnName        string
	WebAuthnOrigins     []string
	DevNoAuth           bool
	TrustedProxies      []netip.Prefix
	MaxRequestBodyBytes int64
}

func ConfigFromEnv() Config {
	listen := os.Getenv("ATTENTION_LISTEN_ADDR")
	if listen == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			// Common convention on PaaS (including Render): bind to PORT on 0.0.0.0.
			// Note: Go's ":<port>" binds on all interfaces, but we make it explicit.
			if n, err := strconv.Atoi(port); err == nil && n > 0 && n <= 65535 {
				listen = "0.0.0.0:" + port
			} else {
				listen = "0.0.0.0:8080"
			}
		} else {
			listen = "0.0.0.0:8080"
		}
	}

	dataDir := os.Getenv("ATTENTION_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(".attention", "data")
	}

	publicOrigin := strings.TrimSpace(os.Getenv("ATTENTION_PUBLIC_ORIGIN"))

	rpID := os.Getenv("ATTENTION_WEBAUTHN_RPID")
	if rpID == "" {
		if publicOrigin != "" {
			if u, err := url.Parse(publicOrigin); err == nil && u.Hostname() != "" {
				rpID = u.Hostname()
			}
		}
		if rpID == "" {
			rpID = "localhost"
		}
	}
	rpName := os.Getenv("ATTENTION_WEBAUTHN_RP_NAME")
	if rpName == "" {
		rpName = "Attention CRM"
	}
	originsEnv := os.Getenv("ATTENTION_WEBAUTHN_ORIGINS")
	origins := []string{"http://localhost:8080"}
	if originsEnv == "" && publicOrigin != "" {
		origins = []string{publicOrigin}
	}
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
	trustedProxiesEnv := os.Getenv("ATTENTION_TRUSTED_PROXY_CIDRS")
	var trustedProxies []netip.Prefix
	if trustedProxiesEnv != "" {
		for _, part := range strings.Split(trustedProxiesEnv, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			pfx, err := netip.ParsePrefix(p)
			if err == nil {
				trustedProxies = append(trustedProxies, pfx)
			}
		}
	}

	maxBodyBytes := int64(2 << 20) // 2 MiB
	if raw := strings.TrimSpace(os.Getenv("ATTENTION_MAX_BODY_BYTES")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			maxBodyBytes = n
		}
	}

	return Config{
		ListenAddr:          listen,
		DataDir:             dataDir,
		PublicOrigin:        publicOrigin,
		WebAuthnRPID:        rpID,
		WebAuthnName:        rpName,
		WebAuthnOrigins:     origins,
		DevNoAuth:           devNoAuth == "1" || strings.EqualFold(devNoAuth, "true"),
		TrustedProxies:      trustedProxies,
		MaxRequestBodyBytes: maxBodyBytes,
	}
}
