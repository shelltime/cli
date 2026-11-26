package daemon

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// gitRepoWithOrigin creates a work tree whose origin is remote.
func gitRepoWithOrigin(t *testing.T, remote string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", remote}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

// A record whose working directory is a clone gets the clone's repository, so the server groups
// it by repository; a repository the record names itself wins.
func TestRepositoryFromWorkingDirectory(t *testing.T) {
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})

	codex := newOtelResource(serviceResource("codex_cli_rs", kv("pwd", strVal(dir))), model.AICodeOtelSourceCodex)
	codex.repository = p.resolveRepository(context.Background(), codex)
	require.Equal(t, "https://github.com/acme/web", codex.repository)

	event := p.parseLogRecord(codexRecord("user_prompt", kv("prompt", strVal("hi"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/web", event.Attributes[model.AICodeRepositoryAttribute])

	event = p.parseLogRecord(codexRecord("user_prompt", kv("vcs.repository.url.full", strVal("https://github.com/acme/other"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/other", event.Attributes[model.AICodeRepositoryAttribute], "the record's own value wins")

	// Claude Code without OTEL_METRICS_INCLUDE_REPOSITORY, and a pwd resource attribute.
	plainClaude := newOtelResource(serviceResource("claude-code", kv("pwd", strVal(dir))), model.AICodeOtelSourceClaudeCode)
	plainClaude.repository = p.resolveRepository(context.Background(), plainClaude)
	delta := metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	metrics := p.parseMetric(sumMetric("claude_code.active_time.total", delta, 30, kv("type", strVal("cli"))), plainClaude, "com.anthropic.claude_code")
	require.Len(t, metrics, 1)
	assert.Equal(t, "https://github.com/acme/web", metrics[0].Attributes[model.AICodeRepositoryAttribute])
	metrics = p.parseMetric(sumMetric("claude_code.active_time.total", delta, 30, kv("vcs.repository.url.full", strVal("https://github.com/acme/other"))), plainClaude, "com.anthropic.claude_code")
	require.Len(t, metrics, 1)
	assert.Equal(t, "https://github.com/acme/other", metrics[0].Attributes[model.AICodeRepositoryAttribute], "the data point's own value wins")

	// Claude Code names its repository on the resource; the daemon leaves it alone.
	claude := newOtelResource(serviceResource("claude-code",
		kv("pwd", strVal(dir)),
		kv("vcs.repository.url.full", strVal("https://github.com/example-org/example-repo")),
	), model.AICodeOtelSourceClaudeCode)
	claude.repository = p.resolveRepository(context.Background(), claude)
	assert.Empty(t, claude.repository, "no lookup when the resource names its repository")
	event = p.parseLogRecord(claudeRecord("user_prompt", kv("prompt", strVal("hi"))), claude, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/example-org/example-repo", event.Attributes[model.AICodeRepositoryAttribute])
	// Metrics don't copy the resource's other attributes, and get no different repository from
	// the folder either, even with a repository resolved for it.
	claude.repository = "https://github.com/acme/web"
	metrics = p.parseMetric(sumMetric("claude_code.active_time.total", delta, 30, kv("type", strVal("cli"))), claude, "com.anthropic.claude_code")
	require.Len(t, metrics, 1)
	assert.NotContains(t, metrics[0].Attributes, model.AICodeRepositoryAttribute)
}

func TestRepositoryNeedsAWorkingDirectory(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	for _, res := range []*otelResource{
		newOtelResource(claudeTestResource(), model.AICodeOtelSourceClaudeCode),
		newOtelResource(serviceResource("codex_cli_rs", kv("pwd", strVal(t.TempDir()))), model.AICodeOtelSourceCodex),
	} {
		assert.Empty(t, p.resolveRepository(context.Background(), res))
	}

	p.repos = nil
	dir := gitRepoWithOrigin(t, "https://github.com/acme/web")
	res := newOtelResource(serviceResource("codex_cli_rs", kv("pwd", strVal(dir))), model.AICodeOtelSourceCodex)
	assert.Empty(t, p.resolveRepository(context.Background(), res), "a processor without a resolver adds nothing")
}

// With aiCodeOtel.sendRepository off, the daemon asks git nothing and removes the repository an
// agent reports itself.
func TestRepositoryNotSentWhenTurnedOff(t *testing.T) {
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	off := false
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{AICodeOtel: &model.AICodeOtel{SendRepository: &off}})
	assert.Nil(t, p.repos)

	codex := newOtelResource(serviceResource("codex_cli_rs", kv("pwd", strVal(dir))), model.AICodeOtelSourceCodex)
	codex.repository = p.resolveRepository(context.Background(), codex)
	assert.Empty(t, codex.repository)
	event := p.parseLogRecord(codexRecord("user_prompt", kv("vcs.repository.url.full", strVal("https://github.com/acme/other"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.NotContains(t, event.Attributes, model.AICodeRepositoryAttribute)

	claude := newOtelResource(serviceResource("claude-code",
		kv("pwd", strVal(dir)),
		kv("vcs.repository.url.full", strVal("https://github.com/example-org/example-repo")),
	), model.AICodeOtelSourceClaudeCode)
	event = p.parseLogRecord(claudeRecord("user_prompt", kv("prompt", strVal("hi"))), claude, "test-scope")
	require.NotNil(t, event)
	assert.NotContains(t, event.Attributes, model.AICodeRepositoryAttribute, "the resource's repository is removed too")
	delta := metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	metrics := p.parseMetric(sumMetric("claude_code.active_time.total", delta, 30, kv("vcs.repository.url.full", strVal("https://github.com/acme/other"))), claude, "com.anthropic.claude_code")
	require.Len(t, metrics, 1)
	assert.NotContains(t, metrics[0].Attributes, model.AICodeRepositoryAttribute)

	assert.Nil(t, socketRepositoryResolver(model.ShellTimeConfig{AICodeOtel: &model.AICodeOtel{SendRepository: &off}}))
	assert.NotNil(t, socketRepositoryResolver(model.ShellTimeConfig{}))
}

// A record that names a working directory of its own doesn't get the resource folder's
// repository; one that names its repository keeps it.
func TestRepositoryNotFilledForAnotherWorkingDirectory(t *testing.T) {
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	codex := newOtelResource(serviceResource("codex_cli_rs", kv("pwd", strVal(dir))), model.AICodeOtelSourceCodex)
	codex.repository = p.resolveRepository(context.Background(), codex)
	require.Equal(t, "https://github.com/acme/web", codex.repository)

	event := p.parseLogRecord(codexRecord("user_prompt", kv("pwd", strVal("/elsewhere"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "/elsewhere", event.Pwd)
	assert.NotContains(t, event.Attributes, model.AICodeRepositoryAttribute)

	event = p.parseLogRecord(codexRecord("user_prompt", kv("pwd", strVal(dir))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/web", event.Attributes[model.AICodeRepositoryAttribute], "the same folder keeps it")

	event = p.parseLogRecord(codexRecord("user_prompt", kv("pwd", strVal(filepath.Join(dir, "src", "api")))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/web", event.Attributes[model.AICodeRepositoryAttribute], "a subfolder of the work tree keeps it")
	assert.False(t, insideDir(dir+"-other", dir), "a sibling with a longer name is outside")

	event = p.parseLogRecord(codexRecord("user_prompt", kv("pwd", strVal("/elsewhere")), kv("vcs.repository.url.full", strVal("https://github.com/acme/web"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/web", event.Attributes[model.AICodeRepositoryAttribute], "the record named the same repository itself")

	event = p.parseLogRecord(codexRecord("user_prompt", kv("pwd", strVal("/elsewhere")), kv("vcs.repository.url.full", strVal("https://github.com/acme/other"))), codex, "test-scope")
	require.NotNil(t, event)
	assert.Equal(t, "https://github.com/acme/other", event.Attributes[model.AICodeRepositoryAttribute], "the record's own repository stays")
}
