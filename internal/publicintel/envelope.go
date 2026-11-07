package publicintel

import (
	"encoding/json"
	"fmt"

	"github.com/andrew867/bg-cli/internal/jsonutil"
)

type upstreamEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func unwrapData(body []byte) (json.RawMessage, error) {
	var env upstreamEnvelope
	if err := jsonutil.Decode(body, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Status == "error" {
		msg := "upstream error"
		if env.Error != nil && env.Error.Message != "" {
			msg = env.Error.Message
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if len(env.Data) == 0 {
		return nil, fmt.Errorf("envelope missing data")
	}
	return env.Data, nil
}
