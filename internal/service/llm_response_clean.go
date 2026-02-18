package service

// cleanLLMJSONResponse mantiene compatibilidad con código existente.
// La implementación canónica vive en CleanLLMJSONResponse.
func cleanLLMJSONResponse(raw string) string {
	return CleanLLMJSONResponse(raw)
}
