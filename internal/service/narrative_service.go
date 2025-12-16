package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

const defaultEmotionalWeightFactor = 0.0005
const (
	workingMemoryLimit                 = 3
	workingMemoryMinImportance         = 8
	workingMemoryMinEmotionalIntensity = 60
	maxTotalMemoriesInContext          = 7
)

type llmClientWithEmbedding interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	Generate(ctx context.Context, prompt string) (string, error)
}

type NarrativeCache interface {
	GetEvocation(key string) (string, bool)
	SetEvocation(key, val string)
	GetJudge(key string) (bool, bool)
	SetJudge(key string, val bool)
}

type NarrativeService struct {
	characterRepo repository.CharacterRepository
	memoryRepo    repository.MemoryRepository
	llmClient     llmClientWithEmbedding
	cache         NarrativeCache
}

func (s *NarrativeService) SetCache(cache NarrativeCache) { s.cache = cache }

func NewNarrativeService(
	characterRepo repository.CharacterRepository,
	memoryRepo repository.MemoryRepository,
	llmClient llmClientWithEmbedding,
) *NarrativeService {
	return &NarrativeService{
		characterRepo: characterRepo,
		memoryRepo:    memoryRepo,
		llmClient:     llmClient,
	}
}

func (s *NarrativeService) BuildNarrativeContext(ctx context.Context, profileID uuid.UUID, userMessage string) (string, error) {
	var sections []string

	msgLower := strings.ToLower(userMessage)
	negExp := strings.Contains(msgLower, "no hables de") || strings.Contains(msgLower, "olvida")
	negSem := hasNegationSemantic(msgLower)
	isBenign := detectBenignIntent(msgLower)
	isMixed := detectMixedIntent(msgLower)

	// weightFactor controla cuanto pesa lo emocional en el ranking
	weightFactor := defaultEmotionalWeightFactor
	if isBenign {
		weightFactor = 0.0
	} else if isMixed {
		// mixed: bajamos un poco el peso emocional, pero no lo anulamos
		weightFactor = defaultEmotionalWeightFactor * 0.5
	}

	// Negacion tiene prioridad absoluta
	if negExp || negSem {
		fmt.Printf("[DIAGNOSTICO] Negacion detectada, silencio total.\n")
		return "", nil
	}

	useCache := s.cache != nil
	var evocationCache map[string]string
	var judgeCache map[string]bool
	if !useCache {
		// Nota: esto es cache "por llamada"; si no usas un cache externo, esto no aporta mucho.
		evocationCache = make(map[string]string)
		judgeCache = make(map[string]bool)
	}

	chars, err := s.characterRepo.ListByProfileID(ctx, profileID)
	if err != nil {
		return "", err
	}

	active := detectActiveCharacters(chars, userMessage)
	if len(active) == 0 {
		active = chars
	}

	// FIX #2: cache keys incluyen profileID (evita contaminacion cross-user)
	evKey := profileID.String() + "||ev||" + userMessage

	searchQuery, ok := "", false
	if useCache {
		searchQuery, ok = s.cache.GetEvocation(evKey)
	} else {
		searchQuery, ok = evocationCache[evKey]
	}

	if !ok {
		searchQuery = s.generateEvocation(ctx, userMessage)
		if useCache {
			s.cache.SetEvocation(evKey, searchQuery)
		} else {
			evocationCache[evKey] = searchQuery
		}
	}

	// Normalizacion robusta
	searchQuery = strings.TrimSpace(searchQuery)
	searchQuery = strings.Trim(searchQuery, "`")
	searchQuery = strings.TrimSpace(searchQuery)
	unquoted := strings.Trim(searchQuery, `"`)
	if strings.TrimSpace(unquoted) == "" {
		searchQuery = ""
	} else {
		searchQuery = strings.TrimSpace(unquoted)
	}

	fmt.Printf("[DIAGNOSTICO] Query Vectorial: %q\n", searchQuery)

	memories := []domain.NarrativeMemory{}
	if searchQuery == "" {
		fmt.Printf("[DIAGNOSTICO] Subconsciente en silencio: no se ejecuta busqueda vectorial\n")
	} else {
		embed, err := s.llmClient.CreateEmbedding(ctx, searchQuery)
		if err != nil {
			return "", err
		}

		scored, err := s.memoryRepo.Search(ctx, profileID, pgvector.NewVector(embed), 5, weightFactor)
		if err != nil {
			return "", err
		}

		sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

		const upperSim = 0.62
		const hardFloor = 0.20
		const maxJudge = 2
		const gapScore = 0.08

		topScore := -1.0
		judgeCalls := 0

		for idx, sm := range scored {
			if idx == 0 {
				topScore = sm.Score
			}

			// Benigno: evita trauma, y acepta solo si pasa el piso
			if isBenign && shouldSkipTrauma(sm.NarrativeMemory) {
				continue
			}
			if isBenign {
				if sm.Similarity >= hardFloor {
					memories = append(memories, sm.NarrativeMemory)
				}
				continue
			}

			// Gap control (solo si NO es mixed). Si el #1 le saca mucha ventaja al #2, ignoramos #2.
			if idx == 1 && !isMixed && topScore-sm.Score >= gapScore {
				continue
			}

			// Aceptacion directa por similitud alta
			if sm.Similarity >= upperSim {
				memories = append(memories, sm.NarrativeMemory)
				continue
			}

			// No juzgamos basura
			if sm.Similarity < hardFloor {
				continue
			}

			// FIX #2: cache key del juez tambien incluye profileID
			jKey := profileID.String() + "||j||" + userMessage + "||" + sm.Content

			var use bool
			var found bool
			if useCache {
				use, found = s.cache.GetJudge(jKey)
			} else {
				use, found = judgeCache[jKey]
			}

			// FIX #1: el presupuesto maxJudge solo aplica si tenemos que llamar al juez (cache miss)
			if !found {
				if judgeCalls >= maxJudge {
					continue
				}

				var reason string
				use, reason, err = s.judgeMemory(ctx, userMessage, sm.Content)
				if err != nil {
					continue
				}
				if useCache {
					s.cache.SetJudge(jKey, use)
				} else {
					judgeCache[jKey] = use
				}
				fmt.Printf("[DIAGNOSTICO] juez use=%t reason=%q\n", use, reason)
				judgeCalls++
			}

			if use {
				memories = append(memories, sm.NarrativeMemory)
			}
		}
	}

	// Working Memory: memorias recientes/impacto alto que no dependen de similitud
	workingMemories, err := s.memoryRepo.GetRecentHighImpactByProfile(ctx, profileID, workingMemoryLimit, workingMemoryMinImportance, workingMemoryMinEmotionalIntensity)
	if err != nil {
		return "", err
	}

	allMemories := mergeDedupMemories(workingMemories, memories)
	allMemories = limitMemories(allMemories, maxTotalMemoriesInContext)

	if len(allMemories) > 0 {
		sort.Slice(allMemories, func(i, j int) bool {
			return allMemories[i].HappenedAt.After(allMemories[j].HappenedAt)
		})

		var lines []string
		sectionTitle := resolveSectionTitle(isBenign, allMemories)
		for _, m := range allMemories {
			lines = append(lines, fmt.Sprintf(
				"- [TEMA: %s | Hace %s] %s",
				strings.ToUpper(m.EmotionCategory),
				humanizeRelative(m.HappenedAt),
				m.Content,
			))
		}
		sections = append(sections, sectionTitle+"\n"+strings.Join(lines, "\n"))

		// Estado interno residual: refuerza la emocion dominante para guiar al clon.
		maxIdx := 0
		maxNorm := normalizeIntensity(allMemories[0].EmotionalIntensity)
		for i, m := range allMemories {
			norm := normalizeIntensity(m.EmotionalIntensity)
			if norm > maxNorm {
				maxNorm = norm
				maxIdx = i
			}
		}
		topMem := allMemories[maxIdx]
		if strings.ToUpper(strings.TrimSpace(topMem.EmotionCategory)) != "NEUTRAL" && maxNorm >= 60 {
			internalLine := fmt.Sprintf(
				"- Emocion residual dominante: %s (estado interno actual; NO asumas eventos previos si no estan en el chat buffer; prioriza el estado emocional sobre trivialidades; no inventes hechos ni atribuyas causas conversacionales).",
				strings.ToUpper(topMem.EmotionCategory),
			)
			sections = append(sections, "[ESTADO INTERNO]\n"+internalLine)
		}
	}

	if len(active) > 0 {
		var lines []string
		for _, c := range active {
			dyn := deriveBondDynamics(c.Relationship.Trust, c.Relationship.Intimacy, c.Relationship.Respect)

			line := fmt.Sprintf(
				"- Interlocutor: %s (Relacion: %s, Confianza: %d, Intimidad: %d, Respeto: %d, Dinamica: %s",
				c.Name,
				c.Relation,
				c.Relationship.Trust,
				c.Relationship.Intimacy,
				c.Relationship.Respect,
				dyn,
			)
			if bs := strings.TrimSpace(c.BondStatus); bs != "" {
				line += fmt.Sprintf(", Estado: %s", bs)
			}
			line += ")."
			lines = append(lines, line)
		}
		sections = append(sections, "[ESTADO DEL VINCULO]\n"+strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n"), nil
}

// --- helpers y servicios auxiliares ---

func mergeDedupMemories(primary, secondary []domain.NarrativeMemory) []domain.NarrativeMemory {
	seen := make(map[uuid.UUID]struct{})
	out := make([]domain.NarrativeMemory, 0, len(primary)+len(secondary))

	for _, m := range primary {
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	for _, m := range secondary {
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	return out
}

func limitMemories(memories []domain.NarrativeMemory, n int) []domain.NarrativeMemory {
	if n <= 0 || len(memories) <= n {
		return memories
	}
	return memories[:n]
}

func (s *NarrativeService) generateEvocation(ctx context.Context, userMessage string) string {
	msgLower := strings.ToLower(userMessage)
	if strings.Contains(msgLower, "no hables de") || hasNegationSemantic(msgLower) {
		return ""
	}

	resp, err := s.llmClient.Generate(ctx, fmt.Sprintf(evocationPromptTemplate, userMessage))
	if err == nil {
		clean := strings.TrimSpace(resp)
		if clean != "" {
			return clean
		}
	}

	resp, err = s.llmClient.Generate(ctx, fmt.Sprintf(evocationFallbackPrompt, userMessage))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(resp)
}

func (s *NarrativeService) judgeMemory(ctx context.Context, userMessage, memoryContent string) (bool, string, error) {
	resp, err := s.llmClient.Generate(ctx, fmt.Sprintf(rerankJudgePrompt, userMessage, memoryContent))
	if err != nil {
		return false, "", err
	}

	raw := extractFirstJSONObject(resp)
	if raw == "" {
		return false, "", fmt.Errorf("judge returned non-json: %q", resp)
	}

	var out struct {
		Use    bool   `json:"use"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return false, "", fmt.Errorf("unmarshal judge json: %w (raw=%q full=%q)", err, raw, resp)
	}
	return out.Use, out.Reason, nil
}
