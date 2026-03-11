package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveIncludeLines(t *testing.T) {
	t.Run("remove single-line include from top", func(t *testing.T) {
		content := "[[ -f ~/.bashrc.shelltime ]] && source ~/.bashrc.shelltime\nalias ll='ls -la'\nexport PATH=$PATH:/usr/local/bin"
		directive := &IncludeDirective{
			IncludeLine: "[[ -f ~/.bashrc.shelltime ]] && source ~/.bashrc.shelltime",
			CheckString: ".bashrc.shelltime",
		}

		result := removeIncludeLines(content, directive)
		assert.NotContains(t, result, ".bashrc.shelltime")
		assert.Contains(t, result, "alias ll='ls -la'")
		assert.Contains(t, result, "export PATH")
	})

	t.Run("remove multi-line include from top (git)", func(t *testing.T) {
		content := "[include]\n    path = ~/.gitconfig.shelltime\n\n[user]\n    name = Test"
		directive := &IncludeDirective{
			IncludeLine: "[include]\n    path = ~/.gitconfig.shelltime",
			CheckString: ".gitconfig.shelltime",
		}

		result := removeIncludeLines(content, directive)
		assert.NotContains(t, result, "[include]")
		assert.NotContains(t, result, ".gitconfig.shelltime")
		assert.Contains(t, result, "[user]")
		assert.Contains(t, result, "name = Test")
	})

	t.Run("fallback to check string removal", func(t *testing.T) {
		content := "some line\n[[ -f ~/.bashrc.shelltime ]] && source ~/.bashrc.shelltime\nalias ll='ls -la'"
		directive := &IncludeDirective{
			IncludeLine: "[[ -f ~/.bashrc.shelltime ]] && source ~/.bashrc.shelltime",
			CheckString: ".bashrc.shelltime",
		}

		result := removeIncludeLines(content, directive)
		assert.NotContains(t, result, ".bashrc.shelltime")
		assert.Contains(t, result, "some line")
		assert.Contains(t, result, "alias ll='ls -la'")
	})
}

func TestBaseApp_ensureIncludeSetup(t *testing.T) {
	app := &BaseApp{name: "test"}

	t.Run("first time setup - no include, no shelltime file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")
		originalContent := "alias ll='ls -la'\nexport PATH=$PATH:/usr/local/bin\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   "[[ -f " + shelltimePath + " ]] && source " + shelltimePath,
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err)

		// Check .shelltime file was created with original content
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(shelltimeContent))

		// Check original file has include line at top
		updatedOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(updatedOriginal), directive.IncludeLine))
		assert.Contains(t, string(updatedOriginal), originalContent)
	})

	t.Run("include line missing but shelltime file exists", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")

		err = os.WriteFile(originalPath, []byte("local stuff\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(shelltimePath, []byte("synced stuff\n"), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   "[[ -f " + shelltimePath + " ]] && source " + shelltimePath,
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err)

		// Check include line was added to original
		updatedOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(updatedOriginal), directive.IncludeLine))
		assert.Contains(t, string(updatedOriginal), "local stuff")

		// .shelltime file should be unchanged
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, "synced stuff\n", string(shelltimeContent))
	})

	t.Run("include line exists but shelltime file missing", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")
		includeLine := "[[ -f " + shelltimePath + " ]] && source " + shelltimePath
		originalContent := includeLine + "\nalias ll='ls -la'\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   includeLine,
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err)

		// .shelltime file should be created with content minus include line
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.NotContains(t, string(shelltimeContent), ".bashrc.shelltime")
		assert.Contains(t, string(shelltimeContent), "alias ll='ls -la'")
	})

	t.Run("both exist - no changes needed", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")
		includeLine := "[[ -f " + shelltimePath + " ]] && source " + shelltimePath

		err = os.WriteFile(originalPath, []byte(includeLine+"\nlocal stuff\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(shelltimePath, []byte("synced stuff\n"), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   includeLine,
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err)

		// Both files should be unchanged
		originalResult, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Equal(t, includeLine+"\nlocal stuff\n", string(originalResult))

		shelltimeResult, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, "synced stuff\n", string(shelltimeResult))
	})

	t.Run("original file does not exist", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		directive := &IncludeDirective{
			OriginalPath:  filepath.Join(tmpDir, "nonexistent"),
			ShelltimePath: filepath.Join(tmpDir, "nonexistent.shelltime"),
			IncludeLine:   "source " + filepath.Join(tmpDir, "nonexistent.shelltime"),
			CheckString:   "nonexistent.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err) // Should not error, just skip
	})

	t.Run("git config multi-line include", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".gitconfig")
		shelltimePath := filepath.Join(tmpDir, ".gitconfig.shelltime")
		originalContent := "[user]\n    name = Test User\n    email = test@example.com\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   "[include]\n    path = " + shelltimePath,
			CheckString:   ".gitconfig.shelltime",
		}

		err = app.ensureIncludeSetup(directive)
		require.NoError(t, err)

		// .shelltime file should have original content
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(shelltimeContent))

		// Original should have multi-line include at top
		updatedOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(updatedOriginal), "[include]\n"))
		assert.Contains(t, string(updatedOriginal), shelltimePath)
		assert.Contains(t, string(updatedOriginal), "[user]")
	})
}

func TestBaseApp_ensureIncludeLineInFile(t *testing.T) {
	app := &BaseApp{name: "test"}

	t.Run("adds include line when missing", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		err = os.WriteFile(originalPath, []byte("alias ll='ls -la'\n"), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: filepath.Join(tmpDir, ".bashrc.shelltime"),
			IncludeLine:   "source ~/.bashrc.shelltime",
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeLineInFile(directive, false)
		require.NoError(t, err)

		content, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(content), "source ~/.bashrc.shelltime"))
		assert.Contains(t, string(content), "alias ll='ls -la'")
	})

	t.Run("skips when include already exists", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		existingContent := "source ~/.bashrc.shelltime\nalias ll='ls -la'\n"
		err = os.WriteFile(originalPath, []byte(existingContent), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: filepath.Join(tmpDir, ".bashrc.shelltime"),
			IncludeLine:   "source ~/.bashrc.shelltime",
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeLineInFile(directive, false)
		require.NoError(t, err)

		content, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content)) // Unchanged
	})

	t.Run("creates file with include line when original does not exist", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: filepath.Join(tmpDir, ".bashrc.shelltime"),
			IncludeLine:   "source ~/.bashrc.shelltime",
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeLineInFile(directive, false)
		require.NoError(t, err)

		content, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Equal(t, "source ~/.bashrc.shelltime\n", string(content))
	})

	t.Run("dry run does not modify file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		originalContent := "alias ll='ls -la'\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directive := &IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: filepath.Join(tmpDir, ".bashrc.shelltime"),
			IncludeLine:   "source ~/.bashrc.shelltime",
			CheckString:   ".bashrc.shelltime",
		}

		err = app.ensureIncludeLineInFile(directive, true)
		require.NoError(t, err)

		content, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(content)) // Unchanged
	})
}

func TestBaseApp_CollectWithIncludeSupport(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	t.Run("collects from shelltime file for includable path", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")
		originalContent := "alias ll='ls -la'\nexport EDITOR=vim\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directives := []IncludeDirective{
			{
				OriginalPath:  originalPath,
				ShelltimePath: shelltimePath,
				IncludeLine:   "source " + shelltimePath,
				CheckString:   ".bashrc.shelltime",
			},
		}

		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "bash", []string{originalPath}, &skipIgnored, directives)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)

		// Should collect from .shelltime path
		assert.Equal(t, shelltimePath, dotfiles[0].Path)
		assert.Equal(t, originalContent, dotfiles[0].Content)

		// Original should now have include line
		updatedOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Contains(t, string(updatedOriginal), ".bashrc.shelltime")
	})

	t.Run("collects non-include paths normally", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		regularFile := filepath.Join(tmpDir, ".gitignore_global")
		err = os.WriteFile(regularFile, []byte("*.swp\n.DS_Store\n"), 0644)
		require.NoError(t, err)

		// No directives for this path
		directives := []IncludeDirective{
			{
				OriginalPath:  filepath.Join(tmpDir, ".gitconfig"),
				ShelltimePath: filepath.Join(tmpDir, ".gitconfig.shelltime"),
				IncludeLine:   "[include]\n    path = " + filepath.Join(tmpDir, ".gitconfig.shelltime"),
				CheckString:   ".gitconfig.shelltime",
			},
		}

		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "git", []string{regularFile}, &skipIgnored, directives)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)

		// Should collect from the original path directly
		assert.Equal(t, regularFile, dotfiles[0].Path)
		assert.Equal(t, "*.swp\n.DS_Store\n", dotfiles[0].Content)
	})

	t.Run("handles mixed includable and non-includable paths", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		gitconfigPath := filepath.Join(tmpDir, ".gitconfig")
		gitignorePath := filepath.Join(tmpDir, ".gitignore_global")
		shelltimePath := filepath.Join(tmpDir, ".gitconfig.shelltime")

		err = os.WriteFile(gitconfigPath, []byte("[user]\n    name = Test\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(gitignorePath, []byte("*.swp\n"), 0644)
		require.NoError(t, err)

		directives := []IncludeDirective{
			{
				OriginalPath:  gitconfigPath,
				ShelltimePath: shelltimePath,
				IncludeLine:   "[include]\n    path = " + shelltimePath,
				CheckString:   ".gitconfig.shelltime",
			},
		}

		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "git", []string{gitconfigPath, gitignorePath}, &skipIgnored, directives)
		require.NoError(t, err)
		require.Len(t, dotfiles, 2)

		// Find each dotfile by path
		var shelltimeDotfile, gitignoreDotfile *DotfileItem
		for i := range dotfiles {
			if dotfiles[i].Path == shelltimePath {
				shelltimeDotfile = &dotfiles[i]
			} else if dotfiles[i].Path == gitignorePath {
				gitignoreDotfile = &dotfiles[i]
			}
		}

		require.NotNil(t, shelltimeDotfile, ".shelltime file should be collected")
		require.NotNil(t, gitignoreDotfile, ".gitignore_global should be collected directly")
	})

	t.Run("skips non-existent original file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		nonExistent := filepath.Join(tmpDir, ".bashrc")
		directives := []IncludeDirective{
			{
				OriginalPath:  nonExistent,
				ShelltimePath: filepath.Join(tmpDir, ".bashrc.shelltime"),
				IncludeLine:   "source " + filepath.Join(tmpDir, ".bashrc.shelltime"),
				CheckString:   ".bashrc.shelltime",
			},
		}

		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "bash", []string{nonExistent}, &skipIgnored, directives)
		require.NoError(t, err)
		assert.Empty(t, dotfiles)
	})

	t.Run("handles directory paths without include", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a directory with files
		dirPath := filepath.Join(tmpDir, "conf.d")
		err = os.MkdirAll(dirPath, 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dirPath, "aliases.fish"), []byte("alias g='git'\n"), 0644)
		require.NoError(t, err)

		directives := []IncludeDirective{
			{
				OriginalPath:  filepath.Join(tmpDir, "config.fish"),
				ShelltimePath: filepath.Join(tmpDir, "config.fish.shelltime"),
				IncludeLine:   "source " + filepath.Join(tmpDir, "config.fish.shelltime"),
				CheckString:   "config.fish.shelltime",
			},
		}

		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "fish", []string{dirPath}, &skipIgnored, directives)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)
		assert.Contains(t, dotfiles[0].Path, "aliases.fish")
	})
}

func TestBaseApp_SaveWithIncludeSupport(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	t.Run("saves shelltime file and ensures include line", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")

		// Original file without include line
		err = os.WriteFile(originalPath, []byte("existing stuff\n"), 0644)
		require.NoError(t, err)

		directives := []IncludeDirective{
			{
				OriginalPath:  originalPath,
				ShelltimePath: shelltimePath,
				IncludeLine:   "source " + shelltimePath,
				CheckString:   ".bashrc.shelltime",
			},
		}

		files := map[string]string{
			shelltimePath: "synced content\n",
		}

		err = app.SaveWithIncludeSupport(ctx, files, false, directives)
		require.NoError(t, err)

		// .shelltime file should be written
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, "synced content\n", string(shelltimeContent))

		// Original should now have include line
		originalContent, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Contains(t, string(originalContent), ".bashrc.shelltime")
	})

	t.Run("saves non-shelltime file normally", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		regularPath := filepath.Join(tmpDir, ".gitignore_global")

		directives := []IncludeDirective{
			{
				OriginalPath:  filepath.Join(tmpDir, ".gitconfig"),
				ShelltimePath: filepath.Join(tmpDir, ".gitconfig.shelltime"),
				IncludeLine:   "[include]\n    path = " + filepath.Join(tmpDir, ".gitconfig.shelltime"),
				CheckString:   ".gitconfig.shelltime",
			},
		}

		files := map[string]string{
			regularPath: "*.swp\n.DS_Store\n",
		}

		err = app.SaveWithIncludeSupport(ctx, files, false, directives)
		require.NoError(t, err)

		content, err := os.ReadFile(regularPath)
		require.NoError(t, err)
		assert.Equal(t, "*.swp\n.DS_Store\n", string(content))
	})

	t.Run("dry run does not modify original file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "dotfile-include-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".bashrc")
		shelltimePath := filepath.Join(tmpDir, ".bashrc.shelltime")
		originalContent := "existing stuff\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directives := []IncludeDirective{
			{
				OriginalPath:  originalPath,
				ShelltimePath: shelltimePath,
				IncludeLine:   "source " + shelltimePath,
				CheckString:   ".bashrc.shelltime",
			},
		}

		files := map[string]string{
			shelltimePath: "synced content\n",
		}

		err = app.SaveWithIncludeSupport(ctx, files, true, directives)
		require.NoError(t, err)

		// Original should NOT have include line in dry run
		content, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(content))
	})
}

func TestGetIncludeDirectives(t *testing.T) {
	t.Run("git app has directives for gitconfig files", func(t *testing.T) {
		app := NewGitApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 2)

		// Check .gitconfig directive
		assert.Equal(t, "~/.gitconfig", directives[0].OriginalPath)
		assert.Equal(t, "~/.gitconfig.shelltime", directives[0].ShelltimePath)
		assert.Contains(t, directives[0].IncludeLine, "[include]")
		assert.Contains(t, directives[0].IncludeLine, ".gitconfig.shelltime")

		// Check config/git/config directive
		assert.Equal(t, "~/.config/git/config", directives[1].OriginalPath)
		assert.Equal(t, "~/.config/git/config.shelltime", directives[1].ShelltimePath)
	})

	t.Run("zsh app has directives for shell configs", func(t *testing.T) {
		app := NewZshApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 3)

		for _, d := range directives {
			assert.Contains(t, d.IncludeLine, "source")
			assert.Contains(t, d.IncludeLine, ".shelltime")
		}
	})

	t.Run("bash app has directives for all bash configs", func(t *testing.T) {
		app := NewBashApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 4)

		expectedPaths := []string{"~/.bashrc", "~/.bash_profile", "~/.bash_aliases", "~/.bash_logout"}
		for i, d := range directives {
			assert.Equal(t, expectedPaths[i], d.OriginalPath)
			assert.Contains(t, d.IncludeLine, "source")
		}
	})

	t.Run("fish app has directive for config.fish", func(t *testing.T) {
		app := NewFishApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 1)
		assert.Equal(t, "~/.config/fish/config.fish", directives[0].OriginalPath)
		assert.Contains(t, directives[0].IncludeLine, "source")
	})

	t.Run("ssh app has directive for ssh config", func(t *testing.T) {
		app := NewSshApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 1)
		assert.Equal(t, "~/.ssh/config", directives[0].OriginalPath)
		assert.Contains(t, directives[0].IncludeLine, "Include")
	})

	t.Run("nvim app has directive for vimrc", func(t *testing.T) {
		app := NewNvimApp()
		directives := app.GetIncludeDirectives()
		assert.Len(t, directives, 1)
		assert.Equal(t, "~/.vimrc", directives[0].OriginalPath)
		assert.Contains(t, directives[0].IncludeLine, "source")
	})

	t.Run("apps without include support return nil", func(t *testing.T) {
		apps := []DotfileApp{
			NewGhosttyApp(),
			NewClaudeApp(),
			NewStarshipApp(),
			NewNpmApp(),
			NewKittyApp(),
			NewKubernetesApp(),
		}

		for _, app := range apps {
			directives := app.GetIncludeDirectives()
			assert.Nil(t, directives, "App %s should return nil directives", app.Name())
		}
	})
}

func TestIncludeDirective_EndToEnd(t *testing.T) {
	t.Run("push then pull workflow", func(t *testing.T) {
		app := &BaseApp{name: "test"}
		ctx := context.Background()

		tmpDir, err := os.MkdirTemp("", "dotfile-e2e-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		originalPath := filepath.Join(tmpDir, ".gitconfig")
		shelltimePath := filepath.Join(tmpDir, ".gitconfig.shelltime")

		originalContent := "[user]\n    name = Test User\n    email = test@example.com\n"
		err = os.WriteFile(originalPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		directive := IncludeDirective{
			OriginalPath:  originalPath,
			ShelltimePath: shelltimePath,
			IncludeLine:   "[include]\n    path = " + shelltimePath,
			CheckString:   ".gitconfig.shelltime",
		}
		directives := []IncludeDirective{directive}

		// Simulate PUSH: collect dotfiles
		skipIgnored := true
		dotfiles, err := app.CollectWithIncludeSupport(ctx, "git", []string{originalPath}, &skipIgnored, directives)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)

		// The collected dotfile should be from .shelltime path
		assert.Equal(t, shelltimePath, dotfiles[0].Path)
		assert.Equal(t, originalContent, dotfiles[0].Content)

		// Original should now have include line
		updatedOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(updatedOriginal), "[include]"))

		// Simulate server storing and returning updated content
		serverContent := "[user]\n    name = Updated User\n    email = updated@example.com\n[core]\n    editor = vim\n"

		// Simulate PULL: save to .shelltime file
		files := map[string]string{
			shelltimePath: serverContent,
		}
		err = app.SaveWithIncludeSupport(ctx, files, false, directives)
		require.NoError(t, err)

		// .shelltime file should have server content
		shelltimeContent, err := os.ReadFile(shelltimePath)
		require.NoError(t, err)
		assert.Equal(t, serverContent, string(shelltimeContent))

		// Original should still have include line
		finalOriginal, err := os.ReadFile(originalPath)
		require.NoError(t, err)
		assert.Contains(t, string(finalOriginal), ".gitconfig.shelltime")
	})
}
