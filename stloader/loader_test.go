package stloader

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewLoader(t *testing.T) {
	l := NewLoader(LoaderConfig{})

	if l == nil {
		t.Fatal("NewLoader returned nil")
	}

	// Check defaults
	if len(l.config.Symbols) != len(DefaultSymbols) {
		t.Errorf("Expected %d symbols, got %d", len(DefaultSymbols), len(l.config.Symbols))
	}
	if l.config.SpinInterval != DefaultSpinInterval {
		t.Errorf("Expected SpinInterval %v, got %v", DefaultSpinInterval, l.config.SpinInterval)
	}
	if l.config.ShineInterval != DefaultShineInterval {
		t.Errorf("Expected ShineInterval %v, got %v", DefaultShineInterval, l.config.ShineInterval)
	}
	if l.config.Writer == nil {
		t.Error("Expected Writer to be set")
	}
	if l.config.HideCursor == nil || !*l.config.HideCursor {
		t.Error("Expected HideCursor to be true by default")
	}
}

func TestNewLoaderWithHideCursorFalse(t *testing.T) {
	hideCursor := false
	l := NewLoader(LoaderConfig{
		HideCursor: &hideCursor,
	})

	if l.config.HideCursor == nil {
		t.Fatal("Expected HideCursor to be set")
	}
	if *l.config.HideCursor != false {
		t.Error("Expected HideCursor to be false when explicitly set")
	}
}

func TestNewLoaderWithText(t *testing.T) {
	text := "Loading..."
	l := NewLoaderWithText(text)

	if l == nil {
		t.Fatal("NewLoaderWithText returned nil")
	}
	if l.config.Text != text {
		t.Errorf("Expected text %q, got %q", text, l.config.Text)
	}
}

func TestLoaderStartStop(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer:       &buf,
		Text:         "Test",
		SpinInterval: 20 * time.Millisecond,
	})

	l.Start()
	time.Sleep(100 * time.Millisecond)
	l.Stop()

	output := buf.String()
	// Should contain at least one spinner symbol
	found := false
	for _, sym := range DefaultSymbols {
		if strings.Contains(output, sym) {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected output to contain at least one spinner symbol")
	}

	// Should contain the text
	if !strings.Contains(output, "Test") {
		t.Error("Expected output to contain the text 'Test'")
	}
}

func TestLoaderDoubleStart(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer: &buf,
	})

	// Should not panic when starting twice
	l.Start()
	l.Start() // Second start should be ignored
	time.Sleep(50 * time.Millisecond)
	l.Stop()
}

func TestLoaderDoubleStop(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer: &buf,
	})

	l.Start()
	time.Sleep(50 * time.Millisecond)

	// Should not panic when stopping twice
	l.Stop()
	l.Stop() // Second stop should be ignored
}

func TestLoaderStopWithoutStart(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer: &buf,
	})

	// Should not panic when stopping without starting
	l.Stop()
}

func TestLoaderUpdateText(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer:       &buf,
		Text:         "Initial",
		SpinInterval: 20 * time.Millisecond,
	})

	l.Start()
	time.Sleep(100 * time.Millisecond)

	l.UpdateText("Updated")
	time.Sleep(100 * time.Millisecond)

	l.Stop()

	output := buf.String()
	if !strings.Contains(output, "Initial") {
		t.Error("Expected output to contain 'Initial'")
	}
	if !strings.Contains(output, "Updated") {
		t.Error("Expected output to contain 'Updated'")
	}
}

func TestLoaderShiningEffect(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer:        &buf,
		Text:          "Test",
		EnableShining: true,
		BaseColor:     RGB{R: 100, G: 150, B: 200},
		ShineInterval: 10 * time.Millisecond,
	})

	l.Start()
	time.Sleep(100 * time.Millisecond)
	l.Stop()

	output := buf.String()
	// Should contain ANSI color codes
	if !strings.Contains(output, "\033[38;2;") {
		t.Error("Expected output to contain ANSI 24-bit color codes")
	}
}

func TestLightenColor(t *testing.T) {
	tests := []struct {
		name     string
		input    RGB
		expected RGB
	}{
		{
			name:     "normal color",
			input:    RGB{R: 100, G: 100, B: 100},
			expected: RGB{R: 151, G: 151, B: 151},
		},
		{
			name:     "near max color",
			input:    RGB{R: 220, G: 220, B: 220},
			expected: RGB{R: 255, G: 255, B: 255}, // Should cap at 255
		},
		{
			name:     "zero color",
			input:    RGB{R: 0, G: 0, B: 0},
			expected: RGB{R: 51, G: 51, B: 51},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lightenColor(tt.input)
			if result != tt.expected {
				t.Errorf("lightenColor(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	result := colorize("X", RGB{R: 255, G: 128, B: 64})
	expected := "\033[38;2;255;128;64mX"

	if result != expected {
		t.Errorf("colorize('X', RGB{255, 128, 64}) = %q, expected %q", result, expected)
	}
}

func TestLoaderCustomSymbols(t *testing.T) {
	var buf bytes.Buffer
	customSymbols := []string{"a", "b", "c"}
	l := NewLoader(LoaderConfig{
		Writer:       &buf,
		Symbols:      customSymbols,
		SpinInterval: 20 * time.Millisecond,
	})

	l.Start()
	time.Sleep(100 * time.Millisecond)
	l.Stop()

	output := buf.String()
	// Should contain at least one custom symbol
	found := false
	for _, sym := range customSymbols {
		if strings.Contains(output, sym) {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected output to contain at least one custom symbol")
	}
}

func TestLoaderNoText(t *testing.T) {
	var buf bytes.Buffer
	l := NewLoader(LoaderConfig{
		Writer:       &buf,
		SpinInterval: 20 * time.Millisecond,
	})

	l.Start()
	time.Sleep(100 * time.Millisecond)
	l.Stop()

	// Should still work without text
	output := buf.String()
	found := false
	for _, sym := range DefaultSymbols {
		if strings.Contains(output, sym) {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected output to contain at least one spinner symbol")
	}
}
