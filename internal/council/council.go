package council

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openjny/council/internal/copilot"
)

// PromptCallback is called when a prompt is sent to a model
type PromptCallback func(model, prompt, response string)

// PhaseCallback is called when a new phase starts
type PhaseCallback func(phase string, modelCount int)

// Config represents the configuration for the council
type Config struct {
	Models     []string
	Aggregator string
	Timeout    time.Duration
	Verbose    bool
	OriginalQ  string
}

// Review represents a model's review of other responses
type Review struct {
	ReviewerModel string
	Rankings      []Ranking
	Duration      time.Duration
	Error         error
}

// Ranking represents a model's ranking of an anonymized response
type Ranking struct {
	ResponseIndex int    // Index of the response being ranked
	Rank          int    // 1 = best, higher = worse
	Reasoning     string // Why this rank was given
}

// Result represents the final result from the council
type Result struct {
	ModelResponses      []copilot.Response
	Reviews             []Review
	AggregatedResponse  string
	AggregationDuration time.Duration
	ReviewDuration      time.Duration
	InitialPrompt       string // The question asked to models
	ReviewPrompts       map[string]string // Model -> review prompt
	AggregationPrompt   string // Final aggregation prompt
	Error               error
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
func (c *Council) Execute(ctx context.Context, question string, progressCallback copilot.ProgressCallback, phaseCallback PhaseCallback) Result {
	result := Result{
		InitialPrompt: question,
		ReviewPrompts: make(map[string]string),
	}

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

	// Step 2: Conduct peer review (each model reviews others' responses)
	if phaseCallback != nil {
		phaseCallback("review", successCount)
	}
	
	reviewStart := time.Now()
	result.Reviews = c.conductPeerReview(ctx, question, result.ModelResponses, progressCallback, &result)
	result.ReviewDuration = time.Since(reviewStart)

	// Step 3: Build aggregation prompt with review results
	aggregationPrompt := c.buildAggregationPrompt(question, result.ModelResponses, result.Reviews)
	result.AggregationPrompt = aggregationPrompt

	// Step 4: Ask aggregator model
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

// conductPeerReview asks each model to review and rank other models' responses
func (c *Council) conductPeerReview(ctx context.Context, question string, responses []copilot.Response, progressCallback copilot.ProgressCallback, result *Result) []Review {
	reviews := make([]Review, 0, len(responses))
	
	// Only review successful responses
	successfulResponses := make([]copilot.Response, 0)
	for _, resp := range responses {
		if resp.Error == nil && resp.Content != "" {
			successfulResponses = append(successfulResponses, resp)
		}
	}
	
	// If we have less than 2 successful responses, skip peer review
	if len(successfulResponses) < 2 {
		return reviews
	}
	
	// Each model reviews all OTHER responses
	for i, reviewer := range successfulResponses {
		// Build anonymized responses (exclude the reviewer's own response)
		anonymizedResponses := make([]copilot.Response, 0)
		for j, resp := range successfulResponses {
			if i != j {
				anonymizedResponses = append(anonymizedResponses, resp)
			}
		}
		
		reviewPrompt := c.buildReviewPrompt(question, anonymizedResponses)
		
		// Store the review prompt for verbose output
		if result != nil {
			result.ReviewPrompts[reviewer.Model] = reviewPrompt
		}
		
		// Get review from this model
		reviewContent, duration, err := c.client.AskSingleModel(
			ctx,
			reviewer.Model,
			reviewPrompt,
			c.config.Timeout,
		)
		
		if progressCallback != nil {
			progressCallback(reviewer.Model+" (review)", duration, err)
		}
		
		review := Review{
			ReviewerModel: reviewer.Model,
			Duration:      duration,
			Error:         err,
		}
		
		if err == nil {
			// Parse rankings from the review content
			// For simplicity, we'll store the raw review for now
			// In a production system, you'd parse structured rankings
			review.Rankings = c.parseRankings(reviewContent, len(anonymizedResponses))
		}
		
		reviews = append(reviews, review)
	}
	
	return reviews
}

// buildReviewPrompt creates the prompt for peer review
func (c *Council) buildReviewPrompt(question string, anonymizedResponses []copilot.Response) string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf(`You are an expert evaluator. Below are %d different responses to the question: "%s"

The responses are anonymized (labeled Response A, Response B, etc.).

`, len(anonymizedResponses), question))
	
	labels := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	for i, resp := range anonymizedResponses {
		if i < len(labels) {
			sb.WriteString(fmt.Sprintf("## Response %s:\n", labels[i]))
			sb.WriteString(resp.Content)
			sb.WriteString("\n\n")
		}
	}
	
	sb.WriteString(`Please evaluate these responses based on:
1. Accuracy of information
2. Depth of insight
3. Practical usefulness
4. Clarity and conciseness

Rank the responses from best to worst (1 = best) and explain your reasoning for each.
Format your response as:

Ranking:
1. Response [X]: [brief reasoning]
2. Response [Y]: [brief reasoning]
...

Be objective and focus on the quality of the content, not stylistic preferences.`)
	
	return sb.String()
}

// parseRankings extracts ranking information from review content
// This is a simplified parser - in production you'd want more robust parsing
func (c *Council) parseRankings(reviewContent string, numResponses int) []Ranking {
	rankings := make([]Ranking, 0)
	
	// For now, store a simple representation
	// A more sophisticated implementation would parse the actual rankings
	lines := strings.Split(reviewContent, "\n")
	labels := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	
	rank := 1
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for i, label := range labels {
			if i >= numResponses {
				break
			}
			if strings.Contains(line, "Response "+label) && (strings.Contains(line, fmt.Sprintf("%d.", rank)) || strings.Contains(line, fmt.Sprintf("%d:", rank))) {
				rankings = append(rankings, Ranking{
					ResponseIndex: i,
					Rank:          rank,
					Reasoning:     line,
				})
				rank++
				break
			}
		}
	}
	
	return rankings
}

// buildAggregationPrompt creates the prompt for the aggregator model with review results
func (c *Council) buildAggregationPrompt(originalQuestion string, responses []copilot.Response, reviews []Review) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are the Chairman of an AI Council. Multiple AI models have answered the following question, and then peer-reviewed each other's responses.

Original Question: "%s"

`, originalQuestion))

	// Show all responses
	sb.WriteString("## Council Members' Responses:\n\n")
	for i, resp := range responses {
		sb.WriteString(fmt.Sprintf("### Response %d - %s:\n", i+1, resp.Model))
		if resp.Error != nil {
			sb.WriteString(fmt.Sprintf("(Error: %v)\n\n", resp.Error))
		} else {
			sb.WriteString(resp.Content)
			sb.WriteString("\n\n")
		}
	}
	
	// Show peer review results
	if len(reviews) > 0 {
		sb.WriteString("## Peer Review Results:\n\n")
		sb.WriteString("Each model reviewed the others' responses. Here are their evaluations:\n\n")
		
		for _, review := range reviews {
			if review.Error == nil && len(review.Rankings) > 0 {
				sb.WriteString(fmt.Sprintf("**%s's Review:**\n", review.ReviewerModel))
				for _, ranking := range review.Rankings {
					sb.WriteString(fmt.Sprintf("- %s\n", ranking.Reasoning))
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString(`## Your Task as Chairman:

Based on the council members' responses AND their peer reviews:

1. Synthesize the BEST answer to the original question
2. Take a CLEAR, DECISIVE stance - avoid vague "it depends" answers
3. If there are multiple valid approaches, CHOOSE the best one and explain why
4. Provide ACTIONABLE recommendations
5. Support your decision with the strongest evidence from the responses

The council expects a definitive answer. Be confident in your conclusion.

Your final answer:`)

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
