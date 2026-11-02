package codex

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type AuthMetadata struct {
	Exists bool
	Mode   string
	Size   int64
}

func ReadAuthMetadata(codexHome string) (AuthMetadata, error) {
	path := filepath.Join(codexHome, "auth.json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return AuthMetadata{Exists: false}, nil
	}
	if err != nil {
		return AuthMetadata{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return AuthMetadata{}, err
	}
	meta := AuthMetadata{Exists: true, Size: stat.Size()}
	decoder := json.NewDecoder(io.LimitReader(file, 4*1024*1024))
	token, err := decoder.Token()
	if err != nil {
		return meta, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return meta, errors.New("auth.json is not a JSON object")
	}
	for decoder.More() {
		keyToken, keyErr := decoder.Token()
		if keyErr != nil {
			return meta, keyErr
		}
		key, ok := keyToken.(string)
		if !ok {
			continue
		}
		if key == "auth_mode" {
			var value string
			if decodeErr := decoder.Decode(&value); decodeErr == nil {
				meta.Mode = value
			}
			continue
		}
		var discard json.RawMessage
		if decodeErr := decoder.Decode(&discard); decodeErr != nil {
			return meta, decodeErr
		}
	}
	return meta, nil
}
