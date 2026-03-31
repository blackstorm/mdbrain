package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

type ParsedHost struct {
	Domain string
	Error  string
}

func ParseHostDomain(host string) ParsedHost {
	host = strings.TrimSpace(host)
	if host == "" {
		return ParsedHost{Error: "missing"}
	}
	if strings.ContainsAny(host, " /") {
		return ParsedHost{Error: "invalid"}
	}

	if strings.HasPrefix(host, "[") {
		closeIdx := strings.Index(host, "]")
		if closeIdx < 0 {
			return ParsedHost{Error: "invalid"}
		}
		domain := host[1:closeIdx]
		rest := host[closeIdx+1:]
		if domain == "" {
			return ParsedHost{Error: "invalid"}
		}
		if rest == "" {
			return ParsedHost{Domain: domain}
		}
		if !strings.HasPrefix(rest, ":") {
			return ParsedHost{Error: "invalid"}
		}
		if !validPort(rest[1:]) {
			return ParsedHost{Error: "invalid"}
		}
		return ParsedHost{Domain: domain}
	}

	parts := strings.Split(host, ":")
	if len(parts) > 2 {
		return ParsedHost{Error: "invalid"}
	}
	domain := parts[0]
	if domain == "" {
		return ParsedHost{Error: "invalid"}
	}
	for _, r := range domain {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return ParsedHost{Error: "invalid"}
		}
	}
	if len(parts) == 2 && !validPort(parts[1]) {
		return ParsedHost{Error: "invalid"}
	}
	return ParsedHost{Domain: domain}
}

func HostErrorResponse(w http.ResponseWriter, parsed ParsedHost) {
	status := http.StatusBadRequest
	body := "Bad request"
	if parsed.Error == "unbound" {
		status = http.StatusForbidden
		body = "Forbidden"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func validPort(raw string) bool {
	n, err := strconv.Atoi(raw)
	return err == nil && n >= 1 && n <= 65535
}
