package pancakeswap

import (
	"context"

	"grpc_cake/gen/go/quoteswap"
)

type Swapper interface {
	GetQuote(ctx context.Context, req *quoteswap.GetQuoteRequest) (resp *quoteswap.GetQuoteResponse, err error)
	ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (resp *quoteswap.ExecuteTxResponse, err error)
}
