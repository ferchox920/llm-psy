package service

import "strings"

func extractFirstJSONObject(input string) string {
	start := strings.IndexByte(input, '{')
	if start == -1 {
		return ""
	}

	inString := false
	escape := false
	depth := 0

	for i := start; i < len(input); i++ {
		ch := input[i]

		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return input[start : i+1]
			}
			if depth < 0 {
				return ""
			}
		}
	}

	return ""
}
