package model

import (
	"bytes"
	"io"

	"github.com/ugorji/go/codec"
)

var msgpackHandle = &codec.MsgpackHandle{
	WriteExt: true,
}

func MsgpackEncode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, msgpackHandle)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MsgpackDecode(data []byte, v interface{}) error {
	dec := codec.NewDecoder(bytes.NewReader(data), msgpackHandle)
	return dec.Decode(v)
}

func MsgpackEncodeWriter(w io.Writer, v interface{}) error {
	enc := codec.NewEncoder(w, msgpackHandle)
	return enc.Encode(v)
}

func MsgpackDecodeReader(r io.Reader, v interface{}) error {
	dec := codec.NewDecoder(r, msgpackHandle)
	return dec.Decode(v)
}
