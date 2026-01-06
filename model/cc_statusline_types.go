package model

// CCStatuslineInput represents the JSON input from Claude Code statusline
type CCStatuslineInput struct {
	Model            CCStatuslineModel         `json:"model"`
	Cost             CCStatuslineCost          `json:"cost"`
	ContextWindow    CCStatuslineContextWindow `json:"context_window"`
	WorkingDirectory string                    `json:"working_directory"`
}

// CCStatuslineModel represents model information
type CCStatuslineModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// CCStatuslineCost represents session cost information
type CCStatuslineCost struct {
	TotalCostUSD    float64 `json:"total_cost_usd"`
	TotalDurationMS int64   `json:"total_duration_ms"`
}

// CCStatuslineContextWindow represents context window usage
type CCStatuslineContextWindow struct {
	TotalInputTokens  int                      `json:"total_input_tokens"`
	TotalOutputTokens int                      `json:"total_output_tokens"`
	ContextWindowSize int                      `json:"context_window_size"`
	CurrentUsage      *CCStatuslineContextUsage `json:"current_usage"`
}

// CCStatuslineContextUsage represents current context usage details
type CCStatuslineContextUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// CCStatuslineDailyCostResponse is the GraphQL response structure for daily cost
type CCStatuslineDailyCostResponse struct {
	FetchUser struct {
		AICodeOtel struct {
			Analytics struct {
				TotalCostUsd        float64 `json:"totalCostUsd"`
				TotalSessionSeconds int     `json:"totalSessionSeconds"`
			} `json:"analytics"`
		} `json:"aiCodeOtel"`
	} `json:"fetchUser"`
}

// CCStatuslineDailyCostQuery is the GraphQL query for fetching daily cost
const CCStatuslineDailyCostQuery = `query fetchAICodeOtelAnalytics($filter: AICodeAnalyticsFilter!) {
	fetchUser {
		aiCodeOtel {
			analytics(filter: $filter) {
				totalCostUsd
				totalSessionSeconds
			}
		}
	}
}`
