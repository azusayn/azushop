package config

import (
	"encoding/json"
	"os"

	"github.com/azusayn/azushop/proto/conf"
	"go.yaml.in/yaml/v2"
	"google.golang.org/protobuf/encoding/protojson"
)

// LoadConfig reads a YAML config file and unmarshals it into the given protobuf message.
func LoadConfig(path string) (*conf.Bootstrap, error) {
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var v any
	if err := yaml.Unmarshal(yamlBytes, &v); err != nil {
		return nil, err
	}

	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var config conf.Bootstrap
	if err := protojson.Unmarshal(jsonBytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
