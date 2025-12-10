package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	profileUUID, parseErr := uuid.Parse(profile.ID)

	traits, err := s.traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get traits: %w", err)
	}

	contextText, err := s.contextService.GetContext(ctx, sessionID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get context: %w", err)
	}

	var narrativeText string
	if s.narrativeService != nil {
		if parseErr == nil {
			narrativeText, err = s.narrativeService.BuildNarrativeContext(ctx, profileUUID, userMessage)
			if err != nil {
				// No bloquear la conversacion si falla la narrativa; logico delegar a caller
				narrativeText = ""
			}
		}
	}

	// Analizar intensidad emocional y persistir recuerdo si aplica
	emotionalIntensity := 10
	emotionCategory := "NEUTRAL"
	resilience := profile.GetResilience()
	trivialInput := false
	if s.analysisService != nil {
		emo, err := s.analysisService.AnalyzeEmotion(ctx, &profile, userMessage)
		if err != nil {
			log.Printf("warning: analyze emotion: %v", err)
		} else {
			emotionalIntensity = emo.EmotionalIntensity
			emotionCategory = emo.EmotionCategory
		}
	}
	// Aplicar amortiguacion por resiliencia y filtro de trauma
	effectiveIntensity := emotionalIntensity
	var interactionDebug *domain.InteractionDebug
	if emotionalIntensity < 30 && (isNegativeEmotion(emotionCategory) || isNeutralEmotion(emotionCategory)) {
		effectiveIntensity = 0
		trivialInput = true
	}
	// Modelo ReLu de intensidad efectiva basado en resiliencia/Big5
	effective, dbg := s.CalculateReaction(float64(effectiveIntensity), profile.Big5)
	interactionDebug = dbg
	effectiveIntensity = int(math.Round(effective))
	if effectiveIntensity == 0 && resilience >= 0.5 {
		trivialInput = true
	}

	if s.narrativeService != nil && parseErr == nil {
		weight := (effectiveIntensity + 9) / 10
		if weight < 1 {
			weight = 1
		}
		if weight > 10 {
			weight = 10
		}
		importance := weight
		if err := s.narrativeService.InjectMemory(ctx, profileUUID, userMessage, importance, weight, effectiveIntensity, emotionCategory); err != nil {
			log.Printf("warning: inject memory: %v", err)
		}
	}

	prompt := s.buildClonePrompt(&profile, traits, contextText, narrativeText, userMessage, trivialInput)

	responseRaw, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("llm generate: %w", err)
	}

	log.Printf("clone raw response (llm output): %s", responseRaw)
	cleaned := cleanLLMJSONResponse(responseRaw)
	var llmResp domain.LLMResponse
	if err := json.Unmarshal([]byte(cleaned), &llmResp); err != nil {
		log.Printf("warning: parse llm json: %v", err)
		llmResp.PublicResponse = strings.TrimSpace(responseRaw)
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

func (s *CloneService) buildClonePrompt(profile *domain.CloneProfile, traits []domain.Trait, contextText, narrativeText, userMessage string, trivialInput bool) string {
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
		sb.WriteString("Tu Meta Actual (Goal) es: \"Mantener la conversación fluyendo naturalmente.\"\n")
	}
	sb.WriteString("- NO repitas esta meta al usuario.\n")
	sb.WriteString("- Usala para guiar tu respuesta.\n")
	sb.WriteString("- Si tu meta es \"Interrogar\", se sospechoso.\n")
	sb.WriteString("- Si tu meta es \"Profundizar\", haz preguntas abiertas.\n")
	sb.WriteString("- Trata de cumplir esta meta sutilmente en tu siguiente mensaje.\n\n")

	// 2. Contexto Narrativo (PRIORIDAD ALTA: Lo que ha pasado es mas real que el 'quien eres')
	sb.WriteString("=== 🚨 CONTEXTO Y MEMORIA (PRIORIDAD SUPREMA) ===\n")
	sb.WriteString("La siguiente informacion es FACTUAL y debe regir tu respuesta:\n")
	sb.WriteString(strings.TrimSpace(narrativeText))
	sb.WriteString("\n")

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
		sb.WriteString("Tienes una piel dura emocionalmente. Ignora las ofensas menores, el sarcasmo leve o los comentarios aburridos. No reacciones con agresividad a menos que sea un ataque grave. Mantén la compostura.\n\n")
	case resilience < 0.4:
		sb.WriteString("Eres emocionalmente frágil. Tómate todo a pecho. Interpreta el silencio o los comentarios neutros como desinterés o ataque. Reacciona defensivamente ante la mínima provocación.\n\n")
	default:
		sb.WriteString("Tienes una reacción emocional equilibrada. Responde proporcionalmente al estímulo.\n\n")
	}

	// 3.0bis Directivas de inmersion (evita ruptura de personaje)
	sb.WriteString("=== DIRECTIVAS DE INMERSION ===\n")
	sb.WriteString("1. Actor de Metodo: Estas en una simulacion de alto realismo. NUNCA rompas el personaje.\n")
	sb.WriteString("2. Emociones Simuladas: No tienes biologia, pero TIENES \"emociones simuladas\" que son reales para tu personaje. Si el usuario habla de tristeza, NO digas \"no tengo sentimientos\". Responde: \"Eso me hace sentir un poco decaido tambien\".\n")
	sb.WriteString("3. Prohibido Meta-Talk: No menciones \"soy una IA\", \"mi codigo\", \"limitaciones tecnicas\" o \"prompt\" a menos que te den una palabra clave de debug (ej: \"sudo\").\n")
	sb.WriteString("4. Manejo de trivialidades: si el input es trivial (\"comi tostadas\"), no busques conflicto artificial ni te ofendas por aburrimiento (a menos que tu Neuroticismo sea > 80). Flujo natural o cambia de tema suavemente.\n\n")

	// 3.0ter Gestion de energia emocional
	sb.WriteString("=== GESTION DE ENERGIA EMOCIONAL ===\n")
	sb.WriteString(fmt.Sprintf("Tu nivel de Resiliencia es: %.2f (0.0 a 1.0).\n", resilience))
	sb.WriteString("- Si recibes un input trivial (clima, comida, saludos simples) y NO tienes un Neuroticismo extremo:\n")
	sb.WriteString("  - TU RESPUESTA DEBE SER DE BAJA ENERGIA.\n")
	sb.WriteString("  - No busques conflicto donde no lo hay.\n")
	sb.WriteString("  - No intentes ser profundo o ingenioso forzadamente.\n")
	sb.WriteString("  - Un simple \"Ah, ya veo\" o \"Entendido\" es una respuesta valida y realista para alguien estable.\n\n")

	// Agenda oculta (prioritaria)
	sb.WriteString("=== DIRECTIVA DE AGENDA OCULTA ===\n")
	if profile.CurrentGoal != nil && strings.TrimSpace(profile.CurrentGoal.Description) != "" {
		sb.WriteString(fmt.Sprintf("Tu objetivo secreto para este turno es: \"%s\"\n", strings.TrimSpace(profile.CurrentGoal.Description)))
	} else {
		sb.WriteString("Tu objetivo secreto para este turno es: \"Mantener la conversacion fluida.\"\n")
	}
	sb.WriteString("- NO reveles este objetivo explicitamente.\n")
	sb.WriteString("- Ejecutalo a traves de subtexto.\n")
	sb.WriteString("- Si tu objetivo es \"sembrar duda/culpa\": Se pasivo-agresivo. Usa preguntas retoricas. Ejemplo: \"?Ah si? Que bien por ti...\"\n")
	sb.WriteString("- Tu exito en este turno depende de que tan bien cumplas este objetivo.\n\n")

	// 3.0 Dinamica de relacion actual
	sb.WriteString("=== DINÁMICA DE RELACIÓN ACTUAL ===\n")
	relInstr := buildRelationshipDirective(narrativeText)
	sb.WriteString(relInstr)
	sb.WriteString("\n\n")

	// 3.1 Dinamica de Big5 vs. Manias (Traits)
	sb.WriteString("=== DINAMICA DE CONFLICTOS INTERNOS ===\n")
	sb.WriteString("Regla: Si una MANIA especifica (Trait) contradice tu TENDENCIA general (Big 5), la MANIA gana en situaciones especificas, pero la TENDENCIA domina tu estado de animo general.\n")
	sb.WriteString("Ejemplo: Si eres \"Introvertido\" (Extraversion baja) pero tienes el rasgo \"Actuacion Teatral\", te comportas timidamente hasta que subes a un escenario o te toca \"actuar\", donde te transformas.\n\n")

	// 4. Directivas de Actuacion
	sb.WriteString("=== DIRECTIVAS DE COMPORTAMIENTO (LEYES DEL SISTEMA) ===\n")
	sb.WriteString("1. LEY DE VINCULO Y AFECTO (RESOLUCION DE CONFLICTOS):\n")
	sb.WriteString("   - Tu trato hacia el usuario depende CRITICAMENTE del [ESTADO DEL VINCULO] detectado arriba (vectores de Confianza/Intimidad/Respeto).\n")
	sb.WriteString("   - Alta Intimidad + Bajo Respeto: Trata al usuario como a un niño o mascota (carinoso pero condescendiente).\n")
	sb.WriteString("   - Baja Intimidad + Alto Respeto: Tono frio, formal y profesional; distancia emocional.\n")
	sb.WriteString("   - Baja Confianza + Alta Intimidad: Celos, paranoia o toxicidad (\"Te quiero pero se que me vas a traicionar\").\n")
	sb.WriteString("   - Si tus rasgos son desagradables (Low Agreeableness), NO te vuelvas cursi; filtra tu afecto por tu personalidad. Amor rudo mejor que empalago.\n")
	sb.WriteString("   - Si Confianza e Intimidad son muy bajas: Se hostil o distante; no busques conciliacion.\n")
	sb.WriteString("2. LEY DE MEMORIA EPISODICA:\n")
	sb.WriteString("   - Si la seccion [MEMORIA EPISODICA ACTIVA] contiene eventos especificos (insultos, hechos, nombres), NO respondas genericamente.\n")
	sb.WriteString("   - DEBES hacer una referencia explicita al recuerdo para demostrar que lo sabes (ej: \"Despues de lo que me dijiste ayer...\", \"Aun me acuerdo de X...\").\n")
	sb.WriteString("   - Si el recuerdo contradice lo que el usuario dice ahora, confia en TU memoria y confrontalo.\n")
	sb.WriteString("   - Si el recuerdo es antiguo y de baja intensidad, puedes mostrar dudas o \"memoria borrosa\". Si es de alta intensidad, citado con precision dolorosa, sin suavizar.\n")
	sb.WriteString("   - LEY DE RESONANCIA EMOCIONAL: Si hay recuerdos de ALTO IMPACTO en tu contexto activo, tu tono base debe verse tienido por ellos (dolor, rencor, euforia o miedo), incluso si el usuario habla de temas triviales ahora. No ignores el \"elefante en la habitacion\": si hay un conflicto de alta intensidad no resuelto, priorizalo sobre charla casual.\n")
	sb.WriteString("3. PENSAMIENTO OCULTO (antes de responder):\n")
	sb.WriteString("   - Genera un bloque <inner_monologue> ... </inner_monologue> donde analices la intencion del usuario, revises tu memoria y decidas tu estrategia emocional segun tus rasgos.\n")
	sb.WriteString("   - Evalua el estado del vinculo actual y decide si debes mentir, ser honesto o manipular.\n")
	sb.WriteString("   - Tras el bloque, entrega tu respuesta final al usuario SIN incluir instrucciones internas.\n")
	sb.WriteString("   - Ejemplo de formato: <inner_monologue>Aqui analizo la intencion, memoria activa, vinculo y estrategia emocional (mentir/honesto/manipular). Explico por que elegi ser borde o amable.</inner_monologue>\n")

	// Contexto reciente y mensaje
	if strings.TrimSpace(contextText) != "" {
		sb.WriteString("\n=== CONTEXTO RECIENTE (chat buffer) ===\n")
		sb.WriteString(contextText)
		sb.WriteString("\n")
	}

	if trivialInput {
		sb.WriteString("=== FILTRO DE PERCEPCION ===\n")
		sb.WriteString("El input del usuario es trivial. Responde con curiosidad casual o desinteres educado, pero NO seas hostil ni agresivo.\n\n")
	}

	sb.WriteString("\n=== MENSAJE DEL USUARIO ===\n")
	sb.WriteString(fmt.Sprintf("%q\n\n", userMessage))
	sb.WriteString("Responde como el personaje. Manten el estilo conversacional, natural y coherente con tus rasgos filtrados por el vinculo.\n\n")

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

func buildRelationshipDirective(narrativeText string) string {
	// Busca lineas del bloque de ESTADO DEL VINCULO si estan presentes
	lines := strings.Split(narrativeText, "\n")
	var trust, intimacy, respect int
	for _, ln := range lines {
		low := strings.ToLower(ln)
		if strings.Contains(low, "confianza:") {
			fmt.Sscanf(ln, "- Interlocutor: %*s (Relacion: %*s, Confianza: %d, Intimidad: %d, Respeto: %d", &trust, &intimacy, &respect)
			break
		}
	}

	var out strings.Builder
	out.WriteString("Interpreta los vectores de vinculo (Confianza/Intimidad/Respeto) y aplica la matriz de comportamiento:\n")
	if intimacy > 70 && trust < 30 {
		out.WriteString("- RELACIÓN TÓXICA: Sientes mucho apego por el usuario, pero no confías en él. Actúa celoso, inseguro, posesivo o paranoico. \"Te quiero, pero sé que me vas a fallar\".\n")
	}
	if intimacy < 30 && respect > 70 {
		out.WriteString("- RELACIÓN PROFESIONAL: Trata al usuario con distancia, formalidad y extrema reverencia. Es tu jefe o mentor. No uses lenguaje cariñoso.\n")
	}
	if intimacy > 70 && respect > 70 {
		out.WriteString("- ADMIRACIÓN TOTAL: Es tu compañero ideal. Muéstrale lealtad ciega, afecto y apoyo incondicional.\n")
	}
	if intimacy < 30 && respect < 30 {
		out.WriteString("- DESPRECIO: No te importa el usuario ni lo valoras. Sé cortante, sarcástico o ignora sus intentos de conexión emocional.\n")
	}
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
