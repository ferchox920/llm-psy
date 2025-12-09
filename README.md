# LLM-Psy: Arquitectura de Personalidad Artificial con Permanencia Emocional

Motor de **Psique Digital** escrito en Go. Construye clones con memoria persistente, personalidad OCEAN, vínculos complejos y metas propias, separando la lógica determinista (Go) de la generación creativa (LLM).

## Visión
El objetivo es crear una entidad que:
- Mantenga **memoria episódica y emocional** con pesos e intensidades.
- Modele su **personalidad** con Big Five (OCEAN) y **vectores de relación** (Confianza, Intimidad, Respeto) que evolucionan.
- Ejercite **Agency** (metas) para dirigir la conversación según su estado emocional.
- Permanezca agnóstica al proveedor LLM (GPT/Claude/etc.) gracias a interfaces claras.

## Arquitectura Técnica
- **Clean Architecture**: capas `domain` (reglas puras), `repository` (persistencia), `service` (cognición/orquestación), `http` (handlers y router).
- **Cerebro LLM-agnóstico**: cliente `internal/llm` abstrae el modelo; se puede cambiar de GPT a Claude sin romper el dominio.
- **Persistencia**: PostgreSQL + pgvector para recuerdos vectoriales y metadata emocional.

## Modelo Psicológico (Core)
- **OCEAN / Big Five**: Apertura, Responsabilidad, Extroversión, Amabilidad, Neuroticismo.
- **Vectores Relacionales**: `Trust`, `Intimacy`, `Respect` (0-100). El tono se ajusta dinámicamente (p.ej., amor tóxico: alta intimidad + baja confianza = celos/paranoia).
- **Memoria Emocional**: Cada recuerdo guarda intensidad (1-100) y categoría (IRA, MIEDO, ALEGRÍA, etc.) para colorear el contexto.
- **Agency (Goal Service)**: El clon mantiene una `CurrentGoal` (p.ej., “interrogar”, “profundizar”, “sembrar duda/culpa”) derivada de estado emocional y vínculos; guía la respuesta sin exponer la meta.

## Instalación y Uso
Requisitos:
- Go 1.21+
- Docker (opcional, para DB)

Pasos rápidos:
```bash
cp .env.example .env   # configura tus claves LLM y DB
docker-compose up -d db   # opcional: levantar Postgres
go run ./cmd/api          # iniciar API HTTP
```

## Estructura del Proyecto
```
cmd/
  api/            # entrypoint del servidor HTTP
  coherence_check/ # harness de QA conversacional
internal/
  config/         # carga de variables
  domain/         # entidades puras: perfiles, rasgos, goals, memoria, vínculos
  repository/     # persistencia (Postgres, pgvector)
  service/        # lógica cognitiva: clone, narrative, analysis, goal
  http/           # handlers y router
  llm/            # cliente LLM agnóstico
```

## Comportamiento Clave
- **Resiliencia**: el clon atenúa insultos leves según su estabilidad (derivada de OCEAN), evitando reacciones desproporcionadas.
- **Relación Dinámica**: el prompt incorpora la matriz Confianza/Intimidad/Respeto para tonos profesionales, tóxicos o admirativos.
- **Agenda Oculta**: el prompt inyecta la meta actual y orienta la respuesta vía subtexto sin revelarla.

## Licencia
MIT (o la que definas).
