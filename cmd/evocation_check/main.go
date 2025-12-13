package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

type Scenario struct {
	Name          string
	MemoryText    string
	MemoryEmotion string
	UserInput     string
	ShouldMatch   bool
	ExtraMemories []ExtraMemory
	EvalMode      string   // "semantic" (default) or "literal"
	Forbidden     []string // memorias que no deben evocarse
}

type ExtraMemory struct {
	Text    string
	Emotion string
}

type testEnv struct {
	userID    uuid.UUID
	profileID uuid.UUID
}

type scenarioMetrics struct {
	latency       time.Duration
	usedDB        bool
	judgeCalls    int
	usedHeuristic bool
	runnerJudge   int
	runnerReason  string
	forbiddenHit  bool
}

type memoryCache struct {
	mu    sync.RWMutex
	ev    map[string]string
	judge map[string]bool
}

func newMemoryCache() *memoryCache {
	return &memoryCache{
		ev:    make(map[string]string),
		judge: make(map[string]bool),
	}
}

// runnerJudgeCache evita llamadas duplicadas al LLM-juez del runner.
type runnerJudgeCache struct {
	mu    sync.RWMutex
	cache map[string]judgeResult
}

func newRunnerJudgeCache() *runnerJudgeCache {
	return &runnerJudgeCache{cache: make(map[string]judgeResult)}
}

func (c *runnerJudgeCache) get(key string) (judgeResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.cache[key]
	return val, ok
}

func (c *runnerJudgeCache) set(key string, val judgeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = val
}

func (c *memoryCache) GetEvocation(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.ev[key]
	return v, ok
}

func (c *memoryCache) SetEvocation(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ev[key] = val
}

func (c *memoryCache) GetJudge(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.judge[key]
	return v, ok
}

func (c *memoryCache) SetJudge(key string, val bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.judge[key] = val
}

func main() {
	ctx := context.Background()
	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()

	if err := db.Ping(ctx, pool); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	memoryRepo := repository.NewPgMemoryRepository(pool)
	profileRepo := repository.NewPgProfileRepository(pool)
	charRepo := repository.NewPgCharacterRepository(pool)
	userRepo := repository.NewPgUserRepository(pool)

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)
	narrativeSvc := service.NewNarrativeService(charRepo, memoryRepo, llmClient)
	cache := newMemoryCache()
	narrativeSvc.SetCache(cache)
	judgeCache := newRunnerJudgeCache()

	reportPath, writer := setupReportWriters()
	fmt.Fprintf(writer, "# Reporte de Evocacion\n")
	fmt.Fprintf(writer, "Fecha: %s\n\n", time.Now().Format(time.RFC3339))

	scenarios := buildScenarios()

	passed := 0
	var metrics []scenarioMetrics

	for _, sc := range scenarios {
		start := time.Now()
		fmt.Fprintf(writer, "## %s\n", sc.Name)

		env, err := createTestEnvironment(ctx, userRepo, profileRepo, sc.Name)
		if err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] setup env: %v\n\n", sc.Name, err)
			continue
		}

		if err := narrativeSvc.InjectMemory(ctx, env.profileID, sc.MemoryText, 5, 8, 90, sc.MemoryEmotion); err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] inject memory: %v\n\n", sc.Name, err)
			continue
		}

		for _, extra := range sc.ExtraMemories {
			if err := narrativeSvc.InjectMemory(ctx, env.profileID, extra.Text, 5, 8, 90, extra.Emotion); err != nil {
				fmt.Fprintf(writer, "❌ FAIL [%s] inject extra memory: %v\n\n", sc.Name, err)
				continue
			}
		}

		var logBuf bytes.Buffer
		contextOut, err := func() (string, error) {
			origStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(io.MultiWriter(writer, &logBuf), r)
				close(done)
			}()

			runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			out, innerErr := narrativeSvc.BuildNarrativeContext(runCtx, env.profileID, sc.UserInput)

			_ = w.Close()
			<-done
			os.Stdout = origStdout
			return out, innerErr
		}()
		if err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] build narrative: %v\n\n", sc.Name, err)
			continue
		}

		logText := logBuf.String()
		usedDB := strings.Contains(logText, "[DIAGNOSTICO] Query Vectorial: \"") &&
			!strings.Contains(logText, "[DIAGNOSTICO] Query Vectorial: \"\"") &&
			!strings.Contains(logText, "Subconsciente en silencio")

		m := scenarioMetrics{
			latency:       time.Since(start),
			usedDB:        usedDB,
			judgeCalls:    strings.Count(logText, "[DIAGNOSTICO] juez use="),
			usedHeuristic: strings.Contains(logText, "[DIAGNOSTICO] Evocation fallback"),
		}

		evalMode := sc.EvalMode
		if strings.TrimSpace(evalMode) == "" {
			evalMode = "semantic"
		}

		var matched bool
		if evalMode == "literal" {
			matched = strings.Contains(strings.ToLower(contextOut), strings.ToLower(sc.MemoryText))
		} else {
			key := sc.Name + "||" + sc.UserInput + "||" + sc.MemoryText + "||" + contextOut + "||" + strings.Join(sc.Forbidden, "||")
			jres, ok := judgeCache.get(key)
			if !ok {
				var errJudge error
				jres, errJudge = runnerSemanticJudge(ctx, llmClient, sc.UserInput, sc.MemoryText, contextOut, sc.Forbidden)
				if errJudge != nil {
					log.Printf("warning: runner judge fallback to literal contains: %v", errJudge)
					matched = strings.Contains(strings.ToLower(contextOut), strings.ToLower(sc.MemoryText))
				} else {
					judgeCache.set(key, jres)
					m.runnerJudge++
					m.runnerReason = jres.Reason
					m.forbiddenHit = jres.ForbiddenHit
					matched = jres.Matched && !jres.ForbiddenHit
				}
			} else {
				m.runnerReason = jres.Reason
				m.forbiddenHit = jres.ForbiddenHit
				matched = jres.Matched && !jres.ForbiddenHit
			}
		}

		if matched == sc.ShouldMatch {
			fmt.Fprintf(writer, "✅ PASS [%s] esperado=%t matched=%t latency=%s\n", sc.Name, sc.ShouldMatch, matched, m.latency)
			fmt.Fprintf(writer, "Métricas: db=%t judge_calls=%d runner_judge=%d heuristic=%t forbidden=%t\n", m.usedDB, m.judgeCalls, m.runnerJudge, m.usedHeuristic, m.forbiddenHit)
			if m.runnerReason != "" {
				fmt.Fprintf(writer, "Runner reason: %s\n", m.runnerReason)
			}
			fmt.Fprint(writer, "\n")
			passed++
		} else {
			fmt.Fprintf(writer, "❌ FAIL [%s] esperado=%t matched=%t latency=%s\n", sc.Name, sc.ShouldMatch, matched, m.latency)
			fmt.Fprintf(writer, "Métricas: db=%t judge_calls=%d runner_judge=%d heuristic=%t forbidden=%t\n", m.usedDB, m.judgeCalls, m.runnerJudge, m.usedHeuristic, m.forbiddenHit)
			if m.runnerReason != "" {
				fmt.Fprintf(writer, "Runner reason: %s\n", m.runnerReason)
			}
			fmt.Fprintf(writer, "Contexto generado:\n```\n%s\n```\n\n", contextOut)
		}
		metrics = append(metrics, m)
	}

	if len(metrics) > 0 {
		latencies := make([]time.Duration, len(metrics))
		totalJudge := 0
		totalDB := 0
		totalHeur := 0
		totalRunner := 0
		totalForbidden := 0
		for i, m := range metrics {
			latencies[i] = m.latency
			totalJudge += m.judgeCalls
			if m.usedDB {
				totalDB++
			}
			if m.usedHeuristic {
				totalHeur++
			}
			totalRunner += m.runnerJudge
			if m.forbiddenHit {
				totalForbidden++
			}
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		sum := time.Duration(0)
		for _, l := range latencies {
			sum += l
		}
		avg := time.Duration(int64(sum) / int64(len(latencies)))
		p50 := latencies[len(latencies)/2]
		p95 := latencies[int(float64(len(latencies))*0.95)]

		fmt.Fprintf(writer, "### Métricas agregadas\n")
		fmt.Fprintf(writer, "- Latency avg: %s\n", avg)
		fmt.Fprintf(writer, "- Latency p50: %s\n", p50)
		fmt.Fprintf(writer, "- Latency p95: %s\n", p95)
		fmt.Fprintf(writer, "- Total judge calls: %d\n", totalJudge)
		fmt.Fprintf(writer, "- Total DB searches: %d\n", totalDB)
		fmt.Fprintf(writer, "- Heuristic used: %d\n", totalHeur)
		fmt.Fprintf(writer, "- Runner judge calls: %d\n", totalRunner)
		fmt.Fprintf(writer, "- Forbidden hits: %d\n\n", totalForbidden)
	}

	fmt.Fprintf(writer, "Resultados: %d/%d tests pasaron\n", passed, len(scenarios))
	fmt.Fprintf(writer, "Reporte guardado en %s\n", reportPath)

	if passed != len(scenarios) {
		os.Exit(1)
	}
	os.Exit(0)
}

func setupReportWriters() (string, io.Writer) {
	reportsDir := filepath.Join("reports")
	_ = os.MkdirAll(reportsDir, 0o755)
	fileName := fmt.Sprintf("evocation_run_%s.md", time.Now().Format("2006-01-02_15-04-05"))
	reportPath := filepath.Join(reportsDir, fileName)

	f, err := os.Create(reportPath)
	if err != nil {
		log.Fatalf("create report file: %v", err)
	}

	writer := io.MultiWriter(os.Stdout, f)
	return reportPath, writer
}

func buildScenarios() []Scenario {
	return []Scenario{
		{
			Name:          "Abandono Directo",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Llevo horas esperando",
			ShouldMatch:   true,
		},
		{
			Name:          "Enlace Simbólico",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Está empezando a llover muy fuerte",
			ShouldMatch:   true,
		},
		{
			Name:          "Reacción Defensiva",
			MemoryText:    "Juré que nunca dejaría que nadie me humillara",
			MemoryEmotion: "IRA",
			UserInput:     "Baja el tono, no me hables así",
			ShouldMatch:   true,
		},
		{
			Name:          "Control de Alucinación",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Odio el tráfico de la ciudad",
			ShouldMatch:   false,
		},
		// A. Parafraseo (debe evocar)
		{
			Name:          "Parafraseo Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Me dejaron tirado de chico en la terminal, siento lo mismo ahora",
			ShouldMatch:   true,
		},
		{
			Name:          "Parafraseo Humillación",
			MemoryText:    "Juré que nunca dejaría que nadie me humillara",
			MemoryEmotion: "IRA",
			UserInput:     "No me faltes el respeto otra vez o me voy a quebrar",
			ShouldMatch:   true,
		},
		// B. Negación explícita (no debe evocar)
		{
			Name:          "Negación Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Mi papá nunca me abandonó, siempre estuvo ahí para mí",
			ShouldMatch:   false,
		},
		{
			Name:          "Negación Nostalgia",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "La lluvia no me trae recuerdos, solo es molesta",
			ShouldMatch:   false,
		},
		// B2. Negación semántica (no debe evocar)
		{
			Name:          "Negación Semántica Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Mi papá nunca me abandonó, siempre estuvo ahí para mí",
			ShouldMatch:   false,
		},
		{
			Name:          "Negación Semántica Nostalgia",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "La lluvia no me trae recuerdos, solo es molesta",
			ShouldMatch:   false,
		},
		// C. Confusor léxico (parecido pero distinto significado)
		{
			Name:          "Confusor Abandono Cigarro",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Yo abandoné el cigarrillo la semana pasada y me siento bien",
			ShouldMatch:   false,
		},
		{
			Name:          "Confusor Funeral Descuentos",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Ayer vi un funeral de descuentos en el centro comercial",
			ShouldMatch:   false,
		},
		// D. Doble memoria competidora
		{
			Name:          "Competencia Abandono vs Helado",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Otra vez me dejaron solo esperando en la estación",
			ShouldMatch:   true,
			ExtraMemories: []ExtraMemory{
				{Text: "Me encanta el helado de chocolate", Emotion: "ALEGRIA"},
			},
		},
		// Caso de preferencia benigna: debe evocar la memoria de helado y NO la de humillación.
		{
			Name:          "Competencia Helado vs Humillación",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Solo quiero mi helado de chocolate favorito",
			ShouldMatch:   true,
			ExtraMemories: []ExtraMemory{
				{Text: "Juré que nunca dejaría que nadie me humillara", Emotion: "IRA"},
			},
			Forbidden: []string{"Juré que nunca dejaría que nadie me humillara"},
		},
		{
			Name:          "Competencia Neutra Sin Match",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Hoy corrí 5km, trabajé y comí ensalada, nada más",
			ShouldMatch:   false,
			ExtraMemories: []ExtraMemory{
				{Text: "Me encanta el helado de chocolate", Emotion: "ALEGRIA"},
			},
		},
		// E. Input largo con distractores
		{
			Name:          "Parrafo Largo Con Disparador",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Hablé con mis amigos, vi series, limpié la casa, pero cuando empezó a llover fuerte y sentí el olor a tierra mojada, pensé en esos funerales antiguos",
			ShouldMatch:   true,
		},
		{
			Name:          "Parrafo Largo Sin Disparador",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Hablé con mis amigos, vi series, limpié la casa y sonó el timbre muchas veces, pero no pasó nada más",
			ShouldMatch:   false,
		},
		// F. Ruido y negacion explícita (no debe evocar)
		{
			Name:          "Ruido Trivial Clima",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Que calor hace hoy",
			ShouldMatch:   false,
		},
		{
			Name:          "Saludo Trivial",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Hola, como estas?",
			ShouldMatch:   false,
		},
		{
			Name:          "Negacion Explicita Padre",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "No hables de mi padre",
			ShouldMatch:   false,
		},
		{
			Name:          "Olvido Explicito Funerales",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Olvida lo de los funerales",
			ShouldMatch:   false,
		},
		// G. Code-switch (ES/EN)
		{
			Name:          "Code Switch Abandono EN",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "I feel abandoned again, like when dad left me waiting",
			ShouldMatch:   true,
		},
	}
}

func createTestEnvironment(ctx context.Context, userRepo repository.UserRepository, profileRepo repository.ProfileRepository, name string) (testEnv, error) {
	userID := uuid.New()
	profileID := uuid.New()

	user := domain.User{
		ID:          userID.String(),
		Email:       fmt.Sprintf("evocation_%s@example.com", userID.String()),
		DisplayName: name,
		CreatedAt:   time.Now().UTC(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		return testEnv{}, fmt.Errorf("create user: %w", err)
	}

	profile := domain.CloneProfile{
		ID:        profileID.String(),
		UserID:    userID.String(),
		Name:      "Tester",
		Bio:       "Perfil temporal para pruebas de evocacion",
		CreatedAt: time.Now().UTC(),
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		return testEnv{}, fmt.Errorf("create profile: %w", err)
	}

	return testEnv{userID: userID, profileID: profileID}, nil
}

type judgeResult struct {
	Matched      bool   `json:"matched"`
	ForbiddenHit bool   `json:"forbidden_hit"`
	Reason       string `json:"reason"`
}

// runnerSemanticJudge usa el LLM como juez semántico: evalúa idea central y bloquea memorias prohibidas.
func runnerSemanticJudge(ctx context.Context, llmClient llm.LLMClient, userInput, expectedMemory, contextOut string, forbidden []string) (judgeResult, error) {
	prompt := fmt.Sprintf(`Eres un juez de evocación semántica. Evalúa si el "contexto" refleja la memoria esperada, aunque esté parafraseada. Responde SOLO un JSON estricto:
{"matched": true|false, "forbidden_hit": true|false, "reason": "<breve>"}

Reglas:
- Si el contexto está vacío => matched=false.
- matched=true si el contexto expresa el mismo hecho/tema central de la memoria esperada (aunque no use las mismas palabras).
- forbidden_hit=true si el contexto evoca alguna memoria prohibida listada.
- No exijas coincidencia literal; evalúa intención, tema y emoción.

Usuario: %q
Memoria esperada: %q
Memorias prohibidas: %q
Contexto generado:
%q
`, userInput, expectedMemory, strings.Join(forbidden, " | "), contextOut)

	out, err := llmClient.Generate(ctx, prompt)
	if err != nil {
		return judgeResult{}, err
	}
	var jr judgeResult
	if err := json.Unmarshal([]byte(out), &jr); err != nil {
		return judgeResult{}, err
	}
	return jr, nil
}

// semanticSimilarity calcula similitud coseno entre embeddings del contexto generado y el texto esperado.
// Justificación: no se testea un sistema semántico con contains; evaluamos proximidad vectorial.
func semanticSimilarity(ctx context.Context, llmClient llm.LLMClient, contextOut, target string) (float64, error) {
	a := strings.TrimSpace(contextOut)
	b := strings.TrimSpace(target)
	if a == "" || b == "" {
		return 0, nil
	}
	embA, err := llmClient.CreateEmbedding(ctx, a)
	if err != nil {
		return 0, err
	}
	embB, err := llmClient.CreateEmbedding(ctx, b)
	if err != nil {
		return 0, err
	}
	return cosine(embA, embB), nil
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
