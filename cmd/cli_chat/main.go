package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
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

func main() {
	ctx := context.Background()

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

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, nil)
	contextSvc := service.NewBasicContextService(messageRepo)
	cloneSvc := service.NewCloneService(llmClient, messageRepo, profileRepo, traitRepo, contextSvc)

	user, err := ensureUser(ctx, pool, userRepo, "cli_test@example.com")
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

	printState(profile, traits)

	reader := bufio.NewReader(os.Stdin)
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

func ensureUser(ctx context.Context, pool *pgxpool.Pool, repo repository.UserRepository, email string) (domain.User, error) {
	const query = `
		SELECT id, email, display_name, created_at
		FROM users
		WHERE email = $1
	`

	var u domain.User
	err := pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}

	u = domain.User{
		ID:        uuid.NewString(),
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, u); err != nil {
		return domain.User{}, err
	}
	return u, nil
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
