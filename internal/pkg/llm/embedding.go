package llm

import (
	"context"

	"github.com/azusayn/azushop/proto/conf"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/samber/lo"
)

const (
	MaxEmbeddingRequestInputToken = 300000
)

type OpenAIClient struct {
	openai.Client
	embeddingModel string
}

func NewOpenAIClient(cd *conf.Data) *OpenAIClient {
	embeddingAPI := cd.GetEmbeddingApi()
	client := openai.NewClient(
		option.WithAPIKey(embeddingAPI.GetSecret()),
		// e.g. https://example.ai.com/v1
		option.WithBaseURL(embeddingAPI.GetEndpoint()),
	)
	return &OpenAIClient{
		embeddingModel: embeddingAPI.GetModel(),
		Client:         client,
	}
}

// Ref: https://developers.openai.com/api/reference/go/resources/embeddings/methods/create
func (client *OpenAIClient) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Model: client.embeddingModel,
	}

	resp, err := client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, err
	}

	results := lo.Map(resp.Data, func(d openai.Embedding, _ int) []float32 {
		return lo.Map(d.Embedding, func(val float64, _ int) float32 {
			return float32(val)
		})
	})

	return results, nil
}
