package app

import (
	"context"
	"go-rag/chat"
	"go-rag/config"
	"go-rag/ingest"
	"go-rag/llm"
	"go-rag/rag"
	"go-rag/vector"
	"go-rag/vector/pgvector"
	"go-rag/web"
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

	// get a retriever and a rewriter
	var retriever *rag.Retriever
	if store != nil {
		retriever = rag.New(embedder, store, rag.Options{
			TopK:     5,
			Rewriter: rag.NewRewriter(client),
		})
	}

	// Start our web server
	if cfg.HTTPAddr != "" {
		srv, err := web.New(client, embedder, retriever, web.Options{
			Addr:             cfg.HTTPAddr,
			SystemPromptFile: cfg.SystemPromptFile,
			Store:            store,
			ProcessDir:       cfg.ProcessedDir,
			ImagesDir:        cfg.ImageDir,
		})

		if err != nil {
			logger.Printf("web server disabled: %v", err)
		} else {
			wg.Go(func() {
				if err := srv.Run(ctx, cfg.HTTPAddr); err != nil {
					logger.Printf("web server stopped: %v", err)
				}
			})
			logger.Printf("web chat at http://localhost%s/chat", cfg.HTTPAddr)
		}
	}

	replErr := chat.RunREPL(ctx, client, retriever, chat.Options{
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
