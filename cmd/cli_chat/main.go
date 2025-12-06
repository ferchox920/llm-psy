package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

// Question define un item del cuestionario IPIP-20 adaptado.
type Question struct {
	ID        int
	Text      string
	Category  string
	Trait     string
	IsInverse bool
}

var questions = []Question{
	// EXTROVERSION
	{Text: "Soy el alma de la fiesta.", Trait: "extraversion", IsInverse: false},
	{Text: "No hablo mucho.", Trait: "extraversion", IsInverse: true},
	{Text: "Hablo con mucha gente distinta en las reuniones.", Trait: "extraversion", IsInverse: false},
	{Text: "Me mantengo en segundo plano.", Trait: "extraversion", IsInverse: true},

	// AMABILIDAD (AGREEABLENESS)
	{Text: "Simpatizo con los sentimientos de los demas.", Trait: "agreeableness", IsInverse: false},
	{Text: "No me interesan los problemas de otros.", Trait: "agreeableness", IsInverse: true},
	{Text: "Tengo un corazon blando.", Trait: "agreeableness", IsInverse: false},
	{Text: "Insulto a la gente.", Trait: "agreeableness", IsInverse: true},

	// RESPONSABILIDAD (CONSCIENTIOUSNESS)
	{Text: "Hago mis tareas de inmediato.", Trait: "conscientiousness", IsInverse: false},
	{Text: "Olvido poner las cosas en su sitio.", Trait: "conscientiousness", IsInverse: true},
	{Text: "Me gusta el orden.", Trait: "conscientiousness", IsInverse: false},
	{Text: "Hago desastres.", Trait: "conscientiousness", IsInverse: true},

	// ESTABILIDAD EMOCIONAL (NEUROTICISMO)
	{Text: "Tengo cambios de humor frecuentes.", Trait: "neuroticism", IsInverse: false},
	{Text: "Estoy relajado la mayor parte del tiempo.", Trait: "neuroticism", IsInverse: true},
	{Text: "Me altero facilmente.", Trait: "neuroticism", IsInverse: false},
	{Text: "Rara vez me siento triste.", Trait: "neuroticism", IsInverse: true},

	// APERTURA (OPENNESS)
	{Text: "Tengo una imaginacion vivida.", Trait: "openness", IsInverse: false},
	{Text: "No me interesan las ideas abstractas.", Trait: "openness", IsInverse: true},
	{Text: "Tengo dificultad para entender ideas abstractas.", Trait: "openness", IsInverse: true},
	{Text: "Estoy lleno de ideas.", Trait: "openness", IsInverse: false},
}

func main() {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	userRepo := repository.NewPgUserRepository(pool)
	profileRepo := repository.NewPgProfileRepository(pool)
	sessionRepo := repository.NewPgSessionRepository(pool)
	messageRepo := repository.NewPgMessageRepository(pool)
	traitRepo := repository.NewPgTraitRepository(pool)

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)
	contextSvc := service.NewBasicContextService(messageRepo)
	cloneSvc := service.NewCloneService(llmClient, messageRepo, profileRepo, traitRepo, contextSvc)

	user, isNew, err := ensureUser(ctx, pool, userRepo, "cli_test@example.com")
	if err != nil {
		log.Fatal(err)
	}

	profile, err := ensureProfile(ctx, profileRepo, user.ID)
	if err != nil {
		log.Fatal(err)
	}

	session := domain.Session{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     uuid.NewString(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		log.Fatal(err)
	}

	traits, err := traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		log.Printf("warning: could not load traits: %v", err)
	}

	if isNew || len(traits) == 0 {
		fmt.Println("Bienvenido. Realizaremos una breve bateria (IPIP-20) para calibrar la personalidad del clon.")
		traits, err = runQuestionnaire(ctx, reader, profile, traitRepo)
		if err != nil {
			log.Fatalf("cuestionario fallo: %v", err)
		}
	}

	printState(profile, traits)
	runChat(ctx, reader, profile, user, session, messageRepo, cloneSvc)
}

func runChat(ctx context.Context, reader *bufio.Reader, profile domain.CloneProfile, user domain.User, session domain.Session, messageRepo repository.MessageRepository, cloneSvc *service.CloneService) {
	for {
		fmt.Print("Tu > ")
		text, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("read input: %v", err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if strings.EqualFold(text, "salir") || strings.EqualFold(text, "exit") {
			fmt.Println("Saliendo...")
			return
		}

		userMsg := domain.Message{
			ID:        uuid.NewString(),
			UserID:    user.ID,
			SessionID: session.ID,
			Content:   text,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := messageRepo.Create(ctx, userMsg); err != nil {
			log.Printf("error saving user message: %v", err)
			continue
		}

		cloneMsg, err := cloneSvc.Chat(ctx, user.ID, session.ID, text)
		if err != nil {
			log.Printf("error generating clone response: %v", err)
			continue
		}

		fmt.Printf("%s > %s\n", profile.Name, cloneMsg.Content)
	}
}

func runQuestionnaire(ctx context.Context, reader *bufio.Reader, profile domain.CloneProfile, traitRepo repository.TraitRepository) ([]domain.Trait, error) {
	for i := range questions {
		questions[i].ID = i + 1
		questions[i].Category = domain.TraitCategoryBigFive
	}

	totals := make(map[string]int)
	counts := make(map[string]int)

	for _, q := range questions {
		for {
			fmt.Printf("[PREGUNTA %d/%d] %s\n", q.ID, len(questions), q.Text)
			fmt.Print("Responde del 1 (Totalmente en desacuerdo) al 5 (Totalmente de acuerdo): ")
			line, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			line = strings.TrimSpace(line)
			val, err := strconv.Atoi(line)
			if err != nil || val < 1 || val > 5 {
				fmt.Println("Entrada invalida, ingresa un numero entre 1 y 5.")
				continue
			}
			score := val
			if q.IsInverse {
				score = 6 - val
			}
			totals[q.Trait] += score
			counts[q.Trait]++
			fmt.Println()
			break
		}
	}

	now := time.Now().UTC()
	var traits []domain.Trait
	fmt.Println("Perfil calculado (previo a guardar):")
	for trait, sum := range totals {
		count := counts[trait]
		normalized := int(math.Round((float64(sum) / (float64(count) * 5.0)) * 100.0))
		description := interpretScore(normalized)
		fmt.Printf("- %s: %d%% (%s)\n", titleCase(trait), normalized, description)

		t := domain.Trait{
			ID:        uuid.NewString(),
			ProfileID: profile.ID,
			Category:  domain.TraitCategoryBigFive,
			Trait:     trait,
			Value:     normalized,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := traitRepo.Upsert(ctx, t); err != nil {
			return nil, fmt.Errorf("upsert trait %s: %w", trait, err)
		}
		traits = append(traits, t)
	}
	fmt.Println("Perfil guardado.")
	return traits, nil
}

func ensureUser(ctx context.Context, pool *pgxpool.Pool, repo repository.UserRepository, email string) (domain.User, bool, error) {
	const query = `
		SELECT id, email, display_name, created_at
		FROM users
		WHERE email = $1
	`

	var u domain.User
	err := pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err == nil {
		return u, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, false, err
	}

	u = domain.User{
		ID:        uuid.NewString(),
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, u); err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

func ensureProfile(ctx context.Context, repo repository.ProfileRepository, userID string) (domain.CloneProfile, error) {
	profile, err := repo.GetByUserID(ctx, userID)
	if err == nil {
		return profile, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.CloneProfile{}, err
	}

	profile = domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Clone Test",
		Bio:       "Soy un clon de prueba en la terminal.",
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, profile); err != nil {
		return domain.CloneProfile{}, err
	}
	return profile, nil
}

func printState(profile domain.CloneProfile, traits []domain.Trait) {
	fmt.Println("====================================")
	fmt.Printf("Clon: %s\n", profile.Name)
	if strings.TrimSpace(profile.Bio) != "" {
		fmt.Printf("Bio: %s\n", profile.Bio)
	}
	fmt.Println("Rasgos actuales:")
	if len(traits) == 0 {
		fmt.Println("- (sin rasgos aun)")
	} else {
		for _, t := range traits {
			if t.Confidence != nil {
				fmt.Printf("- %s: %d/100 (conf=%.2f)\n", titleCase(t.Trait), t.Value, *t.Confidence)
			} else {
				fmt.Printf("- %s: %d/100\n", titleCase(t.Trait), t.Value)
			}
		}
	}
	fmt.Println("====================================")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func interpretScore(score int) string {
	switch {
	case score < 40:
		return "Baja"
	case score < 60:
		return "Moderada"
	default:
		return "Alta"
	}
}
