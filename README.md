# go-rag

> A from-scratch **Retrieval-Augmented Generation (RAG)** chat app in Go: it embeds your documents and images into **PostgreSQL + pgvector**, and answers questions about them with cited sources, streamed token by token to a **terminal REPL** and a **web chat UI**.

It runs fully offline with [Ollama](https://ollama.com), or against any OpenAI-compatible API (OpenAI, Ollama Cloud, LM Studio, vLLM, …). You switch providers by editing one `.env` file.

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18-336791?logo=postgresql&logoColor=white)
![pgvector](https://img.shields.io/badge/pgvector-HNSW-6E4C9F)
![OpenAI compatible](https://img.shields.io/badge/API-OpenAI--compatible-412991)

---

## Table of Contents

- [Features](#features)
- [How It Works](#how-it-works)
- [Tech Stack](#tech-stack)
- [Prerequisites](#prerequisites)
- [Quick Start (5 minutes, fully local)](#quick-start-5-minutes-fully-local)
- [Configuration](#configuration)
- [Usage](#usage)
  - [Adding knowledge](#adding-knowledge)
  - [Terminal chat (REPL)](#terminal-chat-repl)
  - [Web chat](#web-chat)
  - [HTTP API](#http-api)
- [Project Structure](#project-structure)
- [Security: Prompt-Injection Defense](#security-prompt-injection-defense)
- [Customizing the Assistant](#customizing-the-assistant)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Live document ingestion.** Drop a `.txt` / `.md` file into `./documents` and a file watcher (`fsnotify`, debounced) picks it up. The file is chunked, embedded, stored in pgvector, and moved to `./documents/processed`.
- **Smart chunking.** Chunks are about 1000 characters with a 100-character overlap. The chunker prefers to split at paragraph breaks, then at sentence ends, then at word boundaries, so no chunk ends mid-thought.
- **Semantic search.** Retrieval uses cosine similarity over an **HNSW index** in PostgreSQL 18 with pgvector and returns the top-5 chunks with similarity scores.
- **Conversational query rewriting.** Follow-up questions ("what about *its* origin?") are rewritten by the LLM into standalone search queries before retrieval.
- **Grounded, cited answers.** Retrieved excerpts are injected into the prompt with their source filenames, and the model is told to cite them.
- **Multimodal knowledge.** You can upload images with a description, or let a **vision model auto-caption** them. The description is embedded, so the image can be retrieved and shown inline in the chat.
- **Two interfaces, one brain.**
  - A terminal REPL with a spinner and streamed output.
  - A web chat (Tailwind, Markdown rendering, DOMPurify sanitization) streaming over **Server-Sent Events**.
- **Prompt-injection defense.** An HTTP middleware scans chat messages, form fields, and uploaded documents for known jailbreak patterns and rejects them before they reach the model or the vector store.
- **Provider-agnostic.** All LLM calls go through a thin wrapper over the official `openai-go` SDK. Chat, embedding, and vision models can each point at a different endpoint.
- **Graceful degradation.** Without a database the app still runs as a plain chat. Without `HTTP_ADDR` it runs terminal-only. Without a vision model, captioning is skipped.
- **Clean shutdown.** `Ctrl+C` cancels one root `context` that stops the watcher, the web server, and in-flight LLM calls.

---

## How It Works

```
                        ┌──────────────────── INGESTION ────────────────────┐
  ./documents/*.md ──►  │ fsnotify watcher ─► chunk ─► embed ─► upsert      │
  POST /api/upload ──►  │                                     (pgvector)    │
  POST /api/upload/image│ image + description ─► embed ─► upsert            │
                        └───────────────────────────────────────────────────┘
                                                   │
                                                   ▼
                                     ┌──────────────────────────┐
                                     │ PostgreSQL 18 + pgvector  │
                                     │ documents(id, content,    │
                                     │   metadata, embedding)    │
                                     │ HNSW · cosine distance    │
                                     └──────────────────────────┘
                                                   ▲
                        ┌──────────────────── QUERY TIME ───────────────────┐
  user question  ──►    │ rewrite (LLM) ─► embed ─► top-K search ─► format  │
                        │ context + citations ─► stream completion (LLM)    │
                        └───────────────────────────────────────────────────┘
                                                   │
                                     terminal REPL  ◄┴►  web chat (SSE)
```

1. **Ingest.** Each document is split into overlapping chunks and embedded with the embedding model. Old chunks with the same source filename are deleted first, then the new ones are upserted, so re-adding a file replaces its old version.
2. **Rewrite.** On follow-up turns, the conversation is condensed into one standalone search query.
3. **Retrieve.** The query is embedded and the top 5 nearest chunks are fetched from pgvector.
4. **Augment.** The excerpts, each labeled with its source file and similarity score, are prepended to the user's latest message.
5. **Generate.** The chat model streams its answer back token by token.

---

## Tech Stack

| Layer            | Technology                                                                 |
| ---------------- | -------------------------------------------------------------------------- |
| Language         | Go 1.27 (standard library first)                                           |
| LLM SDK          | [`openai-go/v3`](https://github.com/openai/openai-go) for chat, embeddings, and vision |
| Vector database  | PostgreSQL 18 + [pgvector](https://github.com/pgvector/pgvector) (HNSW index) |
| DB driver        | [`pgx/v5`](https://github.com/jackc/pgx) + `pgvector-go`                   |
| HTTP router      | [`chi/v5`](https://github.com/go-chi/chi)                                  |
| File watching    | [`fsnotify`](https://github.com/fsnotify/fsnotify)                         |
| Config           | [`godotenv`](https://github.com/joho/godotenv)                             |
| Frontend         | Go `html/template` (embedded), Tailwind CSS, marked, DOMPurify             |
| Local models     | [Ollama](https://ollama.com): `gemma3`, `nomic-embed-text`, `mistral-small3.1` |

---

## Prerequisites

| Tool | Why | Check |
| --- | --- | --- |
| **Go 1.27+** | Builds and runs the app (version pinned in `go.mod`) | `go version` |
| **Docker** + Docker Compose | Runs PostgreSQL with pgvector | `docker compose version` |
| **Ollama** *(for local mode)* | Serves the chat, embedding, and vision models | `ollama --version` |

> Prefer a hosted API? Skip Ollama and put your provider's base URL and key in `.env` (see [Configuration](#configuration)).

---

## Quick Start (5 minutes, fully local)

**1. Clone the repo**

```sh
git clone https://github.com/AhmedRabea0302/go-rag-based-chat-app.git
cd go-rag-based-chat-app
```

**2. Start the vector database**

```sh
docker compose up -d
```

This starts `pgvector/pgvector:pg18` on `localhost:5432` with user, password, and database all set to `rag`. The app creates the `vector` extension, the `documents` table, and the HNSW index on first run, so no manual SQL is needed. (`create-table.sql` is kept for reference.)

**3. Pull the models**

```sh
ollama pull gemma3              # chat model
ollama pull nomic-embed-text    # embedding model (768 dimensions)
ollama pull mistral-small3.1    # optional: vision model for image auto-captioning
```

**4. Create a `.env` file** in the project root:

```env
# --- Chat model ---
OPENAI_BASE_URL=http://localhost:11434/v1
OPENAI_API_KEY=ollama                 # any non-empty value works for Ollama
OPENAI_MODEL=gemma3:latest
SYSTEM_PROMPT_FILE=./prompts/system-custom.md

# --- Vector store ---
DATABASE_URL=postgres://rag:rag@localhost:5432/rag?sslmode=disable

# --- Embeddings ---
EMBEDDING_BASE_URL=http://localhost:11434/v1
EMBEDDING_MODEL=nomic-embed-text
EMBEDDING_DIM=768

# --- Ingestion ---
INGEST_DIR=./documents
PROCESSED_DIR=./documents/processed

# --- Web UI ---
HTTP_ADDR=:8000

# --- Vision (optional) ---
VISION_MODEL=mistral-small3.1
```

**5. Run it**

```sh
go run ./cmd/rag
```

You should see:

```
[rag] vector store ready...
[rag] web chat at http://localhost:8000/chat
Chat session started. Type Q to quit

>
```

**6. Feed it knowledge and ask away**

```sh
echo "The Kappa is a water creature from Japanese folklore..." > documents/kappa.md
```

Then open **http://localhost:8000/chat**, or type into the terminal:

```
> What is a Kappa and where does it live?
```

---

## Configuration

All settings come from environment variables. A `.env` file in the working directory is loaded automatically (a missing file is ignored), and real environment variables also work.

| Variable | Default | Description |
| --- | --- | --- |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Any OpenAI-compatible chat endpoint. |
| `OPENAI_API_KEY` | *(empty)* | API key for the chat endpoint. |
| `OPENAI_MODEL` | `gpt-4o-mini` | Chat model used for answers **and** query rewriting. |
| `SYSTEM_PROMPT_FILE` | *(none)* | Path to a text file used as the system prompt. Ignored if missing. |
| `DATABASE_URL` | *(none)* | Postgres DSN. **If unset, RAG is disabled** and the app is a plain chatbot. |
| `EMBEDDING_BASE_URL` | same as `OPENAI_BASE_URL` | Embedding endpoint. Can differ from the chat endpoint. |
| `EMBEDDING_API_KEY` | same as `OPENAI_API_KEY` | Key for the embedding endpoint. |
| `EMBEDDING_MODEL` | `nomic-embed-text` | Embedding model name. |
| `EMBEDDING_DIM` | `768` | Vector size. **Must match the embedding model** (see Troubleshooting). |
| `INGEST_DIR` | `./documents` | Folder watched for new documents. |
| `PROCESSED_DIR` | `./documents/processed` | Where ingested files are moved. Must differ from `INGEST_DIR`. |
| `HTTP_ADDR` | *(none)* | Address for the web server, e.g. `:8000`. **If unset, there is no web UI.** |
| `Image_DIR` | `./documents/images` | Where uploaded images are stored and served from `/images/*`. *(The name is case-sensitive, as read in `config/config.go`.)* |
| `VISION_MODEL` | *(none)* | Vision-capable model used to auto-caption images. Uses the chat endpoint. |

### Using a hosted provider instead of Ollama

```env
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
EMBEDDING_MODEL=text-embedding-3-small
EMBEDDING_DIM=1536
```

You can also mix them: a hosted chat model with local embeddings, by setting `EMBEDDING_BASE_URL` separately.

> ⚠️ `.env` holds secrets and is git-ignored. Never commit it.

---

## Usage

### Adding knowledge

| Method | Supports | What happens |
| --- | --- | --- |
| **Drop a file** into `INGEST_DIR` | `.txt`, `.md`, `.markdown` | Picked up automatically (also on startup), ingested, moved to `PROCESSED_DIR`. |
| **Web UI → upload document** | `.txt`, `.md`, `.markdown` (≤ 10 MB) | Ingested immediately and archived in `PROCESSED_DIR`. |
| **Web UI → add image** | `.png`, `.jpg`, `.jpeg`, `.gif` (≤ 10 MB) | Saved to `Image_DIR`. Its description (typed or auto-captioned) is embedded so the image can be found and shown in answers. |

Re-ingesting a file with the same name **replaces** its previous chunks, so you can safely edit a document and drop it in again.

### Terminal chat (REPL)

Starts automatically with `go run ./cmd/rag`.

- Type a question and press Enter. The answer streams in.
- The augmented prompt (retrieved context + question) is printed to **stderr** under `Final Prompt:`, which helps when debugging retrieval. Hide it with `go run ./cmd/rag 2>/dev/null`.
- Quit with `q`, `quit`, `exit`, `/exit`, or `Ctrl+C`.

### Web chat

Open **http://localhost:8000/chat** (when `HTTP_ADDR=:8000`). It supports:

- Streaming answers rendered as sanitized Markdown
- Document upload
- Image upload with an optional ✨ auto-caption
- Inline display of retrieved images

### HTTP API

| Method | Route | Body | Response |
| --- | --- | --- | --- |
| `GET` | `/chat` | — | Web chat page |
| `POST` | `/api/chat/stream` | JSON `{"messages":[{"role":"user","content":"..."}]}` | SSE stream: `delta`, `error`, `done` events |
| `POST` | `/api/upload` | multipart, field `file` | `{"source","bytes","chunks"}` |
| `POST` | `/api/upload/image` | multipart, fields `image`, `description` | `{"source","image_path","description","bytes","chunks"}` |
| `POST` | `/api/caption` | multipart, field `image` *(only if `VISION_MODEL` set)* | `{"description"}` |
| `GET` | `/images/{file}` | — | Serves stored images |

The API is stateless: the client sends the whole conversation each time.

```sh
# Stream a chat answer
curl -N -X POST http://localhost:8000/api/chat/stream \
  -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"Tell me about leprechauns"}]}'

# Upload a document
curl -F "file=@notes.md" http://localhost:8000/api/upload

# Upload an image with a description
curl -F "image=@dragon.png" -F "description=A red dragon coiled on a mountain" \
  http://localhost:8000/api/upload/image
```

---

## Project Structure

```
go-rag/
├── cmd/rag/            # Entrypoint: signal-aware context → app.Run
├── app/                # Wiring: LLM clients, vector store, watcher, web server, REPL
├── config/             # .env + environment loading with defaults
├── llm/                # openai-go wrapper: ChatStream, Embed, DescribeImage (vision)
├── rag/                # Retriever, query rewriter, context/citation formatting
├── ingest/             # Chunker, file watcher, text + image ingestion pipelines
├── vector/             # Store interface (Upsert / Query / Delete / DeleteBySource)
│   └── pgvector/       # PostgreSQL + pgvector implementation, auto-migration
├── chat/               # Terminal REPL with spinner and streaming output
├── web/                # chi server, SSE streaming, uploads, injection middleware
│   └── templates/      # Embedded chat UI (chat.gohtml)
├── prompts/            # System prompt(s)
├── documents/          # Drop zone for ingestion (git-ignored)
├── docker-compose.yaml # PostgreSQL 18 + pgvector
└── create-table.sql    # Reference schema (the app migrates automatically)
```

**Design notes**

- `vector.Store` is an interface, so pgvector could be replaced by Qdrant, Weaviate, or an in-memory store without touching the RAG logic.
- SDK types stay inside `llm/`. The rest of the code only uses the provider-agnostic `llm.Message{Role, Content}`.
- Every long-running component takes a `context.Context`, so one cancellation shuts everything down cleanly.

---

## Security: Prompt-Injection Defense

All `/api/chat/stream` and `/api/upload*` requests go through the `InjectionDefense` middleware (`web/injection.go`). It scans the first 64 KB of:

- every **user** message in JSON chat requests,
- every multipart **form field** (e.g. image descriptions),
- every uploaded **text document**, so poisoned content never reaches the vector store,

and checks them for patterns such as *"ignore previous instructions"*, DAN or developer-mode jailbreaks, system-prompt extraction, safety-bypass phrasing, and ChatML or role-tag smuggling (`<|im_start|>`, `[INST]`, `<system>`). Matching requests are rejected with `400` and logged as `[web-defense]`.

Request bodies are size-capped: 1 MB for JSON and 10 MB for uploads. Model output is sanitized with DOMPurify before it is rendered in the browser.

> Pattern matching is one layer of defense, not a guarantee. Don't expose this server to the public internet without authentication.

---

## Customizing the Assistant

The bundled `prompts/system-custom.md` makes the assistant a **mythology and folklore expert** that refuses off-topic questions and only shows images when they match what was asked. To change its persona:

1. Write a new prompt file, e.g. `prompts/my-assistant.md`.
2. Set `SYSTEM_PROMPT_FILE=./prompts/my-assistant.md` in `.env`.
3. Restart the app.

If you want images to keep working, keep the section explaining how to turn `[image: /images/...]` tags into Markdown `![alt](/images/...)`.

---

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| `vector store disabled: ...connection refused` | Postgres isn't running. Run `docker compose up -d` and check `docker compose ps` shows it as healthy. |
| `expected 768 dimensions, not 1536` (or similar) | `EMBEDDING_DIM` doesn't match your embedding model or the existing table. Set the correct dim, then reset: `docker compose exec postgres psql -U rag -c "DROP TABLE documents;"` and re-ingest. |
| No web UI | `HTTP_ADDR` is not set. Add `HTTP_ADDR=:8000`. |
| Answers ignore my documents | Check that `DATABASE_URL` is set and the file was moved to `processed/`. Look at the `Final Prompt:` output on stderr to see what was retrieved. |
| File stays in `documents/` | Only `.txt`, `.md`, and `.markdown` are supported, and empty files are rejected. Check the `[rag] process ...` log line. |
| `request rejected by injection-defense filter` | Your message or document matched a jailbreak pattern. Rephrase it, or adjust `web/injection.go`. |
| Ollama `404 model not found` | Run `ollama pull <model>` and make sure the name in `.env` matches `ollama list`. |
| Port 5432 already in use | Another Postgres is running. Stop it, or change the port mapping in `docker-compose.yaml` and in `DATABASE_URL`. |

---

## Development

```sh
go build ./...          # build all packages
go vet ./...            # static analysis
go test ./...           # run tests
go run ./cmd/rag        # run the app

docker compose down     # stop the database (data is kept)
docker compose down -v  # stop it AND wipe all embeddings
```

---

## Roadmap

- [ ] Unit tests for the chunker, retriever, and injection filter
- [ ] PDF / DOCX ingestion
- [ ] Hybrid search (BM25 + vectors) and re-ranking
- [ ] Persistent conversation history
- [ ] Authentication for the web UI
- [ ] Dockerfile for the app itself

---

## Contributing

Contributions are welcome:

1. Fork the repo and create a branch: `git checkout -b feature/my-feature`
2. Make your changes and run `go vet ./... && go build ./...`
3. Commit with a clear message and open a pull request

---

## License

No license has been chosen yet. Until one is added, all rights are reserved by the author. If you want others to reuse the code, add a `LICENSE` file (MIT is a common choice).

---

<p align="center">Built with Go, pgvector, and a lot of folklore 🐉</p>
