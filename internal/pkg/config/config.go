package config

import (
	"encoding/json"
	"os"

	"github.com/azusayn/azushop/proto/conf"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// LoadYAMLConfig reads a YAML config file and unmarshals it into the given protobuf message.
// It does this by first decoding the YAML into generic types, then converting to JSON
// and unmarshaling with protojson.
func LoadYAMLConfig(path string, msg *conf.Bootstrap) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	jsonData, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(jsonData, msg)
}
