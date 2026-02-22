package app

import (
	"strconv"
	"strings"
)

func parseInviteIDFromRevokeRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/invites/")
	trimmed = strings.TrimSuffix(trimmed, "/revoke")
	trimmed = strings.Trim(trimmed, "/")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseTenantPath(path string) (slug, rest string, ok bool) {
	if !strings.HasPrefix(path, "/t/") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(path, "/t/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	slug = parts[0]
	rest = "/" + parts[1]
	return slug, rest, true
}

func parseContactIDFromRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/contacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "interactions" {
			return 0, false
		}
		trimmed = parts[0]
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseContactIDFromUpdateRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/contacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[1] != "update" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseDealIDFromRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/deals/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, false
	}
	parts := strings.Split(trimmed, "/")
	idRaw := parts[0]
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseInviteToken(rest string) (string, bool) {
	trimmed := strings.TrimPrefix(rest, "/invite/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 3 || parts[1] != "passkey" || (parts[2] != "start" && parts[2] != "finish") {
			return "", false
		}
		trimmed = parts[0]
	}
	if len(trimmed) < 10 {
		return "", false
	}
	return trimmed, true
}
