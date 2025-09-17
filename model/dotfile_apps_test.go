package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseApp_expandPath(t *testing.T) {
	app := &BaseApp{name: "test"}

	t.Run("expand tilde path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		expanded, err := app.expandPath("~/.config/test")
		require.NoError(t, err)
		expected := filepath.Join(homeDir, ".config/test")
		assert.Equal(t, expected, expanded)
	})

	t.Run("expand absolute path", func(t *testing.T) {
		testPath := "/tmp/test/config"
		expanded, err := app.expandPath(testPath)
		require.NoError(t, err)

		// Should be converted to absolute path
		abs, err := filepath.Abs(testPath)
		require.NoError(t, err)
		assert.Equal(t, abs, expanded)
	})

	t.Run("expand relative path", func(t *testing.T) {
		testPath := "relative/path"
		expanded, err := app.expandPath(testPath)
		require.NoError(t, err)

		// Should be converted to absolute path
		abs, err := filepath.Abs(testPath)
		require.NoError(t, err)
		assert.Equal(t, abs, expanded)
	})
}

func TestBaseApp_readFileContent(t *testing.T) {
	app := &BaseApp{name: "test"}

	// Create temporary file for testing
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.conf")
	testContent := "# Test configuration\nkey=value\n"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	t.Run("read existing file", func(t *testing.T) {
		content, modTime, err := app.readFileContent(testFile)
		require.NoError(t, err)
		assert.Equal(t, testContent, content)
		assert.NotNil(t, modTime)
		assert.False(t, modTime.IsZero())
	})

	t.Run("read non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "does-not-exist.conf")
		_, _, err := app.readFileContent(nonExistentFile)
		assert.Error(t, err)
	})

	t.Run("read with tilde path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		// Create a test file in home directory
		homeTestDir := filepath.Join(homeDir, ".shelltime-test")
		err = os.MkdirAll(homeTestDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(homeTestDir)

		homeTestFile := filepath.Join(homeTestDir, "test.conf")
		err = os.WriteFile(homeTestFile, []byte(testContent), 0644)
		require.NoError(t, err)

		// Use tilde path
		tildePath := "~/.shelltime-test/test.conf"
		content, modTime, err := app.readFileContent(tildePath)
		require.NoError(t, err)
		assert.Equal(t, testContent, content)
		assert.NotNil(t, modTime)
	})
}

func TestBaseApp_CollectFromPaths(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	// Create temporary directory structure
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	configFile := filepath.Join(tmpDir, "config.conf")
	configContent := "key1=value1\n"
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	subFile1 := filepath.Join(subDir, "file1.txt")
	subFile1Content := "content1\n"
	err = os.WriteFile(subFile1, []byte(subFile1Content), 0644)
	require.NoError(t, err)

	subFile2 := filepath.Join(subDir, "file2.txt")
	subFile2Content := "content2\n"
	err = os.WriteFile(subFile2, []byte(subFile2Content), 0644)
	require.NoError(t, err)

	// Create hidden file (should be ignored in directories)
	hiddenFile := filepath.Join(subDir, ".hidden")
	err = os.WriteFile(hiddenFile, []byte("hidden"), 0644)
	require.NoError(t, err)

	t.Run("collect from single file", func(t *testing.T) {
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{configFile}, &skipIgnored)
		require.NoError(t, err)
		assert.Len(t, dotfiles, 1)

		dotfile := dotfiles[0]
		assert.Equal(t, "testapp", dotfile.App)
		assert.Equal(t, configFile, dotfile.Path)
		assert.Equal(t, configContent, dotfile.Content)
		assert.Equal(t, "file", dotfile.FileType)
		assert.NotNil(t, dotfile.FileModifiedAt)
		assert.NotEmpty(t, dotfile.Hostname)
	})

	t.Run("collect from directory", func(t *testing.T) {
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{subDir}, &skipIgnored)
		require.NoError(t, err)

		// Should find 2 files (hidden files are ignored)
		assert.Len(t, dotfiles, 2)

		// Sort by path for consistent comparison
		if strings.Contains(dotfiles[0].Path, "file2") {
			dotfiles[0], dotfiles[1] = dotfiles[1], dotfiles[0]
		}

		assert.Equal(t, "testapp", dotfiles[0].App)
		assert.Equal(t, subFile1, dotfiles[0].Path)
		assert.Equal(t, subFile1Content, dotfiles[0].Content)

		assert.Equal(t, "testapp", dotfiles[1].App)
		assert.Equal(t, subFile2, dotfiles[1].Path)
		assert.Equal(t, subFile2Content, dotfiles[1].Content)
	})

	t.Run("collect from mixed paths", func(t *testing.T) {
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{configFile, subDir}, &skipIgnored)
		require.NoError(t, err)
		assert.Len(t, dotfiles, 3) // 1 file + 2 files from directory
	})

	t.Run("collect from non-existent path", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "does-not-exist")
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{nonExistentPath}, &skipIgnored)
		require.NoError(t, err)
		assert.Empty(t, dotfiles) // Should skip non-existent paths
	})

	t.Run("collect with ignored sections", func(t *testing.T) {
		// Create a file with ignored sections
		configWithIgnore := filepath.Join(tmpDir, "config_with_ignore.conf")
		configContentWithIgnore := `line1
# SHELLTIME IGNORE BEGIN
secret_key=123456
password=hidden
# SHELLTIME IGNORE END
line2
visible_key=value`
		err = os.WriteFile(configWithIgnore, []byte(configContentWithIgnore), 0644)
		require.NoError(t, err)

		// Test with skipIgnored = true (default behavior)
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{configWithIgnore}, &skipIgnored)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)

		// Should not contain ignored sections
		assert.NotContains(t, dotfiles[0].Content, "secret_key")
		assert.NotContains(t, dotfiles[0].Content, "password=hidden")
		assert.NotContains(t, dotfiles[0].Content, "SHELLTIME IGNORE")
		assert.Contains(t, dotfiles[0].Content, "line1")
		assert.Contains(t, dotfiles[0].Content, "line2")
		assert.Contains(t, dotfiles[0].Content, "visible_key=value")

		// Test with skipIgnored = false
		skipIgnored = false
		dotfiles, err = app.CollectFromPaths(ctx, "testapp", []string{configWithIgnore}, &skipIgnored)
		require.NoError(t, err)
		require.Len(t, dotfiles, 1)

		// Should contain all content including ignored sections
		assert.Contains(t, dotfiles[0].Content, "secret_key")
		assert.Contains(t, dotfiles[0].Content, "password=hidden")
		assert.Contains(t, dotfiles[0].Content, "SHELLTIME IGNORE")
	})
}

func TestBaseApp_collectFromDirectory(t *testing.T) {
	app := &BaseApp{name: "test"}

	// Create temporary directory structure
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files and directories
	file1 := filepath.Join(tmpDir, "file1.txt")
	err = os.WriteFile(file1, []byte("content1"), 0644)
	require.NoError(t, err)

	file2 := filepath.Join(tmpDir, "file2.txt")
	err = os.WriteFile(file2, []byte("content2"), 0644)
	require.NoError(t, err)

	// Create hidden file
	hiddenFile := filepath.Join(tmpDir, ".hidden")
	err = os.WriteFile(hiddenFile, []byte("hidden"), 0644)
	require.NoError(t, err)

	// Create subdirectory with file
	subDir := filepath.Join(tmpDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	subFile := filepath.Join(subDir, "subfile.txt")
	err = os.WriteFile(subFile, []byte("subcontent"), 0644)
	require.NoError(t, err)

	files, err := app.collectFromDirectory(tmpDir)
	require.NoError(t, err)

	// Should include regular files but not hidden files
	assert.Contains(t, files, file1)
	assert.Contains(t, files, file2)
	assert.Contains(t, files, subFile)
	assert.NotContains(t, files, hiddenFile)
	assert.Len(t, files, 3)
}

func TestBaseApp_IsEqual(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	// Create temporary files for testing
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile1 := filepath.Join(tmpDir, "file1.txt")
	testContent1 := "content1\n"
	err = os.WriteFile(testFile1, []byte(testContent1), 0644)
	require.NoError(t, err)

	testFile2 := filepath.Join(tmpDir, "file2.txt")
	testContent2 := "content2\n"
	err = os.WriteFile(testFile2, []byte(testContent2), 0644)
	require.NoError(t, err)

	t.Run("files are equal", func(t *testing.T) {
		files := map[string]string{
			testFile1: testContent1,
			testFile2: testContent2,
		}

		result, err := app.IsEqual(ctx, files)
		require.NoError(t, err)
		assert.True(t, result[testFile1])
		assert.True(t, result[testFile2])
	})

	t.Run("files are not equal", func(t *testing.T) {
		files := map[string]string{
			testFile1: testContent1,
			testFile2: "different content\n",
		}

		result, err := app.IsEqual(ctx, files)
		require.NoError(t, err)
		assert.True(t, result[testFile1])
		assert.False(t, result[testFile2])
	})

	t.Run("file does not exist locally", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "does-not-exist.txt")
		files := map[string]string{
			nonExistentFile: "some content",
		}

		result, err := app.IsEqual(ctx, files)
		require.NoError(t, err)
		assert.False(t, result[nonExistentFile])
	})

	t.Run("with tilde path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		// Create test file in home directory
		homeTestDir := filepath.Join(homeDir, ".shelltime-test")
		err = os.MkdirAll(homeTestDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(homeTestDir)

		homeTestFile := filepath.Join(homeTestDir, "test.txt")
		err = os.WriteFile(homeTestFile, []byte(testContent1), 0644)
		require.NoError(t, err)

		files := map[string]string{
			"~/.shelltime-test/test.txt": testContent1,
		}

		result, err := app.IsEqual(ctx, files)
		require.NoError(t, err)
		assert.True(t, result["~/.shelltime-test/test.txt"])
	})
}

func TestBaseApp_Backup(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	// Create temporary files for testing
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "file.txt")
	testContent := "original content\n"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	t.Run("backup existing file", func(t *testing.T) {
		err := app.Backup(ctx, []string{testFile}, false)
		require.NoError(t, err)

		// Check that backup file was created
		files, err := os.ReadDir(tmpDir)
		require.NoError(t, err)

		var backupFile string
		for _, file := range files {
			if strings.HasPrefix(file.Name(), "file.txt.backup.") {
				backupFile = filepath.Join(tmpDir, file.Name())
				break
			}
		}

		assert.NotEmpty(t, backupFile, "Backup file should be created")

		// Check backup content
		backupContent, err := os.ReadFile(backupFile)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(backupContent))
	})

	t.Run("backup non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "does-not-exist.txt")
		err := app.Backup(ctx, []string{nonExistentFile}, false)
		require.NoError(t, err) // Should not error, just skip
	})

	t.Run("backup with tilde path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		// Create test file in home directory
		homeTestDir := filepath.Join(homeDir, ".shelltime-test")
		err = os.MkdirAll(homeTestDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(homeTestDir)

		homeTestFile := filepath.Join(homeTestDir, "test.txt")
		err = os.WriteFile(homeTestFile, []byte(testContent), 0644)
		require.NoError(t, err)

		err = app.Backup(ctx, []string{"~/.shelltime-test/test.txt"}, false)
		require.NoError(t, err)

		// Check that backup was created
		files, err := os.ReadDir(homeTestDir)
		require.NoError(t, err)

		backupExists := false
		for _, file := range files {
			if strings.HasPrefix(file.Name(), "test.txt.backup.") {
				backupExists = true
				break
			}
		}
		assert.True(t, backupExists, "Backup should be created for tilde path")
	})
}

func TestBaseApp_Save(t *testing.T) {
	app := &BaseApp{name: "test"}
	ctx := context.Background()

	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "dotfile-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("save new file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "new-file.txt")
		testContent := "new content\n"

		files := map[string]string{
			testFile: testContent,
		}

		err := app.Save(ctx, files, false)
		require.NoError(t, err)

		// Check that file was created
		savedContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.EqualValues(t, testContent, string(savedContent))
	})

	t.Run("save to existing file with different content", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "existing-file.txt")
		originalContent := "original content\n"
		newContent := "updated content\n"

		// Create original file
		err := os.WriteFile(testFile, []byte(originalContent), 0644)
		require.NoError(t, err)

		files := map[string]string{
			testFile: newContent,
		}

		err = app.Save(ctx, files, false)
		require.NoError(t, err)

		// Check that file was updated
		savedContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, originalContent+newContent, string(savedContent))
	})

	t.Run("save identical content skips file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "identical-file.txt")
		content := "same content\n"

		// Create original file
		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)

		// Get original mod time
		originalInfo, err := os.Stat(testFile)
		require.NoError(t, err)
		originalModTime := originalInfo.ModTime()

		// Wait a bit to ensure mod time would change if file is written
		time.Sleep(10 * time.Millisecond)

		files := map[string]string{
			testFile: content,
		}

		err = app.Save(ctx, files, false)
		require.NoError(t, err)

		// Check that file mod time didn't change (file was not written)
		newInfo, err := os.Stat(testFile)
		require.NoError(t, err)
		assert.Equal(t, originalModTime, newInfo.ModTime(), "File should not be modified when content is identical")
	})

	t.Run("save creates directory if needed", func(t *testing.T) {
		nestedFile := filepath.Join(tmpDir, "nested", "dir", "file.txt")
		content := "nested content\n"

		files := map[string]string{
			nestedFile: content,
		}

		err := app.Save(ctx, files, false)
		require.NoError(t, err)

		// Check that directories were created and file was saved
		savedContent, err := os.ReadFile(nestedFile)
		require.NoError(t, err)
		assert.EqualValues(t, content, string(savedContent))
	})

	t.Run("save with tilde path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		// Create test directory in home
		homeTestDir := filepath.Join(homeDir, ".shelltime-test")
		err = os.MkdirAll(homeTestDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(homeTestDir)

		testContent := "tilde content\n"
		files := map[string]string{
			"~/.shelltime-test/tilde-file.txt": testContent,
		}

		err = app.Save(ctx, files, false)
		require.NoError(t, err)

		// Check that file was saved
		savedFile := filepath.Join(homeTestDir, "tilde-file.txt")
		savedContent, err := os.ReadFile(savedFile)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(savedContent))
	})
}

func TestBaseApp_Integration(t *testing.T) {
	app := &BaseApp{name: "integration-test"}
	ctx := context.Background()

	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "dotfile-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set up test files
	configFile := filepath.Join(tmpDir, "config.conf")
	configContent := "setting1=value1\nsetting2=value2\n"
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	subDir := filepath.Join(tmpDir, "configs")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	subFile := filepath.Join(subDir, "app.conf")
	subContent := "app_setting=app_value\n"
	err = os.WriteFile(subFile, []byte(subContent), 0644)
	require.NoError(t, err)

	t.Run("full workflow", func(t *testing.T) {
		// 1. Collect dotfiles
		skipIgnored := true
		dotfiles, err := app.CollectFromPaths(ctx, "testapp", []string{configFile, subDir}, &skipIgnored)
		require.NoError(t, err)
		assert.Len(t, dotfiles, 2)

		// 2. Check equality (should be equal initially)
		files := make(map[string]string)
		for _, dotfile := range dotfiles {
			files[dotfile.Path] = dotfile.Content
		}

		equality, err := app.IsEqual(ctx, files)
		require.NoError(t, err)
		assert.True(t, equality[configFile])
		assert.True(t, equality[subFile])

		// 3. Modify content and check inequality
		modifiedFiles := map[string]string{
			configFile: configContent + "new_setting=new_value\n",
			subFile:    subContent,
		}

		equality, err = app.IsEqual(ctx, modifiedFiles)
		require.NoError(t, err)
		assert.False(t, equality[configFile])
		assert.True(t, equality[subFile])

		// 4. Backup original files
		err = app.Backup(ctx, []string{configFile, subFile}, false)
		require.NoError(t, err)

		// 5. Save modified content
		err = app.Save(ctx, modifiedFiles, false)
		require.NoError(t, err)

		// 6. Verify files were updated
		updatedContent, err := os.ReadFile(configFile)
		require.NoError(t, err)
		assert.Equal(t, modifiedFiles[configFile], string(updatedContent))

		unchangedContent, err := os.ReadFile(subFile)
		require.NoError(t, err)
		assert.Equal(t, subContent, string(unchangedContent)) // Should remain unchanged
	})
}
