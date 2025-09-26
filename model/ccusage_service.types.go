package model

type CCUsageProjectDailyOutput struct {
	Projects struct {
		UsersAnnatarheCodeMalamtimeWeb []struct {
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
		} `json:"-Users-annatarhe-code-malamtime-web"`
		UsersAnnatarheCodeMalamtimeServer []struct {
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
		} `json:"-Users-annatarhe-code-malamtime-server"`
		UsersAnnatarheCodeMalamtimeCli []struct {
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
		} `json:"-Users-annatarhe-code-malamtime-cli"`
		UsersAnnatarheCodePromptPalNodeSdk []struct {
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
		} `json:"-Users-annatarhe-code-PromptPal-node-sdk"`
		UsersAnnatarheCodePromptPalPromptPal []struct {
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
		} `json:"-Users-annatarhe-code-PromptPal-PromptPal"`
		UsersAnnatarheCodeMalamtimeAgent []struct {
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
		} `json:"-Users-annatarhe-code-malamtime-agent"`
		UsersAnnatarheCodeRealtimeArtAtomIns []struct {
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
		} `json:"-Users-annatarhe-code-realtime-art-atom-ins"`
		UsersAnnatarheCodeLakeUI []struct {
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
		} `json:"-Users-annatarhe-code-lake-ui"`
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
