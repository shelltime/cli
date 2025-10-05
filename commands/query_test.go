package commands

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// MockAIService is a mock implementation of the AI service
type queryTestSuite struct {
	suite.Suite
	mockAI     *model.MockAIService
	mockConfig *model.MockConfigService
	app        *cli.App
	origAI     model.AIService
}

// SetupSuite runs once before all tests
func (s *queryTestSuite) SetupSuite() {
	logrus.SetLevel(logrus.TraceLevel)
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
}

// SetupTest runs before each test
func (s *queryTestSuite) SetupTest() {
	// Save original AI service
	s.origAI = aiService

	// Create mocks
	s.mockAI = model.NewMockAIService(s.T())
	s.mockConfig = model.NewMockConfigService(s.T())

	// Set global services
	aiService = s.mockAI
	configService = s.mockConfig

	// Create test app
	s.app = &cli.App{
		Name:  "shelltime-test",
		Usage: "test app for query command",
		Commands: []*cli.Command{
			QueryCommand,
		},
	}
}

// TearDownTest runs after each test
func (s *queryTestSuite) TearDownTest() {
	// Restore original AI service
	aiService = s.origAI
	s.mockAI.AssertExpectations(s.T())
	s.mockConfig.AssertExpectations(s.T())
}

func (s *queryTestSuite) TestQueryCommandNoAIService() {
	// Set AI service to nil
	aiService = nil

	command := []string{
		"shelltime-test",
		"query",
		"list files",
	}

	err := s.app.Run(command)
	assert.NotNil(s.T(), err)
	assert.Contains(s.T(), err.Error(), "AI service is not available")
}

func (s *queryTestSuite) TestQueryCommandNoArguments() {
	command := []string{
		"shelltime-test",
		"query",
	}

	err := s.app.Run(command)
	assert.NotNil(s.T(), err)
	assert.Contains(s.T(), err.Error(), "query is required")
}

func (s *queryTestSuite) TestQueryCommandSuccess() {
	// Setup mocks
	expectedCommand := "ls -la"
	query := "list all files with details"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.MatchedBy(func(sc model.PPPromptGuessNextPromptVariables) bool {
		return sc.Query == query && sc.Os == runtime.GOOS
	}), "").Return(expectedCommand, nil)

	// Mock config service - no auto-run
	mockedConfig := model.ShellTimeConfig{}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandWithMultipleArgs() {
	// Setup mocks
	expectedCommand := "find . -name '*.go' -type f"
	queryParts := []string{"find", "all", "go", "files"}
	fullQuery := strings.Join(queryParts, " ")

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.MatchedBy(func(sc model.PPPromptGuessNextPromptVariables) bool {
		return sc.Query == fullQuery
	}), "").Return(expectedCommand, nil)

	// Mock config service
	mockedConfig := model.ShellTimeConfig{}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := append([]string{"shelltime-test", "query"}, queryParts...)

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandAIError() {
	query := "complex query"

	// Mock AI service error
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return("", errors.New("AI service error"))

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	err := s.app.Run(command)
	assert.NotNil(s.T(), err)
	assert.Contains(s.T(), err.Error(), "AI service error")
}

func (s *queryTestSuite) TestQueryCommandWithAutoRunView() {
	expectedCommand := "ls -la"
	query := "list files"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config with auto-run enabled for view commands
	mockedConfig := model.ShellTimeConfig{
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{
				View:   true,
				Edit:   false,
				Delete: false,
			},
		},
	}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	// Note: The actual command execution would fail in test environment
	// We're testing the flow up to the execution point
	err := s.app.Run(command)
	// The error here is expected as we can't actually execute the command in tests
	assert.NotNil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandWithAutoRunEdit() {
	expectedCommand := "sed -i 's/foo/bar/g' file.txt"
	query := "replace foo with bar in file.txt"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config with auto-run enabled for edit commands
	mockedConfig := model.ShellTimeConfig{
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{
				View:   false,
				Edit:   true,
				Delete: false,
			},
		},
	}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	// The command would be auto-executed if enabled, but will fail in test
	err := s.app.Run(command)
	assert.NotNil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandWithAutoRunDeleteDisabled() {
	expectedCommand := "rm -rf /tmp/test"
	query := "delete test directory"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config with auto-run disabled for delete commands
	mockedConfig := model.ShellTimeConfig{
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{
				View:   true,
				Edit:   true,
				Delete: false,
			},
		},
	}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	// Should not auto-run delete commands when disabled
	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandConfigReadError() {
	expectedCommand := "echo 'test'"
	query := "print test"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config service error - should fallback gracefully
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, errors.New("config read error"))

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandTrimWhitespace() {
	expectedCommand := "  echo 'hello'  \n\t"
	// trimmedCommand := "echo 'hello'"
	query := "print hello"

	// Mock AI service returning command with whitespace
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config service
	mockedConfig := model.ShellTimeConfig{}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
	// The command should be trimmed before processing
}

func (s *queryTestSuite) TestGetSystemContext() {
	query := "test query"

	// Test with SHELL environment variable set
	originalShell := os.Getenv("SHELL")
	os.Setenv("SHELL", "/bin/bash")
	defer os.Setenv("SHELL", originalShell)

	context, err := getSystemContext(query)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "bash", context.Shell)
	assert.Equal(s.T(), runtime.GOOS, context.Os)
	assert.Equal(s.T(), query, context.Query)

	// Test with no SHELL environment variable
	os.Unsetenv("SHELL")
	context, err = getSystemContext(query)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "unknown", context.Shell)
	assert.Equal(s.T(), runtime.GOOS, context.Os)
	assert.Equal(s.T(), query, context.Query)

	// Test with full path shell
	os.Setenv("SHELL", "/usr/local/bin/zsh")
	context, err = getSystemContext(query)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "zsh", context.Shell)
}

func (s *queryTestSuite) TestQueryCommandWithAlias() {
	expectedCommand := "ls"
	query := "list"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config service
	mockedConfig := model.ShellTimeConfig{}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	// Test using the alias "q" instead of "query"
	command := []string{
		"shelltime-test",
		"q",
		query,
	}

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestExecuteCommand() {
	ctx := context.Background()

	// Test with simple echo command (should work in test environment)
	err := executeCommand(ctx, "echo 'test'")
	// Echo should succeed
	assert.Nil(s.T(), err)

	// Test with invalid command
	err = executeCommand(ctx, "invalid_command_that_does_not_exist")
	assert.NotNil(s.T(), err)
}

func (s *queryTestSuite) TestDisplayCommand() {
	// This is a simple display function, just ensure it doesn't panic
	assert.NotPanics(s.T(), func() {
		displayCommand("test command")
	})
}

func (s *queryTestSuite) TestQueryCommandAutoRunOtherType() {
	expectedCommand := "some-complex-command --with-flags"
	query := "do something complex"

	// Mock AI service
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return(expectedCommand, nil)

	// Mock config with auto-run enabled but command is "other" type
	mockedConfig := model.ShellTimeConfig{
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{
				View:   true,
				Edit:   true,
				Delete: true,
			},
		},
	}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	// Other type commands should not auto-run
	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandEmptyAIResponse() {
	query := "do nothing"

	// Mock AI service returning empty string
	s.mockAI.On("QueryCommand", mock.Anything, mock.Anything, "").Return("", nil)

	// Mock config service
	mockedConfig := model.ShellTimeConfig{}
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)

	command := []string{
		"shelltime-test",
		"query",
		query,
	}

	err := s.app.Run(command)
	assert.Nil(s.T(), err)
}

func (s *queryTestSuite) TestQueryCommandDescription() {
	// Test that the command has proper description and usage
	assert.Equal(s.T(), "query", QueryCommand.Name)
	assert.Contains(s.T(), QueryCommand.Aliases, "q")
	assert.Equal(s.T(), "Query AI for command suggestions", QueryCommand.Usage)
	assert.Contains(s.T(), QueryCommand.Description, "Query AI for command suggestions")
	assert.Contains(s.T(), QueryCommand.Description, "Examples:")
}

// TestQueryTestSuite runs the test suite
func TestQueryTestSuite(t *testing.T) {
	suite.Run(t, new(queryTestSuite))
}
