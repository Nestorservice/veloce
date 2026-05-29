# Veloce — Agent Intelligent de Migration Laravel → Go + Flutter

**Date :** 2026-05-29
**Statut :** Validé par l'utilisateur
**Portée :** Agent CLI autonome (ReAct pattern) écrit en Go, orchestrant la migration d'un monolithe Laravel (~800 fichiers, ~400 000 LOC) vers un backend Go REST + frontend Flutter multiplateforme.

---

## 1. Objectif

Construire `veloce`, un outil CLI en Go qui :
- Lit un projet Laravel local
- Orchestre deux modèles Gemini (Pro = Architecte, Flash = Ouvrier) selon un pipeline ReAct
- Génère un backend Go (Clean Architecture) et un frontend Flutter (Riverpod + feature-first)
- Garantit la cohérence des types entre Go et Dart via un index partagé
- Contrôle les coûts API via le Context Caching, la sortie stricte et un kill switch budgétaire
- Reprend exactement là où il s'est arrêté en cas d'interruption (checkpoint par fichier)

---

## 2. Architecture de l'Agent (le projet `veloce/`)

### 2.1 Structure des packages

```
veloce/
├── cmd/
│   └── veloce/
│       └── main.go              # Entrypoint CLI (cobra)
├── internal/
│   ├── agent/
│   │   ├── orchestrator.go      # Boucle principale, coordination des 4 phases
│   │   └── worker.go            # Worker ReAct : analyse → génère → compile → corrige
│   ├── gemini/
│   │   ├── architect.go         # Client Gemini Pro (appels rares, blocages complexes)
│   │   ├── worker_client.go     # Client Gemini Flash (traduction brute, 95% des appels)
│   │   └── cache.go             # Gestion Context Caching + compteur tokens
│   ├── state/
│   │   ├── migration_state.go   # Checkpoint par fichier (JSON persistant)
│   │   └── shared_types.go      # Index types Go/Dart (shared_types.json, mutex protégé)
│   ├── pipeline/
│   │   ├── compiler.go          # Exécution go build / go vet / dart analyze
│   │   └── corrector.go         # Boucle correction (2× Flash → Pro → alerte humaine, 4 tentatives max)
│   ├── scanner/
│   │   └── php_scanner.go       # Lecture, classification et tri par phase des fichiers PHP
│   └── output/
│       ├── go_writer.go         # Écriture fichiers Go générés
│       └── flutter_writer.go    # Écriture fichiers Dart/Flutter générés
├── configs/
│   └── default_rules.md         # Règles de migration immuables (uploadées dans le cache Gemini)
└── go.mod
```

### 2.2 Flux principal

```
CLI args
  → Scanner        : lire + classifier les 800 fichiers PHP par phase
  → Phase 1        : Infrastructure (config DB, routeur, middlewares, auth JWT)
  → Phase 2        : Modèles/Repositories (sans dépendances inter-fichiers)
  → Phase 3        : Services + Controllers + Routes API
  → Phase 4        : UI — fichiers Blade → Widgets Flutter
  → Rapport final  : N fichiers traités, M échoués, tokens consommés
```

Chaque phase est traitée par un **worker pool** de goroutines (taille configurable via `--workers`, défaut : 5). Les phases sont strictement séquentielles (Phase N+1 ne démarre qu'après Phase N complète) pour garantir les dépendances.

---

## 3. Interface CLI

### Commandes

```bash
# Migration complète
veloce migrate \
  --source  ./mon-projet-laravel \
  --output  ./output \
  --workers 8 \
  --budget  5000000 \
  --api-key $GEMINI_API_KEY

# Reprise après interruption
veloce migrate --source ./mon-projet-laravel --output ./output --resume

# État d'avancement
veloce status --output ./output

# Forcer le re-traitement d'un fichier spécifique
veloce retry --output ./output --file app/Models/User.php
```

### Flags globaux

| Flag | Type | Défaut | Description |
|------|------|--------|-------------|
| `--source` | string | — | Chemin vers le projet Laravel (obligatoire) |
| `--output` | string | `./output` | Répertoire de sortie (monorepo) |
| `--workers` | int | 5 | Taille du worker pool par phase |
| `--budget` | int | 5000000 | Budget tokens (kill switch) |
| `--api-key` | string | `$GEMINI_API_KEY` | Clé API Google AI Studio |
| `--resume` | bool | false | Reprendre depuis le dernier checkpoint |
| `--dry-run` | bool | false | Simuler sans appels API ni écriture |
| `--run-tests` | bool | false | Exécuter go test / flutter test après chaque génération |

---

## 4. Gestion d'État (Persistance)

Tous les fichiers d'état sont stockés dans `./output/.veloce/` :

### `migration_state.json`
```json
{
  "session_id": "2026-05-29T14:32:00Z",
  "total_files": 800,
  "current_phase": 2,
  "files": {
    "app/Models/User.php": {
      "status": "done",
      "phase": 2,
      "output": "backend/internal/domain/user.go",
      "attempts": 1
    },
    "app/Models/Product.php": {
      "status": "failed",
      "phase": 2,
      "attempts": 3,
      "error": "escalated_to_human",
      "last_error": "undefined: uuid.New"
    },
    "app/Http/Controllers/AuthController.php": {
      "status": "pending",
      "phase": 3
    }
  }
}
```

Statuts possibles : `pending` → `processing` → `done` | `failed`

### `shared_types.json`
Index des signatures de types générés, protégé par un mutex (lectures/écritures concurrentes).
```json
{
  "go_types": {
    "User": { "package": "domain", "file": "user.go", "fields": ["ID uuid.UUID", "Email string", "CreatedAt time.Time"] },
    "Product": { "package": "domain", "file": "product.go", "fields": ["ID uuid.UUID", "Name string", "Price float64"] }
  },
  "dart_types": {
    "UserModel": { "file": "features/auth/domain/user_model.dart", "fields": ["String id", "String email", "DateTime createdAt"] }
  }
}
```

### `token_usage.json`
```json
{
  "flash_input_tokens": 1240000,
  "flash_output_tokens": 980000,
  "pro_input_tokens": 45000,
  "pro_output_tokens": 12000,
  "cached_tokens_saved": 3200000,
  "total_tokens": 2277000,
  "budget_limit": 5000000
}
```

### `context_cache_id.txt`
ID du cache Gemini créé au démarrage. Réutilisé pour toutes les requêtes Flash de la session.

---

## 5. Intégration Gemini

### 5.1 Répartition des rôles

| Modèle | Alias | % appels | Rôle |
|--------|-------|----------|------|
| `gemini-2.5-pro` | Architecte | ~5% | Analyse haut niveau, cartographie dépendances, arbitrage blocages |
| `gemini-2.5-flash` | Ouvrier | ~95% | Traduction PHP→Go/Dart, génération tests, application patchs |

### 5.2 Context Caching

Au démarrage (ou si le cache est expiré), l'agent :
1. Upload `default_rules.md` + `shared_types.json` vers l'API Gemini
2. Reçoit un `cache_id` → stocké dans `context_cache_id.txt`
3. Toutes les requêtes Flash incluent `cached_content: cache_id` (économie ~75% tokens d'entrée)

Le cache est **rafraîchi** après chaque phase pour inclure les nouveaux types dans `shared_types.json`.

### 5.3 Structure des prompts Flash (Zéro Blabla)

```
[SYSTEM — via cache]
Tu es un expert en migration PHP → Go/Dart.
Règles : {default_rules.md}
Types existants : {shared_types.json}

[USER]
Traduis le fichier PHP suivant.
- Cible : {go|dart}
- Architecture cible : {instructions spécifiques à la phase}
- Fichier source :
```{php_source_code}```

Réponds avec UNIQUEMENT le code {Go|Dart} valide. Aucune explication, aucun markdown.
```

`response_mime_type` est fixé à `text/plain` pour maximiser la pureté du code retourné.

---

## 6. Boucle ReAct du Worker

```
FICHIER php_source → Worker goroutine
│
├─ GÉNÉRATION (tentative 1) : Gemini Flash → code Go/Dart
│
├─ VERIFY : go build ./... | dart analyze
│   ├── OK  → extraire signatures → shared_types.json → status=done
│   └── ERR → capturer stderr exact
│       ├─ CORRECTION 1 (tentative 2) : Flash + [code erroné + stderr] → patch
│       ├─ CORRECTION 2 (tentative 3) : Flash (idem)
│       ├─ ESCALADE    (tentative 4) : Architecte Pro + contexte complet
│       └─ ÉCHEC       (tentative 5) : status=failed, alerte console, log stderr complet
│
└─ Mise à jour migration_state.json (atomic write)
```

**Limite stricte :** 1 génération initiale + 2 corrections Flash + 1 tentative Pro = 4 tentatives maximum avant escalade humaine. L'agent ne boucle jamais indéfiniment. Le champ `attempts` dans `migration_state.json` reflète ce compteur (1–4).

---

## 7. FinOps — Quatre Mécanismes de Contrôle

### 7.1 Context Caching
Voir §5.2. Économie estimée : 70–80% des tokens d'entrée Flash.

### 7.2 Zero Blabla
Chaque prompt se termine par `Respond with ONLY valid {Go|Dart} code.` + `response_mime_type: text/plain`. Aucun token de sortie superflu.

### 7.3 Kill Switch budgétaire
Avant chaque appel API :
```go
if tokenUsage.Total() >= config.BudgetLimit {
    logger.Fatal("[BUDGET EXHAUSTED] %d/%d tokens. Session sauvegardée. Relancez avec --resume.",
        tokenUsage.Total(), config.BudgetLimit)
}
```

### 7.4 Comptabilité différenciée
Flash et Pro sont comptés séparément dans `token_usage.json`. Le CLI `veloce status` affiche le coût estimé en USD (Flash : $0.075/1M tokens, Pro : $1.25/1M tokens).

---

## 8. Outils de Vérification Locale

| Outil | Cible | Commande |
|-------|-------|----------|
| `go build ./...` | Go | Compilation complète du module généré |
| `go vet ./...` | Go | Analyse statique (bugs courants) |
| `dart analyze` | Flutter | Analyse statique Dart |
| `go test ./...` | Go | Tests unitaires (mode optionnel, activé via `--run-tests`) |
| `flutter test` | Flutter | Tests Flutter (mode optionnel, activé via `--run-tests`) |

Les tests sont désactivés par défaut pour maximiser le débit. Ils peuvent être activés pour des vérifications approfondies.

---

## 9. Structure du Code Généré

### 9.1 Backend Go (`output/backend/`)

```
backend/
├── cmd/api/main.go                  # Entrypoint HTTP
├── internal/
│   ├── handler/                     # Couche HTTP — Chi router, middlewares (logging, recovery, CORS)
│   │   └── {resource}_handler.go    # Un handler par ressource Laravel
│   ├── service/                     # Logique métier — traduit depuis les Service PHP
│   │   └── {resource}_service.go
│   ├── repository/                  # Accès données — GORM, traduit depuis Eloquent
│   │   └── {resource}_repository.go
│   └── domain/                      # Structs métier — traduit depuis les Models PHP
│       └── {resource}.go
├── pkg/
│   ├── auth/                        # JWT (golang-jwt/jwt) + bcrypt
│   ├── middleware/                  # Middlewares réutilisables
│   └── validator/                   # go-playground/validator
├── migrations/                      # SQL migrations (traduit depuis Laravel migrations)
└── go.mod
```

**Conventions Go :**
- Pas de `panic()` dans le code métier
- Pattern `if err != nil { return fmt.Errorf("context: %w", err) }` systématique
- Structs de validation sur tous les handlers (`binding:"required,email"`)

### 9.2 Frontend Flutter (`output/frontend/`)

```
frontend/
├── lib/
│   ├── main.dart
│   ├── core/
│   │   ├── network/                 # Client Dio + intercepteur JWT auto-refresh
│   │   └── router/                  # GoRouter — traduit depuis les routes Laravel
│   ├── features/                    # Architecture feature-first
│   │   └── {feature}/               # Un dossier par feature Laravel (auth, products, etc.)
│   │       ├── data/
│   │       │   ├── dto/             # DTOs JSON (alignés sur les structs Go)
│   │       │   └── repository/      # Appels API Dio
│   │       ├── domain/
│   │       │   └── {feature}_model.dart  # Modèles Dart (alignés sur shared_types.json)
│   │       └── presentation/
│   │           ├── providers/       # Riverpod providers (état)
│   │           └── screens/         # Widgets Flutter (traduits depuis Blade)
│   └── shared/
│       └── widgets/                 # Composants réutilisables
└── pubspec.yaml
```

**Conventions Flutter :**
- Aucune dépendance plateforme-spécifique (compile identique Web/Desktop/Mobile)
- Layout CSS/Tailwind → Flutter : `div flex` → `Row`, `div block` → `Column`, etc.
- Tous les appels réseau passent par le client Dio centralisé (jamais `http.get()` direct)

---

## 10. Scanner PHP — Classification par Phase

Le scanner lit le projet Laravel et classe chaque fichier dans l'une des 4 phases :

| Phase | Patterns détectés | Exemples |
|-------|-------------------|----------|
| 1 — Infrastructure | `config/`, `database/migrations/`, `routes/`, middleware de base | `config/database.php`, `routes/api.php` |
| 2 — Modèles | `app/Models/`, `app/Repositories/` | `app/Models/User.php` |
| 3 — Services/Ctrl | `app/Http/Controllers/`, `app/Services/`, `app/Http/Requests/` | `AuthController.php` |
| 4 — UI | `resources/views/` (`.blade.php`) | `auth/login.blade.php` |

Les fichiers non reconnus sont loggés dans `output/.veloce/unknown_files.txt` pour review humaine.

---

## 11. Contraintes Non-Fonctionnelles

- **Reprise :** Relancer avec `--resume` ne re-traite jamais un fichier au statut `done`
- **Atomicité :** Les écritures dans `migration_state.json` et `shared_types.json` sont atomiques (write-to-temp + rename)
- **Concurrence :** `shared_types.json` est protégé par un `sync.RWMutex`
- **Observabilité :** Chaque worker log en temps réel : `[Phase 2][Worker 3] ✓ app/Models/User.php → domain/user.go (1 attempt, 1240 tokens)`
- **Portabilité :** L'agent compile et tourne sur Linux, macOS et Windows sans dépendance externe au binaire Gemini CLI

---

## 12. Hors Portée (Explicitement Exclus)

- Parsing AST complet du PHP (le scanner classe par chemin/pattern, pas par AST)
- Migration des jobs/queues Laravel (Horizon, Redis)
- Migration des tests PHP existants (les tests Go/Flutter sont générés ex nihilo)
- Déploiement du code généré (CI/CD est hors scope)
- Interface graphique (CLI uniquement)
