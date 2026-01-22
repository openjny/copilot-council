package copilot

import (
	"context"
	"fmt"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

// Client wraps the Copilot SDK client
type Client struct {
	client *copilot.Client
	mu     sync.Mutex
}

// NewClient creates a new Copilot client wrapper
func NewClient() (*Client, error) {
	client := copilot.NewClient(&copilot.ClientOptions{
		LogLevel: "error",
	})

	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Copilot client: %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

// Close stops the Copilot client
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		errs := c.client.Stop()
		if len(errs) > 0 {
			return fmt.Errorf("errors stopping client: %v", errs)
		}
	}
	return nil
}

// ModelSession represents a session with a specific model
type ModelSession struct {
	Model   string
	Session *copilot.Session
}

// CreateSession creates a session for a specific model
func (c *Client) CreateSession(ctx context.Context, model string, streaming bool) (*copilot.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, err := c.client.CreateSession(&copilot.SessionConfig{
		Model:     model,
		Streaming: streaming,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session for model %s: %w", model, err)
	}

	return session, nil
}

// Response represents a model's response
type Response struct {
	Model    string
	Content  string
	Error    error
	Duration time.Duration
}

// ProgressCallback is called when a model completes
type ProgressCallback func(model string, duration time.Duration, err error)

// AskMultipleModels asks the same question to multiple models in parallel
func (c *Client) AskMultipleModels(ctx context.Context, models []string, question string, timeout time.Duration, progress ProgressCallback) []Response {
	var wg sync.WaitGroup
	responses := make([]Response, len(models))

	for i, model := range models {
		wg.Add(1)
		go func(idx int, mdl string) {
			defer wg.Done()

			startTime := time.Now()
			
			// Create context with timeout
			askCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			resp := Response{Model: mdl}

			// Create session
			session, err := c.CreateSession(askCtx, mdl, false)
			if err != nil {
				resp.Error = err
				resp.Duration = time.Since(startTime)
				responses[idx] = resp
				if progress != nil {
					progress(mdl, resp.Duration, err)
				}
				return
			}
			defer func() {
				if err := session.Destroy(); err != nil {
					_ = err // Ignore error on cleanup
				}
			}()

			// Setup event collection
			done := make(chan bool)
			var content string

			session.On(func(event copilot.SessionEvent) {
				if event.Type == "assistant.message" {
					if event.Data.Content != nil {
						content = *event.Data.Content
					}
				}
				if event.Type == "session.idle" {
					close(done)
				}
			})

			// Send message
			_, err = session.Send(copilot.MessageOptions{
				Prompt: question,
			})
			if err != nil {
				resp.Error = fmt.Errorf("failed to send message: %w", err)
				resp.Duration = time.Since(startTime)
				responses[idx] = resp
				if progress != nil {
					progress(mdl, resp.Duration, err)
				}
				return
			}

			// Wait for response or timeout
			select {
			case <-done:
				resp.Content = content
				resp.Duration = time.Since(startTime)
			case <-askCtx.Done():
				resp.Error = fmt.Errorf("timeout waiting for response")
				resp.Duration = time.Since(startTime)
			}

			responses[idx] = resp
			if progress != nil {
				progress(mdl, resp.Duration, resp.Error)
			}
		}(i, model)
	}

	wg.Wait()
	return responses
}

// AskSingleModel asks a question to a single model
func (c *Client) AskSingleModel(ctx context.Context, model string, question string, timeout time.Duration) (string, time.Duration, error) {
	startTime := time.Now()
	
	askCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	session, err := c.CreateSession(askCtx, model, false)
	if err != nil {
		return "", time.Since(startTime), err
	}
	defer func() {
		if err := session.Destroy(); err != nil {
			_ = err // Ignore error on cleanup
		}
	}()

	done := make(chan bool)
	var content string

	session.On(func(event copilot.SessionEvent) {
		if event.Type == "assistant.message" {
			if event.Data.Content != nil {
				content = *event.Data.Content
			}
		}
		if event.Type == "session.idle" {
			close(done)
		}
	})

	_, err = session.Send(copilot.MessageOptions{
		Prompt: question,
	})
	if err != nil {
		return "", time.Since(startTime), fmt.Errorf("failed to send message: %w", err)
	}

	select {
	case <-done:
		return content, time.Since(startTime), nil
	case <-askCtx.Done():
		return "", time.Since(startTime), fmt.Errorf("timeout waiting for response")
	}
}
