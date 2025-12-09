package service

import (
	"context"
	"fmt"
	"log"
	"regexp"
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

var innerMonologueRE = regexp.MustCompile(`(?s)<inner_monologue>.*?</inner_monologue>`)

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
func (s *CloneService) Chat(ctx context.Context, userID, sessionID, userMessage string) (domain.Message, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get profile: %w", err)
	}
	profileUUID, parseErr := uuid.Parse(profile.ID)

	traits, err := s.traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get traits: %w", err)
	}

	contextText, err := s.contextService.GetContext(ctx, sessionID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get context: %w", err)
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

	prompt := s.buildClonePrompt(&profile, traits, contextText, narrativeText, userMessage)

	// Analizar intensidad emocional y persistir recuerdo si aplica
	emotionalIntensity := 10
	emotionCategory := "NEUTRAL"
	if s.analysisService != nil {
		emo, err := s.analysisService.AnalyzeEmotion(ctx, userMessage)
		if err != nil {
			log.Printf("warning: analyze emotion: %v", err)
		} else {
			emotionalIntensity = emo.EmotionalIntensity
			emotionCategory = emo.EmotionCategory
		}
	}
	if s.narrativeService != nil && parseErr == nil {
		weight := (emotionalIntensity + 9) / 10
		if weight < 1 {
			weight = 1
		}
		if weight > 10 {
			weight = 10
		}
		importance := weight
		if err := s.narrativeService.InjectMemory(ctx, profileUUID, userMessage, importance, weight, emotionalIntensity, emotionCategory); err != nil {
			log.Printf("warning: inject memory: %v", err)
		}
	}

	responseRaw, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, fmt.Errorf("llm generate: %w", err)
	}

	log.Printf("clone raw response (with inner monologue): %s", responseRaw)
	response := strings.TrimSpace(innerMonologueRE.ReplaceAllString(responseRaw, ""))

	cloneMessage := domain.Message{
		ID:        uuid.NewString(),
		UserID:    userID,
		SessionID: sessionID,
		Content:   response,
		Role:      "clone",
		CreatedAt: time.Now().UTC(),
	}

	if err := s.messageRepo.Create(ctx, cloneMessage); err != nil {
		return domain.Message{}, fmt.Errorf("persist clone message: %w", err)
	}

	return cloneMessage, nil
}

func (s *CloneService) buildClonePrompt(profile *domain.CloneProfile, traits []domain.Trait, contextText, narrativeText, userMessage string) string {
	var sb strings.Builder

	// 1. Identidad Base
	sb.WriteString(fmt.Sprintf("Eres %s. ", profile.Name))
	sb.WriteString(fmt.Sprintf("Tu biografia es: %s\n\n", profile.Bio))

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

	sb.WriteString("\n=== MENSAJE DEL USUARIO ===\n")
	sb.WriteString(fmt.Sprintf("%q\n\n", userMessage))
	sb.WriteString("Responde como el personaje. Manten el estilo conversacional, natural y coherente con tus rasgos filtrados por el vinculo.")

	return sb.String()
}
