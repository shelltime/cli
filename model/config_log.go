package model

import "log/slog"

// LogValue implements slog.LogValuer, so a config passed to slog (for example
// slog.Any("config", cfg)) is logged without its secrets: tokens are masked,
// proxy credentials redacted and exclude patterns, which may hold the very
// strings they keep out of history, only counted. Fields added to
// ShellTimeConfig are not logged until they are added here.
func (c ShellTimeConfig) LogValue() slog.Value {
	endpoints := make([]string, 0, len(c.Endpoints))
	for _, e := range c.Endpoints {
		endpoints = append(endpoints, e.APIEndpoint)
	}

	attrs := []slog.Attr{
		slog.String("token", maskToken(c.Token)),
		slog.String("apiEndpoint", c.APIEndpoint),
		slog.String("webEndpoint", c.WebEndpoint),
		slog.Int("flushCount", c.FlushCount),
		slog.Int("gcTime", c.GCTime),
		boolPtrAttr("dataMasking", c.DataMasking),
		boolPtrAttr("enableMetrics", c.EnableMetrics),
		boolPtrAttr("encrypted", c.Encrypted),
		slog.Any("endpoints", endpoints),
		slog.Int("excludePatterns", len(c.Exclude)),
		slog.String("socketPath", c.SocketPath),
	}
	if c.AI != nil {
		attrs = append(attrs, slog.Group("ai",
			boolPtrAttr("showTips", c.AI.ShowTips),
			boolPtrAttr("shareContext", c.AI.ShareContext),
			slog.Bool("agentView", c.AI.Agent.View),
			slog.Bool("agentEdit", c.AI.Agent.Edit),
			slog.Bool("agentDelete", c.AI.Agent.Delete),
		))
	}
	if c.AICodeOtel != nil {
		attrs = append(attrs, slog.Group("aiCodeOtel",
			boolPtrAttr("enabled", c.AICodeOtel.Enabled),
			slog.Int("grpcPort", c.AICodeOtel.GRPCPort),
			boolPtrAttr("debug", c.AICodeOtel.Debug),
		))
	}
	if c.CodeTracking != nil {
		attrs = append(attrs, slog.Group("codeTracking",
			boolPtrAttr("enabled", c.CodeTracking.Enabled),
			slog.String("apiEndpoint", c.CodeTracking.APIEndpoint),
			slog.String("token", maskToken(c.CodeTracking.Token)),
		))
	}
	if c.LogCleanup != nil {
		attrs = append(attrs, slog.Group("logCleanup",
			boolPtrAttr("enabled", c.LogCleanup.Enabled),
			slog.Int64("thresholdMB", c.LogCleanup.ThresholdMB),
		))
	}
	if c.Storage != nil {
		attrs = append(attrs, slog.Group("storage", slog.String("engine", c.Storage.Engine)))
	}
	if c.Proxy != nil {
		attrs = append(attrs, slog.Group("proxy",
			slog.String("url", RedactProxyURL(c.Proxy.URL)),
			slog.Any("noProxy", c.Proxy.NoProxy),
		))
	}
	return slog.GroupValue(attrs...)
}

// boolPtrAttr logs an optional bool by value; slog would print the pointer.
func boolPtrAttr(key string, b *bool) slog.Attr {
	if b == nil {
		return slog.Any(key, nil)
	}
	return slog.Bool(key, *b)
}
