package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

func main() {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	logger, _ := zap.NewExample()
	defer logger.Sync()

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
	characterRepo := repository.NewPgCharacterRepository(pool)
	memoryRepo := repository.NewPgMemoryRepository(pool)

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, logger)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, logger)
	testSvc := service.NewTestService(llmClient, analysisSvc, logger)
	contextSvc := service.NewBasicContextService(messageRepo)
	narrativeSvc := service.NewNarrativeService(characterRepo, memoryRepo, llmClient)
	cloneSvc := service.NewCloneService(llmClient, messageRepo, profileRepo, traitRepo, contextSvc, narrativeSvc, analysisSvc)

	user, err := ensureUser(ctx, pool, userRepo, "cli_test@example.com")
	if err != nil {
		log.Fatal(err)
	}

	for {
		fmt.Println("===== Director Mode =====")
		profiles, err := listProfiles(ctx, pool, user.ID)
		if err != nil {
			log.Fatalf("listar perfiles: %v", err)
		}
		if len(profiles) == 0 {
			fmt.Println("No hay perfiles. Crea uno nuevo.")
			newProfile, err := createProfileFlow(ctx, reader, profileRepo, user.ID)
			if err != nil {
				log.Fatalf("crear perfil: %v", err)
			}
			profiles = append(profiles, *newProfile)
		}

		fmt.Println("Perfiles disponibles:")
		for i, p := range profiles {
			fmt.Printf("[%d] %s (ID: %s)\n", i+1, p.Name, p.ID)
		}
		fmt.Println("[C] Crear nuevo perfil")
		fmt.Print("Selecciona un perfil: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		var selected domain.CloneProfile
		if strings.EqualFold(choice, "C") {
			newProfile, err := createProfileFlow(ctx, reader, profileRepo, user.ID)
			if err != nil {
				log.Fatalf("crear perfil: %v", err)
			}
			selected = *newProfile
		} else {
			idx, err := strconv.Atoi(choice)
			if err != nil || idx < 1 || idx > len(profiles) {
				fmt.Println("Seleccion invalida.")
				continue
			}
			selected = profiles[idx-1]
		}

		if selected.Big5.Openness == 0 && selected.Big5.Neuroticism == 0 {
			fmt.Println("\n--- Inicialización de Personalidad ---")
			fmt.Println("Parece que este perfil no tiene rasgos OCEAN iniciales.")
			fmt.Println("[T] Responder Test de Personalidad (Recomendado para un mejor comportamiento)")
			fmt.Println("[C] Continuar con la conversación (los rasgos se inferirán solo de la biografía)")
			fmt.Print("Selección [T/C]: ")

			selection, _ := reader.ReadString('\n')
			selection = strings.TrimSpace(strings.ToUpper(selection))

			if selection == "T" {
				runPersonalityTest(ctx, testSvc, selected.UserID, reader, logger)
				if refreshed, err := profileRepo.GetByUserID(ctx, selected.UserID); err == nil {
					selected = refreshed
				}
			}
		}

		if err := runActionsMenu(ctx, reader, selected, user, sessionRepo, messageRepo, traitRepo, characterRepo, narrativeSvc, cloneSvc); err != nil {
			log.Printf("error en menu: %v", err)
		}
	}
}

func runActionsMenu(
	ctx context.Context,
	reader *bufio.Reader,
	profile domain.CloneProfile,
	user domain.User,
	sessionRepo repository.SessionRepository,
	messageRepo repository.MessageRepository,
	traitRepo repository.TraitRepository,
	characterRepo repository.CharacterRepository,
	narrativeSvc *service.NarrativeService,
	cloneSvc *service.CloneService,
) error {
	for {
		fmt.Printf("\n--- Trabajando con: %s ---\n", strings.ToUpper(profile.Name))
		fmt.Println("[1] Chatear")
		fmt.Println("[2] Agregar Vinculo/Personaje")
		fmt.Println("[3] Sembrar Escenario/Recuerdo")
		fmt.Println("[4] Cambiar Clon")
		fmt.Println("[5] Salir")
		fmt.Print("Selecciona una opcion: ")

		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		switch line {
		case "1":
			if err := chatFlow(ctx, reader, profile, user, sessionRepo, messageRepo, cloneSvc); err != nil {
				fmt.Printf("Error en chat: %v\n", err)
			}
		case "2":
			if err := addCharacterFlow(ctx, reader, profile, narrativeSvc); err != nil {
				fmt.Printf("Error creando personaje: %v\n", err)
			} else {
				fmt.Println("Vinculo/personaje creado.")
			}
		case "3":
			if err := seedMemoryFlow(ctx, reader, profile, narrativeSvc); err != nil {
				fmt.Printf("Error sembrando escenario: %v\n", err)
			} else {
				fmt.Println("Escenario implantado. El clon ahora recordara esto al iniciar el chat.")
			}
		case "4":
			return nil
		case "5":
			os.Exit(0)
		default:
			fmt.Println("Opcion invalida.")
		}
	}
}

func chatFlow(ctx context.Context, reader *bufio.Reader, profile domain.CloneProfile, user domain.User, sessionRepo repository.SessionRepository, messageRepo repository.MessageRepository, cloneSvc *service.CloneService) error {
	session := domain.Session{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     uuid.NewString(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		return fmt.Errorf("crear sesion: %w", err)
	}

	fmt.Println("---- Modo Chat (escribe 'salir' para terminar chat) ----")
	for {
		fmt.Print("Tu > ")
		text, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("leer input: %w", err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if strings.EqualFold(text, "salir") || strings.EqualFold(text, "exit") {
			fmt.Println("Saliendo del chat...")
			return nil
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
			fmt.Printf("error guardando mensaje de usuario: %v\n", err)
			continue
		}

		cloneMsg, _, err := cloneSvc.Chat(ctx, user.ID, session.ID, text)
		if err != nil {
			fmt.Printf("error generando respuesta: %v\n", err)
			continue
		}
		fmt.Printf("%s > %s\n", profile.Name, cloneMsg.Content)
	}
}

func addCharacterFlow(ctx context.Context, reader *bufio.Reader, profile domain.CloneProfile, narrativeSvc *service.NarrativeService) error {
	fmt.Print("Nombre del personaje: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	fmt.Print("Relacion (ej: amigo, ex-pareja): ")
	rel, _ := reader.ReadString('\n')
	rel = strings.TrimSpace(rel)
	fmt.Print("Estado del vinculo (ej: estable, conflictivo): ")
	bondStatus, _ := reader.ReadString('\n')
	bondStatus = strings.TrimSpace(bondStatus)
	trust := readIntDefault(reader, "Confianza (0-100, default 50): ", 50)
	intimacy := readIntDefault(reader, "Intimidad (0-100, default 50): ", 50)
	respect := readIntDefault(reader, "Respeto (0-100, default 50): ", 50)

	profileUUID, err := uuid.Parse(profile.ID)
	if err != nil {
		return fmt.Errorf("parse profile id: %w", err)
	}

	return narrativeSvc.CreateRelation(ctx, profileUUID, name, rel, bondStatus, domain.RelationshipVectors{
		Trust:    trust,
		Intimacy: intimacy,
		Respect:  respect,
	})
}

func seedMemoryFlow(ctx context.Context, reader *bufio.Reader, profile domain.CloneProfile, narrativeSvc *service.NarrativeService) error {
	fmt.Print("Describe el escenario/recuerdo: ")
	content, _ := reader.ReadString('\n')
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("contenido vacio")
	}
	fmt.Print("Importancia (1-10, default 5): ")
	impStr, _ := reader.ReadString('\n')
	impStr = strings.TrimSpace(impStr)
	importance := 5
	if impStr != "" {
		if v, err := strconv.Atoi(impStr); err == nil {
			importance = v
		}
	}

	fmt.Print("Peso emocional (1-10, default igual a importancia): ")
	emoStr, _ := reader.ReadString('\n')
	emoStr = strings.TrimSpace(emoStr)
	emotionalWeight := importance
	if emoStr != "" {
		if v, err := strconv.Atoi(emoStr); err == nil {
			emotionalWeight = v
		}
	}

	fmt.Print("Etiqueta de sentimiento (Ira/Alegria/Miedo/etc, default Neutral): ")
	sentimentLabel, _ := reader.ReadString('\n')
	sentimentLabel = strings.TrimSpace(sentimentLabel)
	if sentimentLabel == "" {
		sentimentLabel = "Neutral"
	}

	profileUUID, err := uuid.Parse(profile.ID)
	if err != nil {
		return fmt.Errorf("parse profile id: %w", err)
	}

	emotionalIntensity := emotionalWeight * 10
	if emotionalIntensity < 1 {
		emotionalIntensity = 10
	}
	return narrativeSvc.InjectMemory(ctx, profileUUID, content, importance, emotionalWeight, emotionalIntensity, sentimentLabel)
}

func readIntDefault(reader *bufio.Reader, prompt string, def int) int {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	if v, err := strconv.Atoi(line); err == nil {
		return v
	}
	return def
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

func listProfiles(ctx context.Context, pool *pgxpool.Pool, userID string) ([]domain.CloneProfile, error) {
	const query = `
		SELECT id, user_id, name, bio, created_at
		FROM clone_profiles
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []domain.CloneProfile
	for rows.Next() {
		var p domain.CloneProfile
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Bio, &p.CreatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func createProfileFlow(ctx context.Context, reader *bufio.Reader, repo repository.ProfileRepository, userID string) (*domain.CloneProfile, error) {
	fmt.Print("Nombre del clon: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	fmt.Print("Bio: ")
	bio, _ := reader.ReadString('\n')
	bio = strings.TrimSpace(bio)

	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Bio:       bio,
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func runPersonalityTest(ctx context.Context, testSvc *service.TestService, userID string, reader *bufio.Reader, logger *zap.Logger) {
	questions := testSvc.GenerateInitialQuestions()
	responses := make(map[string]string)

	fmt.Println("\n--- TEST DE PERSONALIDAD OCEAN (5 Preguntas) ---")
	fmt.Println("Por favor, responde honestamente para inicializar la personalidad del clon.")

	for i, q := range questions {
		fmt.Printf("\n[%d/%d] %s: ", i+1, len(questions), q)
		input, _ := reader.ReadString('\n')
		responses[q] = strings.TrimSpace(input)
	}

	fmt.Println("\nAnalizando respuestas para inferir rasgos OCEAN. Por favor, espere...")

	if err := testSvc.AnalyzeTestResponses(ctx, userID, responses); err != nil {
		logger.Error("Error during test analysis", zap.Error(err), zap.String("user_id", userID))
		fmt.Println("ERROR: No se pudieron guardar los rasgos. Revise los logs.")
	} else {
		fmt.Println("\n✅ Personalidad inicial guardada con éxito.")
	}
}
