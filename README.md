# 🧠 Clone LLM - Motor de Identidad Conversacional

Backend en Go diseñado para crear, entrenar y persistir clones conversacionales basados en perfiles psicológicos.

A diferencia de un chatbot estándar, este sistema construye un **Modelo de Identidad** dinámico. Analiza las interacciones del usuario en segundo plano, infiere rasgos de personalidad (usando modelos como Big Five) y adapta el comportamiento del clon para reflejar fielmente a su contraparte humana.

## 🚀 Estado del Proyecto

| Sprint | Módulo | Estado | Descripción |
| :--- | :--- | :--- | :--- |
| **01** | **Core Architecture** | ✅ Completado | Arquitectura limpia, persistencia PostgreSQL, Auth básica. |
| **02** | **Profile Engine** | ✅ Completado | Inferencia psicológica asíncrona, almacenamiento de rasgos (Big Five). |
| **03** | **Clone Voice** | ✅ Completado (MVP) | Generación de respuesta (RAG) con memoria a corto plazo e inyección de personalidad. |

## 🛠️ Stack Tecnológico

* **Lenguaje:** Go 1.23+
* **Framework Web:** Gin Gonic
* **Base de Datos:** PostgreSQL 15 (Driver `pgx/v5` con Pool)
* **IA / LLM:** Integración agnóstica (OpenAI/Anthropic)
* **Arquitectura:** Clean Architecture (Hexagonal)

## 📂 Estructura del Proyecto

El proyecto sigue una estructura estándar de Go para servicios escalables:

* `cmd/api`: Punto de entrada del servidor HTTP.
* `internal/config`: Gestión de configuración y variables de entorno.
* `internal/domain`: Definición de entidades (User, Profile, Trait, Message).
* `internal/http`: Capa de transporte (Handlers, Router, Middlewares).
* `internal/service`: Lógica de negocio (Orquestación de análisis, Clones).
* `internal/repository`: Capa de persistencia (Implementaciones SQL).
* `internal/llm`: Cliente y adaptadores para Modelos de Lenguaje.
* `pkg/logger`: Utilidades transversales (Logging estructurado con Zap).

## 🔌 API Endpoints

### Gestión de Usuarios & Clones
* `POST /users`: Crear un nuevo usuario.
* `POST /clone/init`: Inicializar el perfil de clon para un usuario.
* `GET /clone/profile?user_id={id}`: Obtener la radiografía psicológica del clon (Perfil + Rasgos).

### Chat & Sesión
* `POST /session`: Crear una sesión de chat efímera.
* `POST /message`: Enviar mensaje.
    * *Nota:* Este endpoint dispara el **AnalysisService** en segundo plano (Goroutine) para actualizar los rasgos del clon sin bloquear la respuesta.

## 🧠 Profile Engine (Motor de Psicología)

El sistema implementa un pipeline de análisis asíncrono:
1.  **Input:** Recibe texto del usuario.
2.  **Analysis:** Un agente LLM especializado ("El Psicólogo") analiza el texto buscando marcadores de personalidad.
3.  **Persistencia:** Actualiza o inserta valores en la tabla `traits` usando un modelo **Big Five** (Apertura, Responsabilidad, Extroversión, Amabilidad, Neuroticismo).
4.  **Evolución:** Los rasgos tienen un nivel de `confidence` y se ajustan con el tiempo (Upsert).

## ⚡ Guía de Inicio Rápido

### Requisitos
* Go 1.23+
* Docker & Docker Compose (para la DB)

### 1. Configuración
Clona el archivo de ejemplo y configura tu API Key de OpenAI (o compatible):
```bash
cp .env.example .env
# Edita .env y agrega tu LLM_API_KEY
````

### 2. Levantar Infraestructura

Inicia la base de datos PostgreSQL:

```bash
docker-compose up -d db
```

### 3. Ejecutar Migraciones

El servicio aplica migraciones manualmente o puedes usar la herramienta `migrate`:

```bash
docker-compose up migrate
```

### 4. Ejecutar Servidor

```bash
go run ./cmd/api
```

El servidor iniciará en el puerto `:8080`.

-----

**Autor:** Fernando Ramones
