package externalprovider

import (
	"context"
	"fmt"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda"
	"github.com/serverledge-faas/serverledge/internal/function"
	"time"
)

type Provider interface {
	CreateFunction(ctx context.Context, function *function.Function) (string, error)
	ListFunctions(ctx context.Context) ([]string, error)
	InvokeProviderFunction(request *function.Request, payload []byte) (function.ExecutionReport, error)
	DeleteProviderFunction(ctx context.Context, function *function.Function) error
	GetRegion() (string, error)
	GetRtt() time.Duration
}

const LambdaOffloader = "aws"

func NewOffloader(name string) (Provider, error) {
	switch name {
	case "aws":
		p, err := lambda.GetProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize AWS provider: %w", err)
		}
		return p, nil

	//Here we can add other provider

	default:
		return nil, fmt.Errorf("unknown offload provider %q", name)
	}
}
