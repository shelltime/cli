package model

import (
	"context"
	"time"

	"github.com/PromptPal/go-sdk/promptpal"
)

type AIService interface {
	QueryCommand(ctx context.Context, systemContext PPPromptGuessNextPromptVariables, userId string) (string, error)
}

type AIServiceConfig struct {
	Endpoint  string
	Token     string
	Timeout   time.Duration
	UserToken string
}

type promptPalAIService struct {
	client promptpal.PromptPalClient
}

func NewAIService(config AIServiceConfig) AIService {
	if config.Timeout == 0 {
		config.Timeout = 1 * time.Minute
	}

	applyTokenFunc := func(ctx context.Context) (promptpal.ApplyTemporaryTokenResult, error) {
		// Read the config to get the user's token
		return promptpal.ApplyTemporaryTokenResult{
			Token: "Bearer " + config.UserToken,
		}, nil
	}

	clientOptions := promptpal.PromptPalClientOptions{
		Timeout:             &config.Timeout,
		ApplyTemporaryToken: &applyTokenFunc,
	}

	client := promptpal.NewPromptPalClient(config.Endpoint, config.Token, clientOptions)

	return &promptPalAIService{
		client: client,
	}
}

func (s promptPalAIService) QueryCommand(
	ctx context.Context,
	systemContext PPPromptGuessNextPromptVariables,
	userId string,
) (string, error) {
	response, err := s.client.Execute(ctx, string(PPPromptGuessNextPrompt), PPPromptGuessNextPromptVariables{
		Shell: systemContext.Shell,
		Os:    systemContext.Os,
		Query: systemContext.Query,
	}, &userId)

	if err != nil {
		return "", err
	}

	return response.ResponseMessage, nil
}