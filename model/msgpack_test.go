package model

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMsgpackEncodeDecode(t *testing.T) {
	t.Run("encode and decode struct", func(t *testing.T) {
		type TestStruct struct {
			Name   string
			Age    int
			Active bool
			Tags   []string
		}

		original := TestStruct{
			Name:   "John Doe",
			Age:    30,
			Active: true,
			Tags:   []string{"developer", "golang"},
		}

		encoded, err := MsgpackEncode(original)
		require.NoError(t, err)
		assert.NotEmpty(t, encoded)

		var decoded TestStruct
		err = MsgpackDecode(encoded, &decoded)
		require.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("encode and decode map", func(t *testing.T) {
		original := map[string]interface{}{
			"name":    "test",
			"value":   123,
			"enabled": true,
			"items":   []int{1, 2, 3},
		}

		encoded, err := MsgpackEncode(original)
		require.NoError(t, err)
		assert.NotEmpty(t, encoded)

		var decoded map[string]interface{}
		err = MsgpackDecode(encoded, &decoded)
		require.NoError(t, err)
		assert.Equal(t, original["name"], decoded["name"])
		assert.Equal(t, original["enabled"], decoded["enabled"])
	})

	t.Run("encode and decode slice", func(t *testing.T) {
		original := []string{"apple", "banana", "cherry"}

		encoded, err := MsgpackEncode(original)
		require.NoError(t, err)
		assert.NotEmpty(t, encoded)

		var decoded []string
		err = MsgpackDecode(encoded, &decoded)
		require.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("encode nil value", func(t *testing.T) {
		encoded, err := MsgpackEncode(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, encoded)

		var decoded interface{}
		err = MsgpackDecode(encoded, &decoded)
		require.NoError(t, err)
		assert.Nil(t, decoded)
	})

	t.Run("decode error with invalid data", func(t *testing.T) {
		var decoded string
		err := MsgpackDecode([]byte{0xFF, 0xFF, 0xFF}, &decoded)
		assert.Error(t, err)
	})
}

func TestMsgpackEncodeDecodeWriter(t *testing.T) {
	t.Run("encode and decode with writer/reader", func(t *testing.T) {
		type TestData struct {
			ID      int
			Message string
		}

		original := TestData{ID: 42, Message: "hello world"}

		var buf bytes.Buffer
		err := MsgpackEncodeWriter(&buf, original)
		require.NoError(t, err)

		var decoded TestData
		err = MsgpackDecodeReader(&buf, &decoded)
		require.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("multiple encode decode operations", func(t *testing.T) {
		var buf bytes.Buffer
		
		// Write multiple values
		err := MsgpackEncodeWriter(&buf, "first")
		require.NoError(t, err)
		
		err = MsgpackEncodeWriter(&buf, 42)
		require.NoError(t, err)
		
		err = MsgpackEncodeWriter(&buf, true)
		require.NoError(t, err)
		
		// Read them back
		var str string
		err = MsgpackDecodeReader(&buf, &str)
		require.NoError(t, err)
		assert.Equal(t, "first", str)
		
		var num int
		err = MsgpackDecodeReader(&buf, &num)
		require.NoError(t, err)
		assert.Equal(t, 42, num)
		
		var boolean bool
		err = MsgpackDecodeReader(&buf, &boolean)
		require.NoError(t, err)
		assert.True(t, boolean)
	})
}

func BenchmarkMsgpackEncode(b *testing.B) {
	type BenchData struct {
		ID       int
		Name     string
		Values   []float64
		Metadata map[string]string
	}

	data := BenchData{
		ID:     12345,
		Name:   "benchmark test",
		Values: []float64{1.1, 2.2, 3.3, 4.4, 5.5},
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MsgpackEncode(data)
	}
}

func BenchmarkMsgpackDecode(b *testing.B) {
	type BenchData struct {
		ID       int
		Name     string
		Values   []float64
		Metadata map[string]string
	}

	data := BenchData{
		ID:     12345,
		Name:   "benchmark test",
		Values: []float64{1.1, 2.2, 3.3, 4.4, 5.5},
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	encoded, _ := MsgpackEncode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded BenchData
		_ = MsgpackDecode(encoded, &decoded)
	}
}