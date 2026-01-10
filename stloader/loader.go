package stloader

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// RGB represents an RGB color
type RGB struct {
	R, G, B uint8
}

// DefaultSymbols are the spinner symbols used by default (Braille dots)
var DefaultSymbols = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Default configuration values
const (
	DefaultSpinInterval  = 200 * time.Millisecond
	DefaultShineInterval = 80 * time.Millisecond
)

// LoaderConfig holds configuration options for the loader
type LoaderConfig struct {
	// Symbols to rotate through for the spinner (default: ["/", "*", "\\", "|", "-"])
	Symbols []string
	// SpinInterval is the time between spinner symbol changes (default: 200ms)
	SpinInterval time.Duration
	// Text is the optional text to display after the spinner
	Text string
	// EnableShining enables the color sweep effect on text
	EnableShining bool
	// ShineInterval is the time between color sweep updates (default: 32ms)
	ShineInterval time.Duration
	// BaseColor is the base text color (user-defined)
	BaseColor RGB
	// Writer is the output writer (default: os.Stdout)
	Writer io.Writer
	// HideCursor hides the cursor while loading (default: true)
	// Use pointer to distinguish between unset (nil, defaults to true) and explicitly false
	HideCursor *bool
}

// Loader represents a terminal spinner with optional shining text effect
type Loader struct {
	config         LoaderConfig
	mu             sync.Mutex
	running        bool
	stopChan       chan struct{}
	doneChan       chan struct{}
	symbolIdx      int
	highlightIndex int
}

// NewLoader creates a new Loader with the given configuration
func NewLoader(cfg LoaderConfig) *Loader {
	// Apply defaults for zero values
	if cfg.Symbols == nil || len(cfg.Symbols) == 0 {
		cfg.Symbols = DefaultSymbols
	}
	if cfg.SpinInterval == 0 {
		cfg.SpinInterval = DefaultSpinInterval
	}
	if cfg.ShineInterval == 0 {
		cfg.ShineInterval = DefaultShineInterval
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	// HideCursor defaults to true if not explicitly set
	if cfg.HideCursor == nil {
		hideCursor := true
		cfg.HideCursor = &hideCursor
	}

	return &Loader{
		config: cfg,
	}
}

// NewLoaderWithText creates a simple loader with default settings and the given text
func NewLoaderWithText(text string) *Loader {
	return NewLoader(LoaderConfig{
		Text: text,
	})
}

// Start begins the loading animation
func (l *Loader) Start() {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return
	}
	l.running = true
	l.stopChan = make(chan struct{})
	l.doneChan = make(chan struct{})
	l.symbolIdx = 0
	l.highlightIndex = 0
	l.mu.Unlock()

	if *l.config.HideCursor {
		fmt.Fprint(l.config.Writer, "\033[?25l") // Hide cursor
	}

	go l.animate()
}

// Stop stops the loading animation and clears the line
func (l *Loader) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	l.running = false
	l.mu.Unlock()

	close(l.stopChan)
	<-l.doneChan // Wait for animation to finish

	// Clear the line
	fmt.Fprint(l.config.Writer, "\r\033[K")

	if *l.config.HideCursor {
		fmt.Fprint(l.config.Writer, "\033[?25h") // Show cursor
	}
}

// UpdateText changes the displayed text while running
func (l *Loader) UpdateText(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.Text = text
	l.highlightIndex = 0 // Reset highlight position for new text
}

// animate runs the animation loop in a goroutine
func (l *Loader) animate() {
	defer close(l.doneChan)

	// Use the faster interval for smooth animation
	interval := l.config.SpinInterval
	if l.config.EnableShining && l.config.ShineInterval < interval {
		interval = l.config.ShineInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Calculate how many ticks before updating the spinner symbol
	spinThreshold := 1
	if l.config.EnableShining && l.config.ShineInterval > 0 {
		spinThreshold = int(l.config.SpinInterval / l.config.ShineInterval)
		if spinThreshold < 1 {
			spinThreshold = 1
		}
	}

	spinCounter := 0

	for {
		select {
		case <-l.stopChan:
			return
		case <-ticker.C:
			// Consolidate all shared state access within a single mutex lock
			l.mu.Lock()
			symbol := l.config.Symbols[l.symbolIdx]
			text := l.config.Text
			highlightIdx := l.highlightIndex

			// Update highlight index for shining effect
			if l.config.EnableShining {
				textLen := len([]rune(text))
				if textLen > 0 {
					l.highlightIndex = (l.highlightIndex + 1) % textLen
				}
			}

			// Update spinner symbol at appropriate interval
			spinCounter++
			if spinCounter >= spinThreshold {
				l.symbolIdx = (l.symbolIdx + 1) % len(l.config.Symbols)
				spinCounter = 0
			}
			l.mu.Unlock()

			// Render outside the lock since it only reads local copies
			l.renderWithValues(symbol, text, highlightIdx)
		}
	}
}

// renderWithValues draws the current state to the terminal with pre-fetched values
func (l *Loader) renderWithValues(symbol, text string, highlightIdx int) {
	var output strings.Builder
	output.WriteString("\r\033[K") // Clear line first
	output.WriteString(symbol)

	if text != "" {
		output.WriteString(" ")
		if l.config.EnableShining {
			l.renderShiningText(&output, text, highlightIdx)
		} else {
			output.WriteString(text)
		}
	}

	fmt.Fprint(l.config.Writer, output.String())
}

// renderShiningText renders text with the shining effect directly to the builder
func (l *Loader) renderShiningText(w *strings.Builder, text string, highlightIdx int) {
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}

	baseColor := l.config.BaseColor
	highlightColor := lightenColor(baseColor)

	for i, r := range runes {
		if r == ' ' {
			w.WriteRune(r)
			continue
		}

		if i == highlightIdx {
			fmt.Fprintf(w, "\033[38;2;%d;%d;%dm%c", highlightColor.R, highlightColor.G, highlightColor.B, r)
		} else {
			fmt.Fprintf(w, "\033[38;2;%d;%d;%dm%c", baseColor.R, baseColor.G, baseColor.B, r)
		}
	}
	// Reset color at the end
	w.WriteString("\033[0m")
}

// lightenColor returns a color 20% lighter
func lightenColor(c RGB) RGB {
	return RGB{
		R: uint8(min(255, int(c.R)+51)), // 255 * 0.2 ≈ 51
		G: uint8(min(255, int(c.G)+51)),
		B: uint8(min(255, int(c.B)+51)),
	}
}

// colorize wraps text with ANSI 24-bit color escape codes
func colorize(text string, c RGB) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s", c.R, c.G, c.B, text)
}
