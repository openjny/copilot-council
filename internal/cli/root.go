package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openjny/council/internal/council"
	"github.com/openjny/council/internal/output"
	"github.com/spf13/cobra"
)

var (
	models     []string
	aggregator string
	timeout    int
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "copilot-council [question]",
	Short: "Copilot Council - Ask multiple AI models and aggregate their responses",
	Long: `Copilot Council is a CLI tool that implements the "Council Pattern".
It asks the same question to multiple AI models (Claude, GPT, Gemini) in parallel,
then aggregates their responses using another model to produce a final synthesized answer.`,
	Args: cobra.ExactArgs(1),
	RunE: run,
	Example: `  # Ask a question using default models
  copilot-council "What is the capital of France?"

  # Specify custom models
  copilot-council --models claude-sonnet-4.5,gpt-5 "Explain quantum computing"

  # Use a different aggregator
  copilot-council --aggregator gpt-5 "Best practices for Go programming"

  # Increase timeout and enable verbose mode
  copilot-council --timeout 120 --verbose "Complex question here"`,
}

func init() {
	rootCmd.Flags().StringSliceVarP(&models, "models", "m", council.DefaultModels(),
		"Comma-separated list of models to consult")
	rootCmd.Flags().StringVarP(&aggregator, "aggregator", "a", council.DefaultAggregator(),
		"Model to use for aggregating responses")
	rootCmd.Flags().IntVarP(&timeout, "timeout", "t", 60,
		"Timeout in seconds for each model request")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false,
		"Enable verbose output")
}

func run(cmd *cobra.Command, args []string) error {
	question := args[0]
	printer := output.NewPrinter(verbose)

	// Print banner
	printer.PrintBanner()
	printer.PrintQuestion(question)

	// Validate models
	if len(models) == 0 {
		return fmt.Errorf("at least one model must be specified")
	}

	printer.PrintVerbose("Using models: %s", strings.Join(models, ", "))
	printer.PrintVerbose("Aggregator: %s", aggregator)
	printer.PrintVerbose("Timeout: %d seconds", timeout)

	// Create council
	c, err := council.NewCouncil(council.Config{
		Models:     models,
		Aggregator: aggregator,
		Timeout:    time.Duration(timeout) * time.Second,
		Verbose:    verbose,
		OriginalQ:  question,
	})
	if err != nil {
		printer.PrintError(err)
		return err
	}
	defer c.Close()

	// Execute council pattern
	ctx := context.Background()
	startTime := time.Now()

	// Print querying start
	printer.PrintQueryingStart()

	// Start spinners for each model
	for _, model := range models {
		printer.StartModelSpinner(model)
	}

	// Progress callback to update spinners
	progressCallback := func(model string, duration time.Duration, err error) {
		printer.StopModelSpinner(model, duration, err)
	}

	result := c.Execute(ctx, question, progressCallback)

	fmt.Println() // Space after spinners

	// Print individual model responses
	for _, resp := range result.ModelResponses {
		printer.PrintModelResponse(resp)
	}

	// Print aggregation phase
	if result.Error == nil {
		successCount := 0
		for _, resp := range result.ModelResponses {
			if resp.Error == nil {
				successCount++
			}
		}

		printer.PrintAggregationStart(aggregator, successCount)
		printer.StopAggregationSpinner(result.AggregationDuration)
		printer.PrintFinalResult(result.AggregatedResponse)
	} else {
		printer.PrintError(result.Error)
		return result.Error
	}

	// Print summary
	duration := time.Since(startTime)
	printer.PrintSummary(result.ModelResponses, duration)

	return nil
}

// Execute runs the root command
func Execute(ver string) {
	rootCmd.Version = ver
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
