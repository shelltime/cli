package model

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// diffMergeTestSuite defines the test suite for DiffMergeService
type diffMergeTestSuite struct {
	suite.Suite
	service DiffMergeService
}

// SetupTest runs before each test in the suite
func (suite *diffMergeTestSuite) SetupTest() {
	suite.service = NewDiffMergeService()
}

// TestNewDiffMergeService tests the service constructor
func (suite *diffMergeTestSuite) TestNewDiffMergeService() {
	require.NotNil(suite.T(), suite.service, "NewDiffMergeService() should not return nil")

	// Test that it implements the interface
	var _ DiffMergeService = suite.service
}

// TestFindDiff tests the FindDiff method with various inputs
func (s *diffMergeTestSuite) TestFindDiffAndApplyChanges() {
	tests := []struct {
		name          string
		localContent  string
		remoteContent string
		expectError   bool
		expectDelta   bool
		finalContent  string
	}{
		{
			name:          "identical content",
			localContent:  "hello world",
			remoteContent: "hello world",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "hello world",
		},
		{
			name:          "different content",
			localContent:  "hello world",
			remoteContent: "hello universe",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "hello world\nhello universe",
		},
		{
			name:          "empty local content",
			localContent:  "",
			remoteContent: "\n\nhello world",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "\r\r\n\nhello world",
		},
		{
			name:          "empty remote content",
			localContent:  "hello world",
			remoteContent: "",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "hello world",
		},
		{
			name:          "both empty",
			localContent:  "",
			remoteContent: "",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "",
		},
		{
			name:          "multiline content",
			localContent:  "line 1\nline 2\nline 3",
			remoteContent: "line 1\nmodified line 2\nline 3",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "line 1\nline 2\nline 3\nmodified line 2\n",
		},
		{
			name:          "large content difference",
			localContent:  "short",
			remoteContent: "this is a much longer string with many more characters than the original",
			expectError:   false,
			expectDelta:   true,
			finalContent:  "short\nthis is a much longer string with many more characters than the original",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			delta, err := s.service.FindDiff(tt.localContent, tt.remoteContent)

			if tt.expectError {
				s.Error(err, "FindDiff() should return an error")
			} else {
				s.NoError(err, "FindDiff() should not return an error")
			}

			if tt.expectDelta {
				s.NotNil(delta, "FindDiff() should return a delta object")
			}

			if delta != nil {
				// Verify the delta is a valid encoded object
				s.Contains([]plumbing.ObjectType{plumbing.OFSDeltaObject, plumbing.REFDeltaObject},
					delta.Type(), "FindDiff() should return a delta object type")

				// Verify we can read the delta size
				s.GreaterOrEqual(delta.Size(), int64(0), "FindDiff() delta size should be non-negative")
			}

			l, err := s.service.ConvertToEncodedObject(tt.localContent)

			s.Nil(err)
			finalContent, err := s.service.ApplyDiff(l, delta)
			s.Nil(err)

			s.EqualValues(tt.finalContent, string(finalContent))
		})
	}
}

// TestFindDiffWithSpecialCharacters tests FindDiff with special characters
func (suite *diffMergeTestSuite) TestFindDiffWithSpecialCharacters() {
	tests := []struct {
		name          string
		localContent  string
		remoteContent string
	}{
		{
			name:          "unicode characters",
			localContent:  "hello 🌍",
			remoteContent: "hello 🌎",
		},
		{
			name:          "special characters",
			localContent:  "hello\t\n\r",
			remoteContent: "hello\t\n\r\x00",
		},
		{
			name:          "json content",
			localContent:  `{"name": "test", "value": 123}`,
			remoteContent: `{"name": "test", "value": 456}`,
		},
		{
			name:          "binary-like content",
			localContent:  "\x00\x01\x02\x03",
			remoteContent: "\x00\x01\x02\x04",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			delta, err := suite.service.FindDiff(tt.localContent, tt.remoteContent)

			suite.NoError(err, "FindDiff() with special characters should not fail")
			suite.NotNil(delta, "FindDiff() with special characters should return a delta")
		})
	}
}

// TestFindDiffInterface tests the interface implementation
func (suite *diffMergeTestSuite) TestFindDiffInterface() {
	// Test that the service properly implements the interface
	var service DiffMergeService = NewDiffMergeService()

	delta, err := service.FindDiff("test", "test")
	suite.NoError(err, "Interface method FindDiff() should not fail")
	suite.NotNil(delta, "Interface method FindDiff() should return a delta")
}

// TestDiffMergeTestSuite runs the test suite
func TestDiffMergeTestSuite(t *testing.T) {
	suite.Run(t, new(diffMergeTestSuite))
}
