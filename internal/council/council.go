package council

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openjny/council/internal/copilot"
)

// Config represents the configuration for the council
type Config struct {
	Models      []string
	Aggregator  string
	Timeout     time.Duration
	Verbose     bool
	OriginalQ   string
}

// Result represents the final result from the council
type Result struct {
	ModelResponses []copilot.Response
	AggregatedResponse string
	AggregationDuration time.Duration
	Error error
}

// Council orchestrates multiple AI models and aggregates their responses
type Council struct {
	client *copilot.Client
	config Config
}

// NewCouncil creates a new council instance
func NewCouncil(config Config) (*Council, error) {
	client, err := copilot.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Copilot client: %w", err)
	}

	return &Council{
		client: client,
		config: config,
	}, nil
}

// Close releases resources
func (c *Council) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Execute runs the council pattern: ask multiple models, then aggregate
func (c *Council) Execute(ctx context.Context, question string, progressCallback copilot.ProgressCallback) Result {
	result := Result{}

	// Step 1: Ask all models in parallel
	result.ModelResponses = c.client.AskMultipleModels(
		ctx,
		c.config.Models,
		question,
		c.config.Timeout,
		progressCallback,
	)

	// Check if we got at least one successful response
	successCount := 0
	for _, resp := range result.ModelResponses {
		if resp.Error == nil && resp.Content != "" {
			successCount++
		}
	}

	if successCount == 0 {
		result.Error = fmt.Errorf("all models failed to respond")
		return result
	}

	// Step 2: Build aggregation prompt
	aggregationPrompt := c.buildAggregationPrompt(question, result.ModelResponses)

	// Step 3: Ask aggregator model
	aggregated, duration, err := c.client.AskSingleModel(
		ctx,
		c.config.Aggregator,
		aggregationPrompt,
		c.config.Timeout,
	)
	if err != nil {
		result.Error = fmt.Errorf("aggregation failed: %w", err)
		return result
	}

	result.AggregatedResponse = aggregated
	result.AggregationDuration = duration
	return result
}

// buildAggregationPrompt creates the prompt for the aggregator model
func (c *Council) buildAggregationPrompt(originalQuestion string, responses []copilot.Response) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("以下は同じ質問「%s」に対する複数のAIモデルからの回答です：\n\n", originalQuestion))

	for _, resp := range responses {
		sb.WriteString(fmt.Sprintf("## %s の回答:\n", resp.Model))
		if resp.Error != nil {
			sb.WriteString(fmt.Sprintf("(エラー: %v)\n\n", resp.Error))
		} else {
			sb.WriteString(resp.Content)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString(`これらの回答を分析し、以下の観点で最も正確で包括的な最終回答を作成してください：
1. 共通している重要なポイント
2. 各モデル特有の洞察
3. 矛盾がある場合はその調整
4. 最も信頼できる情報の選択

最終回答を提供してください。回答は日本語で、明確で簡潔にまとめてください。`)

	return sb.String()
}

// DefaultModels returns the default set of models to use
func DefaultModels() []string {
	return []string{
		"claude-sonnet-4.5",
		"gpt-5.2",
		"gemini-3-pro-preview",
	}
}

// DefaultAggregator returns the default aggregator model
func DefaultAggregator() string {
	return "gpt-4.1"
}
