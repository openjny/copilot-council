package output

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/openjny/council/internal/copilot"
	"golang.org/x/term"
)

var (
	// Colors
	titleColor   = color.New(color.FgCyan, color.Bold)
	modelColor   = color.New(color.FgGreen, color.Bold)
	errorColor   = color.New(color.FgRed)
	successColor = color.New(color.FgGreen)
	dimColor     = color.New(color.Faint)
	warningColor = color.New(color.FgYellow)
)

// Printer handles formatted output
type Printer struct {
	verbose      bool
	spinners     map[string]*spinner.Spinner
	isTerminal   bool
	noSpinner    bool
}

// NewPrinter creates a new output printer
func NewPrinter(verbose bool) *Printer {
	// Check if stdout is a terminal
	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	
	// Disable spinner if not a TTY or if running in certain environments
	noSpinner := !isTerminal || os.Getenv("TERM") == "dumb" || os.Getenv("CI") == "true"
	
	return &Printer{
		verbose:    verbose,
		spinners:   make(map[string]*spinner.Spinner),
		isTerminal: isTerminal,
		noSpinner:  noSpinner,
	}
}

// PrintBanner prints the application banner
func (p *Printer) PrintBanner() {
	titleColor.Println("╔════════════════════════════════════════════════════════╗")
	titleColor.Println("║          🏛️  Council - AI Model Council               ║")
	titleColor.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// PrintQuestion prints the question being asked
func (p *Printer) PrintQuestion(question string) {
	titleColor.Print("❓ Question: ")
	fmt.Println(question)
	fmt.Println()
}

// PrintQueryingStart prints when querying starts
func (p *Printer) PrintQueryingStart() {
	titleColor.Println("🔄 Querying models in parallel...")
	fmt.Println()
}

// StartModelSpinner starts a spinner for a model
func (p *Printer) StartModelSpinner(model string) {
	if p.noSpinner {
		// No spinner, just print a simple message
		fmt.Printf("  [⋯] %s\n", model)
		return
	}
	
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf("  %s", model)
	s.Writer = os.Stderr // Write to stderr to avoid output conflicts
	s.Start()
	p.spinners[model] = s
}

// StopModelSpinner stops a spinner and shows result
func (p *Printer) StopModelSpinner(model string, duration time.Duration, err error) {
	if p.noSpinner {
		// Update the line we printed earlier
		if err != nil {
			errorColor.Printf("  [✗] %-25s ⏱️  %.2fs  ❌ %v\n", model, duration.Seconds(), err)
		} else {
			successColor.Printf("  [✓] %-25s ⏱️  %.2fs\n", model, duration.Seconds())
		}
		return
	}
	
	if s, ok := p.spinners[model]; ok {
		s.Stop()
		delete(p.spinners, model)
	}
	
	if err != nil {
		errorColor.Printf("  [✗] %-25s ⏱️  %.2fs  ❌ %v\n", model, duration.Seconds(), err)
	} else {
		successColor.Printf("  [✓] %-25s ⏱️  %.2fs\n", model, duration.Seconds())
	}
}

// PrintModelResponse prints a model's response
func (p *Printer) PrintModelResponse(resp copilot.Response) {
	fmt.Println()
	fmt.Println("┌────────────────────────────────────────────────────────┐")
	modelColor.Printf("│ 🤖 %-40s ⏱️  %.2fs │\n", resp.Model, resp.Duration.Seconds())
	fmt.Println("└────────────────────────────────────────────────────────┘")
	fmt.Println()

	if resp.Error != nil {
		p.PrintDetailedError(resp.Model, resp.Error, resp.Duration)
	} else {
		fmt.Println(resp.Content)
	}
	fmt.Println()
}

// PrintDetailedError prints a detailed error box
func (p *Printer) PrintDetailedError(model string, err error, duration time.Duration) {
	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	errorColor.Println("║ ⚠️  ERROR                                             ║")
	fmt.Println("╠═══════════════════════════════════════════════════════╣")
	fmt.Printf("║ Model:      %-41s ║\n", model)
	fmt.Printf("║ Issue:      %-41s ║\n", truncate(err.Error(), 41))
	fmt.Printf("║ Duration:   %-41s ║\n", fmt.Sprintf("%.2fs", duration.Seconds()))
	
	// Suggest solution based on error
	suggestion := getSuggestion(err)
	if suggestion != "" {
		fmt.Printf("║ Suggestion: %-41s ║\n", truncate(suggestion, 41))
	}
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
}

// getSuggestion returns a helpful suggestion based on the error
func getSuggestion(err error) string {
	errStr := err.Error()
	if strings.Contains(errStr, "timeout") {
		return "Try --timeout 120"
	}
	if strings.Contains(errStr, "failed to create session") {
		return "Check Copilot CLI is installed"
	}
	return ""
}

// truncate truncates a string to maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// PrintAggregationStart prints when aggregation begins
func (p *Printer) PrintAggregationStart(aggregator string, modelCount int) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	titleColor.Println("║ 🔄 Synthesizing responses...                          ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	
	if p.verbose {
		dimColor.Printf("  Aggregator: %s\n", aggregator)
		dimColor.Printf("  Analyzing: %d responses\n", modelCount)
	}
	
	if p.noSpinner {
		fmt.Println("  [⋯] Processing...")
		return
	}
	
	// Start aggregation spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = "  Processing..."
	s.Writer = os.Stderr
	s.Start()
	p.spinners["aggregator"] = s
}

// StopAggregationSpinner stops the aggregation spinner
func (p *Printer) StopAggregationSpinner(duration time.Duration) {
	if p.noSpinner {
		successColor.Printf("  [✓] Synthesis complete (%.2fs)\n", duration.Seconds())
		fmt.Println()
		return
	}
	
	if s, ok := p.spinners["aggregator"]; ok {
		s.Stop()
		delete(p.spinners, "aggregator")
	}
	successColor.Printf("  [✓] Synthesis complete (%.2fs)\n", duration.Seconds())
	fmt.Println()
}

// PrintFinalResult prints the final aggregated result
func (p *Printer) PrintFinalResult(content string) {
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	titleColor.Println("║ ⭐ FINAL ANSWER                                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println(content)
	fmt.Println()
}

// PrintError prints an error message
func (p *Printer) PrintError(err error) {
	errorColor.Printf("\n✗ Error: %v\n", err)
}

// PrintSummary prints a summary of the execution
func (p *Printer) PrintSummary(responses []copilot.Response, totalDuration time.Duration) {
	successCount := 0
	var fastestModel string
	var fastestDuration time.Duration = time.Hour
	var slowestDuration time.Duration
	var totalModelTime time.Duration

	for _, resp := range responses {
		if resp.Error == nil {
			successCount++
			totalModelTime += resp.Duration
			
			if resp.Duration < fastestDuration {
				fastestDuration = resp.Duration
				fastestModel = resp.Model
			}
			if resp.Duration > slowestDuration {
				slowestDuration = resp.Duration
			}
		}
	}

	// Calculate speedup (sequential time vs parallel time)
	var speedup float64
	if totalDuration.Seconds() > 0 {
		speedup = totalModelTime.Seconds() / totalDuration.Seconds()
	}

	fmt.Println("╔════════════════════════════════════════════════════════╗")
	titleColor.Println("║ 📊 EXECUTION SUMMARY                                   ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	
	if successCount == len(responses) {
		successColor.Printf("║ Models queried:  %-37s ║\n", fmt.Sprintf("%d/%d successful", successCount, len(responses)))
	} else {
		warningColor.Printf("║ Models queried:  %-37s ║\n", fmt.Sprintf("%d/%d successful", successCount, len(responses)))
	}
	
	if successCount > 0 {
		fmt.Printf("║ Fastest model:   %-37s ║\n", fmt.Sprintf("%s (%.2fs)", fastestModel, fastestDuration.Seconds()))
	}
	
	fmt.Printf("║ Total time:      %-37s ║\n", fmt.Sprintf("%.2fs", totalDuration.Seconds()))
	
	if speedup > 1 {
		fmt.Printf("║ Parallel speedup: %-36s ║\n", fmt.Sprintf("~%.1fx", speedup))
	}
	
	fmt.Println("╚════════════════════════════════════════════════════════╝")
}

// PrintVerbose prints verbose information
func (p *Printer) PrintVerbose(format string, args ...interface{}) {
	if p.verbose {
		dimColor.Printf("[VERBOSE] "+format+"\n", args...)
	}
}
