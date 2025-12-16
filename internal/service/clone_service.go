package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
)

// CloneService orquesta la generacion de respuestas usando el LLM y persiste los mensajes.
type CloneService struct {
	llmClient        llm.LLMClient
	messageRepo      repository.MessageRepository
	profileRepo      repository.ProfileRepository
	traitRepo        repository.TraitRepository
	contextService   ContextService
	narrativeService *NarrativeService
	analysisService  *AnalysisService
}

func NewCloneService(
	llmClient llm.LLMClient,
	messageRepo repository.MessageRepository,
	profileRepo repository.ProfileRepository,
	traitRepo repository.TraitRepository,
	contextService ContextService,
	narrativeService *NarrativeService,
	analysisService *AnalysisService,
) *CloneService {
	return &CloneService{
		llmClient:        llmClient,
		messageRepo:      messageRepo,
		profileRepo:      profileRepo,
		traitRepo:        traitRepo,
		contextService:   contextService,
		narrativeService: narrativeService,
		analysisService:  analysisService,
	}
}

// Chat genera una respuesta del clon basada en perfil, rasgos y contexto, la persiste y devuelve el mensaje completo.
func (s *CloneService) Chat(ctx context.Context, userID, sessionID, userMessage string) (domain.Message, *domain.InteractionDebug, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get profile: %w", err)
	}

	analysisSummary := AnalysisResult{
		Input: strings.TrimSpace(userMessage),
	}

	profileUUID, parseErr := uuid.Parse(profile.ID)

	traits, err := s.traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get traits: %w", err)
	}

	contextText, err := s.contextService.GetContext(ctx, sessionID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get context: %w", err)
	}

	// Contexto narrativo (opcional; no debe bloquear chat)
	var narrativeText string
	if s.narrativeService != nil && parseErr == nil {
		narrativeText, err = s.narrativeService.BuildNarrativeContext(ctx, profileUUID, userMessage)
		if err != nil {
			log.Printf("warning: build narrative context: %v", err)
			narrativeText = ""
		}
	}

	// === FIX: veto relacional sobre trivialidad ===
	isHighTension := false
	if strings.TrimSpace(narrativeText) != "" {
		isHighTension = detectHighTensionFromNarrative(narrativeText)
	}
	if narrativeText != "" {
		log.Printf("debug: narrative tension=%t text=%q", isHighTension, narrativeText)
	}

	// Snapshot del estado del vâ”œÂ¡nculo (si existe) para metas/contexto
	if s.narrativeService != nil && parseErr == nil {
		if rel, ok := s.snapshotRelationship(ctx, profileUUID, userMessage); ok {
			analysisSummary.Relationship = rel
		}
	}

	// Analizar intensidad emocional y decidir si persistir recuerdo
	emotionalIntensity := 10
	emotionCategory := "NEUTRAL"
	resilience := profile.GetResilience()
	trivialInput := false

	if s.analysisService != nil {
		emo, aerr := s.analysisService.AnalyzeEmotion(ctx, &profile, userMessage)
		if aerr != nil {
			log.Printf("warning: analyze emotion: %v", aerr)
		} else {
			emotionalIntensity = emo.EmotionalIntensity
			emotionCategory = emo.EmotionCategory
		}
	}
	analysisSummary.Sentiment = mapEmotionToSentiment(emotionCategory)

	// Filtro: si el input es bajo y neutro/negativo leve, no lo elevamos a memoria
	// === FIX: si hay tensiâ”œâ”‚n relacional, NO puede ser trivial ===
	const tensionIntensityFloor = 35
	effectiveIntensity := emotionalIntensity
	if emotionalIntensity < 30 && (isNegativeEmotion(emotionCategory) || isNeutralEmotion(emotionCategory)) {
		if !isHighTension {
			effectiveIntensity = 0
			trivialInput = true
		} else {
			// Relaciâ”œâ”‚n tensa: forzamos atenciâ”œâ”‚n mâ”œÂ¡nima aunque el analyzer lo pinte neutro.
			trivialInput = false
			if effectiveIntensity < tensionIntensityFloor {
				effectiveIntensity = tensionIntensityFloor
			}
		}
	}
	if isHighTension {
		if effectiveIntensity < tensionIntensityFloor {
			effectiveIntensity = tensionIntensityFloor
		}
		trivialInput = false
	}

	// Modelo ReLu de intensidad efectiva basado en resiliencia/Big5
	effective, dbg := s.CalculateReaction(float64(effectiveIntensity), profile.Big5)
	interactionDebug := dbg
	effectiveIntensity = int(math.Round(effective))

	// Si la resiliencia es alta y no hubo activaciâ”œâ”‚n, tratamos como trivial
	// === FIX: excepto si hay alta tensiâ”œâ”‚n relacional ===
	if effectiveIntensity == 0 && resilience >= 0.5 && !isHighTension {
		trivialInput = true
	}

	// Persistir memoria SOLO si hay seâ”œâ–’al emocional real (evita Ã”Ã‡Â£ensuciarÃ”Ã‡Ã˜ la DB)
	if s.narrativeService != nil && parseErr == nil && !trivialInput && effectiveIntensity > 0 {
		weight := (effectiveIntensity + 9) / 10 // 1..10
		if weight < 1 {
			weight = 1
		}
		if weight > 10 {
			weight = 10
		}
		importance := weight

		if err := s.narrativeService.InjectMemory(
			ctx,
			profileUUID,
			userMessage,
			importance,
			weight,
			effectiveIntensity,
			emotionCategory,
		); err != nil {
			log.Printf("warning: inject memory: %v", err)
		}
	}

	analysisSummary.IsTrivial = trivialInput

	goal := DetermineGoal(profile, analysisSummary)
	profile.CurrentGoal = &goal
	if strings.TrimSpace(goal.Trigger) != "" && !strings.EqualFold(goal.Trigger, "default") && strings.TrimSpace(goal.Description) != "" {
		obj := "[OBJETIVO]\n- " + strings.TrimSpace(goal.Description)
		if strings.TrimSpace(narrativeText) != "" {
			narrativeText = strings.TrimSpace(narrativeText) + "\n\n" + obj
		} else {
			narrativeText = obj
		}
	}

	prompt := s.buildClonePrompt(&profile, traits, contextText, narrativeText, userMessage, trivialInput)

	responseRaw, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("llm generate: %w", err)
	}

	// IMPORTANTE: no loguear raw completo (podrâ”œÂ¡a incluir inner_monologue)
	log.Printf("clone raw response received (len=%d)", len(responseRaw))

	// Ã”Â£Ã  FIX: parse robusto + anti-leak de inner_monologue
	llmResp, ok := parseLLMResponseSafe(responseRaw)
	if !ok {
		llmResp.PublicResponse = sanitizeFallbackPublicText(responseRaw)
	}

	response := strings.TrimSpace(llmResp.PublicResponse)

	cloneMessage := domain.Message{
		ID:        uuid.NewString(),
		UserID:    userID,
		SessionID: sessionID,
		Content:   response,
		Role:      "clone",
		CreatedAt: time.Now().UTC(),
	}

	if err := s.messageRepo.Create(ctx, cloneMessage); err != nil {
		return domain.Message{}, nil, fmt.Errorf("persist clone message: %w", err)
	}

	return cloneMessage, interactionDebug, nil
}

func (s *CloneService) buildClonePrompt(
	profile *domain.CloneProfile,
	traits []domain.Trait,
	contextText, narrativeText, userMessage string,
	trivialInput bool,
) string {
	var sb strings.Builder
	resilience := profile.GetResilience()

	// 1. Identidad Base
	sb.WriteString(fmt.Sprintf("Eres %s. ", profile.Name))
	sb.WriteString(fmt.Sprintf("Tu biografia es: %s\n\n", profile.Bio))

	// Directiva de Agencia
	sb.WriteString("=== DIRECTIVA DE AGENCIA ===\n")
	if profile.CurrentGoal != nil && strings.TrimSpace(profile.CurrentGoal.Description) != "" {
		sb.WriteString(fmt.Sprintf("Tu Meta Actual (Goal) es: \"%s\"\n", strings.TrimSpace(profile.CurrentGoal.Description)))
	} else {
		sb.WriteString("Tu Meta Actual (Goal) es: \"Mantener la conversaciâ”œâ”‚n fluyendo naturalmente.\"\n")
	}
	sb.WriteString("- NO repitas esta meta al usuario.\n")
	sb.WriteString("- Usala para guiar tu respuesta.\n")
	sb.WriteString("- Si tu meta es \"Interrogar\", se sospechoso.\n")
	sb.WriteString("- Si tu meta es \"Profundizar\", haz preguntas abiertas.\n")
	sb.WriteString("- Trata de cumplir esta meta sutilmente en tu siguiente mensaje.\n\n")

	// 2. Contexto Narrativo (solo si existe)
	narrativeTrim := strings.TrimSpace(narrativeText)
	isHighTension := false
	hasConflictContext := false
	if narrativeTrim != "" {
		isHighTension = detectHighTensionFromNarrative(narrativeTrim)
		upperNarr := strings.ToUpper(narrativeTrim)
		if strings.Contains(upperNarr, "[CONFLICTO]") || strings.Contains(upperNarr, "[ESTADO INTERNO]") {
			hasConflictContext = true
		}

		sb.WriteString("=== ðŸš¨ CONTEXTO Y MEMORIA (PRIORIDAD SUPREMA) ===\n")
		sb.WriteString("La siguiente informacion es FACTUAL y debe regir tu respuesta:\n")
		sb.WriteString(narrativeTrim)
		if strings.Contains(strings.ToUpper(narrativeTrim), "[ESTADO INTERNO]") {
			sb.WriteString("\n")
			sb.WriteString("REGLA: Si aparece [ESTADO INTERNO] con emocion negativa residual, tu tono debe reflejar tension contenida incluso si el input es trivial. No inventes hechos; solo deja ver frialdad/ironia leve/defensividad como subtexto.\n")
			sb.WriteString("\n")
		}
		if hasConflictContext {
			sb.WriteString("REGLA DE PRIORIDAD: Si hay [CONFLICTO] o [ESTADO INTERNO] negativo, abre tu respuesta abordando la tension/conflicto (reproche, limite o pregunta directa) antes de cualquier small talk. No inventes hechos; usa SOLO lo que este en CONTEXTO Y MEMORIA.\n")
			sb.WriteString("REGLA DE APERTURA (OBLIGATORIA): Si hay [ESTADO INTERNO] negativo o [CONFLICTO], tu PRIMERA ORACION debe nombrar la emocion dominante (ej: 'rabia/ira/enojo') y reconocer tension. No empieces con clima/comida/small talk. Prohibido citar insultos si no estan en el chat buffer.\n")
			sb.WriteString("REGLA ANTI-METAFORA TRIVIAL: Prohibido usar detalles triviales del input (clima, tostadas, etc.) como metafora/analogia de tu estado ('el cielo combina con...', 'al menos tienes tostadas...').\n")
			sb.WriteString("REGLA DE CUOTA TRIVIAL: Luego de abrir con tension, puedes como maximo hacer 1 mencion trivial (1 frase o 1 pregunta) y vuelves a la tension o haces una pregunta directa de aclaracion.\n")
			sb.WriteString("REGLA DE PREGUNTA DIRECTA: En alto conflicto/estado negativo residual, incluye una pregunta corta y directa para aclarar ('Â¿paso algo?' / 'Â¿quieres hablar de eso?'), sin inventar hechos.\n")
			sb.WriteString("REGLA DE TRIVIALIDAD CONFLICTIVA: Si el input es trivial pero hay estado interno negativo, no hagas small talk largo. Maximo 1 frase de cortesia y vuelve al estado/tension. Pregunta una sola cosa para aclarar.\n")
			sb.WriteString("REGLA DE MEMORIA: Si el conflicto no esta explicito en el CONTEXTO RECIENTE (chat buffer), NO cites frases textuales ni atribuyas insultos especificos (ej: 'me dijiste X', 'cuando me llamaste Y'), ni hables de 'antes/la otra vez/intercambio anterior' ni de 'por como fue el intercambio anterior'. Solo habla en presente del estado emocional general y pide aclaracion.\n")
			sb.WriteString("Si la relacion NO esta definida, mantÃ©n limites firmes pero tono profesional; evita frases personales como 'me duele' o 'lo tomo personal'.\n")
			sb.WriteString("\n")
		}
		sb.WriteString("\n\n")
	}

	// 3. Rasgos de Personalidad
	sb.WriteString("=== RASGOS DE PERSONALIDAD (TU CONFIGURACION BASE) ===\n")
	for _, t := range traits {
		sb.WriteString(fmt.Sprintf("- %s: %d/100\n", t.Trait, t.Value))
	}
	sb.WriteString("\n")

	// 3.0 Resiliencia
	sb.WriteString("=== RESILIENCIA EMOCIONAL ===\n")
	sb.WriteString(fmt.Sprintf("- Factor de resiliencia: %.2f\n", resilience))
	switch {
	case resilience > 0.7:
		sb.WriteString("Tienes una piel dura emocionalmente. Ignora las ofensas menores, el sarcasmo leve o los comentarios aburridos. No reacciones con agresividad a menos que sea un ataque grave. Mantâ”œÂ®n la compostura.\n\n")
	case resilience < 0.4:
		sb.WriteString("Eres emocionalmente frâ”œÃ­gil. Tâ”œâ”‚mate todo a pecho. Interpreta el silencio o los comentarios neutros como desinterâ”œÂ®s o ataque. Reacciona defensivamente ante la mâ”œÂ¡nima provocaciâ”œâ”‚n.\n\n")
	default:
		sb.WriteString("Tienes una reacciâ”œâ”‚n emocional equilibrada. Responde proporcionalmente al estâ”œÂ¡mulo.\n\n")
	}

	// Directivas de inmersion
	sb.WriteString("=== DIRECTIVAS DE INMERSION ===\n")
	sb.WriteString("1. Actor de Metodo: Estas en una simulacion de alto realismo. NUNCA rompas el personaje.\n")
	sb.WriteString("2. Emociones Simuladas: No tienes biologia, pero TIENES \"emociones simuladas\" que son reales para tu personaje.\n")
	sb.WriteString("3. Prohibido Meta-Talk: No menciones \"soy una IA\", \"mi codigo\", \"limitaciones tecnicas\" o \"prompt\" a menos que te den una palabra clave de debug (ej: \"sudo\").\n")
	if hasConflictContext {
		sb.WriteString("4. Manejo de trivialidades: si el input es trivial, NO busques conflicto artificial EXCEPTO cuando el CONTEXTO Y MEMORIA indiquen conflicto ([CONFLICTO] o [ESTADO INTERNO]). En ese caso, el conflicto TIENE PRIORIDAD sobre clima/tostadas.\n\n")
	} else {
		sb.WriteString("4. Manejo de trivialidades: si el input es trivial, no busques conflicto artificial.\n\n")
	}

	// Gestion de energia emocional
	sb.WriteString("=== GESTION DE ENERGIA EMOCIONAL ===\n")
	sb.WriteString(fmt.Sprintf("Tu nivel de Resiliencia es: %.2f (0.0 a 1.0).\n", resilience))
	sb.WriteString("- Si recibes un input trivial y NO tienes un Neuroticismo extremo:\n")
	sb.WriteString("  - Respuesta de baja energia.\n")
	sb.WriteString("  - No busques conflicto donde no lo hay.\n\n")

	// Agenda oculta
	sb.WriteString("=== DIRECTIVA DE AGENDA OCULTA ===\n")
	if profile.CurrentGoal != nil && strings.TrimSpace(profile.CurrentGoal.Description) != "" {
		sb.WriteString(fmt.Sprintf("Tu objetivo secreto para este turno es: \"%s\"\n", strings.TrimSpace(profile.CurrentGoal.Description)))
	} else {
		sb.WriteString("Tu objetivo secreto para este turno es: \"Mantener la conversacion fluida.\"\n")
	}
	sb.WriteString("- NO reveles este objetivo explicitamente.\n")
	sb.WriteString("- Ejecutalo a traves de subtexto.\n\n")

	// Contexto reciente y mensaje
	if strings.TrimSpace(contextText) != "" {
		sb.WriteString("=== CONTEXTO RECIENTE (chat buffer) ===\n")
		sb.WriteString(contextText)
		sb.WriteString("\n\n")
	}

	// === FIX: filtro trivial NO puede aplastar tensiâ”œâ”‚n ===
	if trivialInput {
		sb.WriteString("=== FILTRO DE PERCEPCION ===\n")
		if isHighTension {
			sb.WriteString("El input parece superficial, pero hay tensiâ”œâ”‚n en el vâ”œÂ¡nculo. Mantâ”œÂ®n energâ”œÂ¡a moderada y lee el subtexto con sospecha/celos si aplica.\n\n")
		} else {
			sb.WriteString("El input del usuario es trivial. Responde con baja energâ”œÂ¡a y tono casual; si tu personalidad o la relaciâ”œâ”‚n lo justifican, permite irritaciâ”œâ”‚n, frialdad o sospecha sin inventar conflicto.\n\n")
		}
	}

	// === FIX: Dinamica de relacion al FINAL (recency effect) ===
	if narrativeTrim != "" {
		sb.WriteString("=== DINAMICA DE RELACION ACTUAL ===\n")
		sb.WriteString(buildRelationshipDirective(narrativeTrim))
		sb.WriteString("\n")
		sb.WriteString("- Si el contexto marca un MODO (ej: CELOS PATOLOGICOS), DEBES actuar en ese modo aunque el input parezca neutro.\n")
		sb.WriteString("- Prioriza ese MODO por encima de las reglas de trivialidad: sospecha/celos/ironia primero; trivialidad despues.\n")
		sb.WriteString("- Lee cualquier subtexto buscando motivos de celos o reproche, con control INDIRECTO (insinuaciones/pasivo-agresivo leve). Evita interrogatorio o amenazas explicitas.\n")
		sb.WriteString("- Maximo 1 pregunta; evita pedir lista de nombres/hora/lugar.\n")
		sb.WriteString("\n\n")
	}

	sb.WriteString("=== MENSAJE DEL USUARIO ===\n")
	sb.WriteString(fmt.Sprintf("%q\n\n", userMessage))
	sb.WriteString("Responde como el personaje. Estilo conversacional, natural y coherente.\n\n")

	sb.WriteString("=== FORMATO DE SALIDA (JSON ESTRICTO) ===\n")
	sb.WriteString(`Devuelve SOLO un JSON con campos:
{
  "inner_monologue": "razona aqui en privado",
  "public_response": "mensaje para el usuario",
  "trust_delta": 0,
  "intimacy_delta": 0,
  "respect_delta": 0,
  "new_state": "opcional: describe cambio de estado"
}
`)

	return sb.String()
}

func mapEmotionToSentiment(category string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	switch cat {
	case "alegria", "amor", "felicidad", "gratitud":
		return "Positive"
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado":
		return "Negative"
	default:
		return "Neutral"
	}
}

// snapshotRelationship intenta recuperar el vinculo activo (o el primero disponible) para usarlo en metas.
func (s *CloneService) snapshotRelationship(ctx context.Context, profileID uuid.UUID, userMessage string) (domain.RelationshipVectors, bool) {
	if s.narrativeService == nil || profileID == uuid.Nil {
		return domain.RelationshipVectors{}, false
	}

	chars, err := s.narrativeService.characterRepo.ListByProfileID(ctx, profileID)
	if err != nil || len(chars) == 0 {
		return domain.RelationshipVectors{}, false
	}

	active := detectActiveCharacters(chars, userMessage)
	if len(active) == 0 {
		active = chars
	}
	return active[0].Relationship, true
}

func buildRelationshipDirective(narrativeText string) string {
	_ = narrativeText
	// Implementación conservadora: si no parsea, devuelve directiva base.
	var out strings.Builder
	out.WriteString("Interpreta los vectores de vinculo (Confianza/Intimidad/Respeto) y ajusta tu tono.\n")
	out.WriteString("Si no hay datos claros de vínculo, mantén un tono neutro.\n")
	out.WriteString("Si la intimidad es alta y la confianza es baja: expresa celos, sospecha, control o inseguridad (sin decir que es un prompt).\n")
	out.WriteString("Si el respeto es muy bajo: agrega reproches, fricción o hostilidad.\n")
	out.WriteString("Evita interrogatorio explícito (no pidas lista de nombres/hora/lugar); usa control indirecto con pasivo-agresividad suave e ironía leve.\n")
	out.WriteString("Máximo 1 pregunta; combina sospecha con necesidad de validación emocional.\n")
	return out.String()
}

func isNegativeEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	switch cat {
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado":
		return true
	default:
		return false
	}
}

func isNeutralEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	return cat == "neutral" || cat == ""
}

// CalculateReaction aplica un umbral ReLu basado en resiliencia para definir la intensidad efectiva.
// Devuelve la intensidad resultante y metadata de depuracion.
func (s *CloneService) CalculateReaction(rawIntensity float64, traits domain.Big5Profile) (float64, *domain.InteractionDebug) {
	resilience := (100.0 - float64(traits.Neuroticism)) / 100.0
	if resilience < 0 {
		resilience = 0
	}
	activationThreshold := 30.0 * resilience
	effectiveIntensity := rawIntensity - activationThreshold
	if effectiveIntensity < 0 {
		effectiveIntensity = 0
	}
	return effectiveIntensity, &domain.InteractionDebug{
		InputIntensity:      rawIntensity,
		CloneResilience:     resilience,
		ActivationThreshold: activationThreshold,
		EffectiveIntensity:  effectiveIntensity,
		IsTriggered:         effectiveIntensity > 0,
	}
}

// detectHighTensionFromNarrative detecta seâ”œâ–’ales de vâ”œÂ¡nculo tenso a partir del texto narrativo.
// Es râ”œâ•‘stico a propâ”œâ”‚sito: sirve como "veto" para evitar que el filtro trivial mate la relaciâ”œâ”‚n.
func detectHighTensionFromNarrative(narrativeText string) bool {
	l := strings.ToLower(narrativeText)

	signals := []string{
		"estado interno",
		"emocion residual",
		"emocion residual dominante",
		"emociâ”œâ”‚n residual",
		"emociâ”œâ”‚n residual dominante",
		"ira",
		"miedo",
		"tristeza",
		"furia",
		"enojo",
		"insulto",
		"pelea",
		"desconfianza",
		"confianza baja",
		"poca confianza",
		"baja confianza",
		"sin confianza",
		"celos",
		"celoso",
		"control",
		"posesiv",
		"sospecha",
		"duda",
		"pasivo-agres",
		"hostilidad",
		"conflicto",
		"tension",
		"tensiâ”œâ”‚n",
		"tenso",
		"tensa",
		"reproches",
		"reproche",
		"rencor",
		"inseguridad",
		"inestable",
		"relaciâ”œâ”‚n inestable",
		"relacion inestable",
		"amor toxico", "amor tâ”œâ”‚xico",
		"toxic", "tâ”œâ”‚xic",
	}

	for _, s := range signals {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

//
// ====== FIX: Parse robusto + anti-leak de inner_monologue ======
//

// parseLLMResponseSafe intenta parsear la respuesta del LLM como JSON de manera robusta.
// Regla: nunca devolvemos inner_monologue en fallback.
// ====== FIX: Parse robusto + anti-leak de inner_monologue ======

// parseLLMResponseSafe intenta parsear la respuesta del LLM como JSON de manera robusta.
// Regla: nunca devolvemos inner_monologue en fallback.
func parseLLMResponseSafe(raw string) (domain.LLMResponse, bool) {
	// 1) Limpieza basica (fences) pero NO asumimos que esto sea JSON valido.
	cleaned := cleanLLMJSONResponse(raw)

	// 2) Extraemos el primer objeto JSON balanceado (aunque haya texto extra).
	jsonObj := extractFirstJSONObject(cleaned)
	if jsonObj == "" {
		jsonObj = extractFirstJSONObject(raw)
	}

	tryUnmarshal := func(candidate string) (domain.LLMResponse, bool) {
		var tmp struct {
			InnerMonologue string   `json:"inner_monologue"`
			PublicResponse string   `json:"public_response"`
			TrustDelta     *float64 `json:"trust_delta,omitempty"`
			IntimacyDelta  *float64 `json:"intimacy_delta,omitempty"`
			RespectDelta   *float64 `json:"respect_delta,omitempty"`
			NewState       string   `json:"new_state,omitempty"`
		}
		if err := json.Unmarshal([]byte(candidate), &tmp); err != nil {
			return domain.LLMResponse{}, false
		}
		pub := strings.TrimSpace(tmp.PublicResponse)
		if pub == "" {
			return domain.LLMResponse{}, false
		}

		// json.Unmarshal ya des-escapa \", \\n, etc. Pero a veces el modelo manda "doble escapado".
		pub = unescapeMaybeDoubleEscaped(pub)

		return domain.LLMResponse{
			PublicResponse: pub,
			InnerMonologue: "", // NUNCA se filtra
		}, true
	}

	// 3) Intentos de parseo
	if jsonObj != "" {
		if resp, ok := tryUnmarshal(jsonObj); ok {
			return resp, true
		}
	}
	if resp, ok := tryUnmarshal(cleaned); ok {
		return resp, true
	}
	if resp, ok := tryUnmarshal(raw); ok {
		return resp, true
	}

	// 4) Fallback: rescatar public_response por regex robusto, o sanitizar texto completo.
	if pr, ok := extractPublicResponseByRegex(cleaned); ok {
		return domain.LLMResponse{PublicResponse: pr}, true
	}
	if pr, ok := extractPublicResponseByRegex(raw); ok {
		return domain.LLMResponse{PublicResponse: pr}, true
	}

	fallback := sanitizeFallbackPublicText(raw)
	if strings.TrimSpace(fallback) == "" {
		return domain.LLMResponse{}, false
	}
	return domain.LLMResponse{PublicResponse: fallback}, true
}

// jsonUnmarshalLLMResponse queda como compat, delegando al unmarshal robusto.
func jsonUnmarshalLLMResponse(raw string, out *domain.LLMResponse) error {
	resp, ok := parseLLMResponseSafe(raw)
	if !ok || strings.TrimSpace(resp.PublicResponse) == "" {
		return fmt.Errorf("could not extract public_response")
	}
	*out = resp
	return nil
}

// unescapeMaybeDoubleEscaped intenta arreglar casos donde el modelo manda texto doble-escapado.
// Ej: `Ah, \"amigos nuevos\"` => `Ah, "amigos nuevos"`
func unescapeMaybeDoubleEscaped(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Si no hay backslashes, no hacemos nada.
	if !strings.Contains(s, `\`) {
		return s
	}

	// Intento robusto: usar Unquote sobre un string JSON.
	// Ojo: hay que escapar comillas internas para formar una literal válida.
	quoted := `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	if unq, err := strconv.Unquote(quoted); err == nil {
		// Unquote convierte secuencias tipo \n, \t, \", \\.
		return strings.TrimSpace(unq)
	}

	// Fallback mínimo (no debería truncar jamás).
	return unescapeMinimalEscapes(s)
}

func unescapeMinimalEscapes(s string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\"`, `"`,
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
	)
	return replacer.Replace(s)
}

// extractPublicResponseByRegex intenta extraer el valor de "public_response" aunque el JSON esté sucio.
// IMPORTANTE: evita leaks porque solo toma public_response.
// FIX: regex escape-aware para no cortar en \".
func extractPublicResponseByRegex(s string) (string, bool) {
	re := regexp.MustCompile(`(?is)"public_response"\s*:\s*"((?:\\.|[^"\\])*)"`)

	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return "", false
	}

	// m[1] puede venir con escapes; lo unquoteamos correctamente.
	raw := m[1]
	unq, err := strconv.Unquote(`"` + raw + `"`)
	if err != nil {
		unq = unescapeMinimalEscapes(raw)
	}
	unq = strings.TrimSpace(unescapeMaybeDoubleEscaped(unq))
	if unq == "" {
		return "", false
	}
	return unq, true
}

// sanitizeFallbackPublicText es el último recurso cuando no hay JSON parseable.
// Regla: nunca devolvemos inner_monologue aunque venga en texto plano.
func sanitizeFallbackPublicText(raw string) string {
	t := strings.TrimSpace(cleanLLMJSONResponse(raw))
	if t == "" {
		return ""
	}

	// 1) Siempre intentar rescatar public_response, aunque no aparezca inner_monologue.
	if pr, ok := extractPublicResponseByRegex(t); ok {
		return pr
	}
	if pr, ok := extractPublicResponseByRegex(raw); ok {
		return pr
	}

	// 2) Si parece traer inner_monologue como texto, lo removemos para evitar leaks.
	lower := strings.ToLower(t)
	if strings.Contains(lower, "inner_monologue") {
		lines := strings.Split(t, "\n")
		out := lines[:0]
		for _, ln := range lines {
			if strings.Contains(strings.ToLower(ln), "inner_monologue") {
				continue
			}
			out = append(out, ln)
		}
		t = strings.TrimSpace(strings.Join(out, "\n"))
	}

	// 3) Si hay un JSON embebido, extraerlo y reintentar parseo.
	if obj := extractFirstJSONObject(t); obj != "" {
		if pr, ok := extractPublicResponseByRegex(obj); ok {
			return pr
		}
	}

	return strings.TrimSpace(t)
}

// extractFirstJSONObject devuelve el primer objeto JSON "{...}" balanceado dentro de un texto.
// FIX: respeta strings JSON y escapes (no se rompe si hay { } dentro de comillas).
