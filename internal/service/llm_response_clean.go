package service

import (
	"regexp"
	"strings"
)

// cleanLLMJSONResponse quita fences ```json ... ``` y BOM, dejando el contenido usable.
func cleanLLMJSONResponse(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// BOM (por si acaso)
	s = strings.TrimPrefix(s, "\uFEFF")

	// Quitar fences tipo ```json ... ``` o ``` ... ```
	reStart := regexp.MustCompile("(?is)^\\s*```(?:json)?\\s*")
	reEnd := regexp.MustCompile("(?is)\\s*```\\s*$")
	s = reStart.ReplaceAllString(s, "")
	s = reEnd.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
