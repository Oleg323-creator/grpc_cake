package service

import (
	"context"
	"errors"
	"fmt"

	"grpc_cake/gen/go/quoteswap"
	"grpc_cake/internal/pancakeswap"
)

type QuoteSwapServiceServer struct {
	quoteswap.UnimplementedQuoteSwapServiceServer
	// using this map to save chain as key and Swapper implementation as value
	V2Services map[string]pancakeswap.Swapper
	V3Services map[string]pancakeswap.Swapper
}

func (s *QuoteSwapServiceServer) GetQuote(ctx context.Context, req *quoteswap.GetQuoteRequest) (*quoteswap.GetQuoteResponse, error) {
	var service pancakeswap.Swapper
	chain := req.GetChain()

	switch req.GetDex() {
	case "v2":
		service = s.V2Services[chain]
	case "v3":
		service = s.V3Services[chain]
	default:
		return nil, errors.New(fmt.Sprintf("unsupported dex version: %s", req.GetDex()))
	}

	if service == nil {
		return nil, errors.New(fmt.Sprintf("no service found for chain: %s", chain))
	}

	return service.GetQuote(ctx, req)
}

func (s *QuoteSwapServiceServer) ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (*quoteswap.ExecuteTxResponse, error) {
	var service pancakeswap.Swapper
	chain := req.QuotingResponse.GetChain()

	switch req.QuotingResponse.GetDex() {
	case "v2":
		service = s.V2Services[chain]
	case "v3":
		service = s.V3Services[chain]
	default:
		return nil, errors.New(fmt.Sprintf("unsupported dex version: %s", req.QuotingResponse.GetDex()))
	}

	if service == nil {
		return nil, errors.New(fmt.Sprintf("no service found for chain: %s", chain))
	}

	return service.ExecuteSwap(ctx, req)
}
