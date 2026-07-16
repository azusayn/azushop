package llm

import (
	"context"

	"github.com/azusayn/azushop/proto/conf"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/lo"
)

const (
	MaxEmbeddingRequestInputToken = 300000
	// Some embedding providers impose a batch size limit (e.g. LiteLLM/bge-m3 max 32).
	// Split large batches to stay compatible.
	MaxEmbeddingBatchSize = 32
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
func (client *OpenAIClient) CreateEmbeddings(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	var results []pgvector.Vector
	for i := 0; i < len(texts); i += MaxEmbeddingBatchSize {
		end := i + MaxEmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		params := openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{
				OfArrayOfStrings: batch,
			},
			Model:          client.embeddingModel,
			EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
		}

		resp, err := client.Embeddings.New(ctx, params)
		if err != nil {
			return nil, err
		}

		for _, d := range resp.Data {
			vec := lo.Map(d.Embedding, func(val float64, _ int) float32 {
				return float32(val)
			})
			results = append(results, pgvector.NewVector(vec))
		}
	}

	return results, nil
}
