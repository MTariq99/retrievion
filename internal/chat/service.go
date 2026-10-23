package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/citation"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/conversation"
	"github.com/mtariq99/retrievion/internal/embedding"
	"github.com/mtariq99/retrievion/internal/llm"
	"github.com/mtariq99/retrievion/internal/prompt"
	"github.com/mtariq99/retrievion/internal/retrieval"
	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/models"
)

type chatMutator interface {
	CreateMessage(ctx context.Context, conversationID uuid.UUID, role consts.MessageRole, content string) (*models.Message, error)
	CreateMessageSources(ctx context.Context, messageID uuid.UUID, sources []models.MessageSource) error
}

type chatReader interface {
	GetMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]*models.Message, error)
}

type Service struct {
	chatMutator         chatMutator
	chatReader          chatReader
	conversationService *conversation.Service
	retrieval           *retrieval.Service
	prompt              *prompt.Builder
	citation            *citation.Service
	embedding           *embedding.Service
	llm                 *llm.Service
	logger              *zap.Logger
	config              *models.Config
	tokenizer           tokenizer.Counter
}

func NewService(repo *Repository, conversationService *conversation.Service, retrieval *retrieval.Service, promptBuilder *prompt.Builder, citationService *citation.Service, embeddingService *embedding.Service, llmService *llm.Service, logger *zap.Logger, config *models.Config, tokenizer tokenizer.Counter) *Service {
	return &Service{
		chatMutator:         repo,
		chatReader:          repo,
		conversationService: conversationService,
		retrieval:           retrieval,
		prompt:              promptBuilder,
		citation:            citationService,
		embedding:           embeddingService,
		llm:                 llmService,
		logger:              logger,
		config:              config,
		tokenizer:           tokenizer,
	}
}

func (s *Service) Chat(ctx context.Context, userID, tenantID uuid.UUID, req *models.ChatRequest) (*models.ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request is required")
	}

	if userID == uuid.Nil {
		return nil, fmt.Errorf("user ID is required")
	}

	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant ID is required")
	}

	if req.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation ID is required")
	}

	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	conv, err := s.conversationService.GetConversation(ctx, userID, tenantID, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	_, err = s.chatMutator.CreateMessage(ctx, req.ConversationID, consts.MessageRoleUser, req.Message)
	if err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}
	query := req.Message

	embeddings, err := s.embedding.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("embedding provider returned no embedding")
	}

	retrievalQuery := models.RetrievalQuery{
		TenantID:       tenantID,
		UserID:         userID,
		Query:          query,
		QueryEmbedding: embeddings[0],
	}
	results, err := s.retrieval.Retrieve(ctx, retrievalQuery)
	if err != nil {
		return nil, fmt.Errorf("retrieve context: %w", err)
	}

	contextText := s.buildBoundedContext(conv, query, results)

	generateRequest, err := s.prompt.Build(conv, query, contextText)
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	result, err := s.llm.Generate(ctx, generateRequest.Prompt)
	if err != nil {
		return nil, fmt.Errorf("generate answer: %w", err)
	}

	answer := result.Text

	if answer == "" {
		return nil, fmt.Errorf("LLM returned an empty response")
	}

	sources, messageSources, err := s.citation.Resolve(ctx, result.Text, results)
	if err != nil {
		return nil, err
	}

	assistantMessage, err := s.chatMutator.CreateMessage(ctx, req.ConversationID, consts.MessageRoleAssistant, result.Text)
	if err != nil {
		return nil, err
	}

	for i := range messageSources {
		messageSources[i].MessageID = assistantMessage.ID
	}

	if err := s.chatMutator.CreateMessageSources(ctx, assistantMessage.ID, messageSources); err != nil {
		return nil, err
	}

	return &models.ChatResponse{
		ConversationID: req.ConversationID,
		MessageID:      assistantMessage.ID,
		Answer:         answer,
		Sources:        sources,
	}, nil
}

func (s *Service) StreamChat(ctx context.Context, userID, tenantID uuid.UUID, req *models.ChatRequest, onToken func(string) error) (*models.ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request is required")
	}

	if userID == uuid.Nil {
		return nil, fmt.Errorf("user ID is required")
	}

	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant ID is required")
	}

	if req.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation ID is required")
	}

	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	conv, err := s.conversationService.GetConversation(ctx, userID, tenantID, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	_, err = s.chatMutator.CreateMessage(ctx, req.ConversationID, consts.MessageRoleUser, req.Message)
	if err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}

	embeddings, err := s.embedding.Embed(ctx, []string{req.Message})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("embedding provider returned no embedding")
	}

	retrievalQuery := models.RetrievalQuery{
		TenantID:       tenantID,
		UserID:         userID,
		Query:          req.Message,
		QueryEmbedding: embeddings[0],
	}
	results, err := s.retrieval.Retrieve(ctx, retrievalQuery)
	if err != nil {
		return nil, fmt.Errorf("retrieve context: %w", err)
	}

	contextText := s.buildBoundedContext(conv, req.Message, results)

	generateRequest, err := s.prompt.Build(conv, req.Message, contextText)
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	events := s.llm.Stream(ctx, generateRequest.Prompt)

	var answer strings.Builder

	for event := range events {
		if event.Err != nil {
			return nil, fmt.Errorf("LLM stream: %w", event.Err)
		}
		if event.Content != "" {
			answer.WriteString(event.Content)
			if onToken != nil {
				if err := onToken(event.Content); err != nil {
					return nil, fmt.Errorf("send token: %w", err)
				}
			}
		}
	}

	finalAnswer := answer.String()

	if finalAnswer == "" {
		return nil, fmt.Errorf("LLM returned an empty response")
	}

	sources, messageSources, err := s.citation.Resolve(ctx, finalAnswer, results)
	if err != nil {
		return nil, fmt.Errorf("resolve citations: %w", err)
	}

	assistantMessage, err := s.chatMutator.CreateMessage(ctx, req.ConversationID, consts.MessageRoleAssistant, finalAnswer)
	if err != nil {
		return nil, fmt.Errorf("save assistant message: %w", err)
	}

	for i := range messageSources {
		messageSources[i].MessageID = assistantMessage.ID
	}

	if err := s.chatMutator.CreateMessageSources(ctx, assistantMessage.ID, messageSources); err != nil {
		return nil, fmt.Errorf("save message sources: %w", err)
	}

	return &models.ChatResponse{
		ConversationID: req.ConversationID,
		MessageID:      assistantMessage.ID,
		Answer:         finalAnswer,
		Sources:        sources,
	}, nil
}

func (s *Service) buildBoundedContext(
	conv *models.Conversation,
	query string,
	results []models.RetrievedChunk,
) string {
	overhead := s.prompt.SystemPromptTokens()

	if conv.Summary != nil {
		overhead += s.tokenizer.Count(*conv.Summary)
	}

	overhead += s.tokenizer.Count(query)

	maxContextTokens := s.config.LLM.MaxContextTokens - overhead
	if maxContextTokens < 0 {
		maxContextTokens = 0
	}

	contextText, dropped := BuildContext(
		results,
		maxContextTokens,
		s.tokenizer,
	)

	if dropped > 0 {
		s.logger.Warn(
			"context truncated to fit token budget",
			zap.Int("chunks_dropped", dropped),
			zap.Int("chunks_included", len(results)-dropped),
			zap.Int("max_context_tokens", maxContextTokens),
		)
	}

	return contextText
}
