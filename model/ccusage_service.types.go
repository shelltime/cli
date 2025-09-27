package model

type CCUsageProjectDailyOutput struct {
	Projects map[string][]struct {
		Date                string   `json:"date"`
		InputTokens         int      `json:"inputTokens"`
		OutputTokens        int      `json:"outputTokens"`
		CacheCreationTokens int      `json:"cacheCreationTokens"`
		CacheReadTokens     int      `json:"cacheReadTokens"`
		TotalTokens         int      `json:"totalTokens"`
		TotalCost           float64  `json:"totalCost"`
		ModelsUsed          []string `json:"modelsUsed"`
		ModelBreakdowns     []struct {
			ModelName           string  `json:"modelName"`
			InputTokens         int     `json:"inputTokens"`
			OutputTokens        int     `json:"outputTokens"`
			CacheCreationTokens int     `json:"cacheCreationTokens"`
			CacheReadTokens     int     `json:"cacheReadTokens"`
			Cost                float64 `json:"cost"`
		} `json:"modelBreakdowns"`
	} `json:"projects"`
	Totals struct {
		InputTokens         int     `json:"inputTokens"`
		OutputTokens        int     `json:"outputTokens"`
		CacheCreationTokens int     `json:"cacheCreationTokens"`
		CacheReadTokens     int     `json:"cacheReadTokens"`
		TotalCost           float64 `json:"totalCost"`
		TotalTokens         int     `json:"totalTokens"`
	} `json:"totals"`
}
