package app

import (
	"context"
	"go-rag/chat"
	"go-rag/config"
	"go-rag/llm"
	"go-rag/vector"
	"go-rag/vector/pgvector"
	"log"
	"os"
)

func Run(ctx context.Context, cfg config.Config) error {
	logger := log.New(os.Stderr, "[rag] ", log.LstdFlags)

	// llm client
	client := llm.New(cfg)

	// Connect to database
	store, err := openStore(ctx, cfg)
	if err != nil {
		logger.Printf("vector store disabled: %v", err)
	}

	if store != nil {
		defer store.Close()
		logger.Printf("vector store ready...")
	}

	return chat.RunREPL(ctx, client, chat.Options{
		SystemPromptFile: cfg.SystemPromptFile,
	})
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
