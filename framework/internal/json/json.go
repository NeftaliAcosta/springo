package json

import (
	stdjson "encoding/json"
	"io"

	gojson "github.com/goccy/go-json"
)

type codec interface {
	Marshal(any) ([]byte, error)
	Unmarshal([]byte, any) error
	NewEncoder(io.Writer) encoder
	NewDecoder(io.Reader) decoder
}

type encoder interface{ Encode(any) error }
type decoder interface{ Decode(any) error }

type standardCodec struct{}

func (standardCodec) Marshal(value any) ([]byte, error)      { return stdjson.Marshal(value) }
func (standardCodec) Unmarshal(data []byte, value any) error { return stdjson.Unmarshal(data, value) }
func (standardCodec) NewEncoder(writer io.Writer) encoder    { return stdjson.NewEncoder(writer) }
func (standardCodec) NewDecoder(reader io.Reader) decoder    { return stdjson.NewDecoder(reader) }

type goccyCodec struct{}

func (goccyCodec) Marshal(value any) ([]byte, error)      { return gojson.Marshal(value) }
func (goccyCodec) Unmarshal(data []byte, value any) error { return gojson.Unmarshal(data, value) }
func (goccyCodec) NewEncoder(writer io.Writer) encoder    { return gojson.NewEncoder(writer) }
func (goccyCodec) NewDecoder(reader io.Reader) decoder    { return gojson.NewDecoder(reader) }

func configuredCodec() codec {
	return standardCodec{}
}

// Marshal serializes a value with the standard JSON engine.
func Marshal(value any) ([]byte, error) { return configuredCodec().Marshal(value) }

// Unmarshal parses JSON data with the standard JSON engine.
func Unmarshal(data []byte, value any) error { return configuredCodec().Unmarshal(data, value) }

// NewEncoder creates an encoder using the standard JSON engine.
func NewEncoder(writer io.Writer) encoder { return configuredCodec().NewEncoder(writer) }

// NewDecoder creates a decoder using the standard JSON engine.
func NewDecoder(reader io.Reader) decoder { return configuredCodec().NewDecoder(reader) }

// NewEncoderFor creates an encoder for a configured JSON engine.
func NewEncoderFor(engine string, writer io.Writer) encoder {
	if engine == "go-json" {
		return goccyCodec{}.NewEncoder(writer)
	}
	return standardCodec{}.NewEncoder(writer)
}

// NewDecoderFor creates a decoder for a configured JSON engine.
func NewDecoderFor(engine string, reader io.Reader) decoder {
	if engine == "go-json" {
		return goccyCodec{}.NewDecoder(reader)
	}
	return standardCodec{}.NewDecoder(reader)
}
