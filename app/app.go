package app

import (
	"context"
	"go-rag/chat"
	"go-rag/config"
	"go-rag/ingest"
	"go-rag/llm"
	"go-rag/vector"
	"go-rag/vector/pgvector"
	"log"
	"os"
	"sync"
)

func Run(parent context.Context, cfg config.Config) error {
	logger := log.New(os.Stderr, "[rag] ", log.LstdFlags)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	// llm client
	client := llm.New(cfg)

	// embedding client
	embedder := llm.NewEmbedder(cfg)

	// Connect to database
	store, err := openStore(ctx, cfg)
	if err != nil {
		logger.Printf("vector store disabled: %v", err)
	}

	if store != nil {
		defer store.Close()
		logger.Printf("vector store ready...")
	}

	var wg sync.WaitGroup
	if store != nil {
		wg.Go(func() {
			opts := ingest.Options{
				SourceDir:    cfg.IngestDir,
				ProcessedDir: cfg.ProcessedDir,
			}

			if err := ingest.Watch(ctx, opts, embedder, store, logger); err != nil && ctx.Err() == nil {
				logger.Printf("watcher stopped: %v", err)
			}
			logger.Printf("watching %s for new documents", cfg.IngestDir)
		})
	}

	replErr := chat.RunREPL(ctx, client, chat.Options{
		SystemPromptFile: cfg.SystemPromptFile,
	})

	cancel()
	wg.Wait()
	return replErr
}

func openStore(ctx context.Context, cfg config.Config) (vector.Store, error) {
	if cfg.DatabaseURL == "" {
		return nil, nil
	}

	s, err := pgvector.New(ctx, pgvector.Options{
		DSN:          cfg.DatabaseURL,
		EmbeddingDim: cfg.EmbeddingDim,
	})

	if err != nil {
		return nil, err
	}
	return s, nil
}
